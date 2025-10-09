#!/usr/bin/env python3

from abc import ABC, abstractmethod
from argparse import ArgumentParser
from collections import UserDict, UserList
from copy import copy
from dataclasses import dataclass
from pathlib import Path
from string import ascii_letters, digits, printable, whitespace
from types import ModuleType
from typing import (
    Any,
    Callable,
    Iterable,
    Optional,
    SupportsFloat,
    Tuple,
    Type,
    TypeVar,
    Union,
    final,
)
import code
import enum
import html
import json
import math
import os
import random
import re
import sys
import traceback


try:
    # Mellifera canonically uses re2 for regular expressions.
    import re2
except ImportError:
    # Reasonable fallback if re2 is not installed, e.g if mf.py is used as a
    # standalone script outside of a virtual environment.
    import re as re2  # type: ignore

readline: Optional[ModuleType]
try:
    # REPL readline support.
    import readline
except ImportError:
    readline = None

rng = random.Random()


def escape(text: str) -> str:
    MAPPING = {
        "\t": "\\t",
        "\n": "\\n",
        '"': '\\"',
        "\\": "\\\\",
    }
    return "".join([MAPPING.get(c, c) for c in text])


def hexscape(text: Union[bytes, str]) -> str:
    data = text if isinstance(text, bytes) else text.encode("utf-8")
    return "".join([f"\\x{b:02X}" for b in data])


def quote(item: Any) -> str:
    text = str(item)
    return f"`{text}`" if "`" not in text else f'"{text}"'


class InvalidFieldAccess(KeyError):
    def __init__(self, value: "Value", field: "Value"):
        self.value = copy(value)
        self.field = copy(field)
        super().__init__(str(self))

    def __str__(self):
        return f"invalid access into value {self.value} with field {self.field}"


class SharedVectorData(UserList["Value"]):
    def __init__(self, data: Optional[Iterable["Value"]] = None):
        self.uses: int = 0
        super().__init__(data)

    def __copy__(self) -> "SharedVectorData":
        return SharedVectorData([copy(x) for x in self])


class SharedMapData(UserDict["Value", "Value"]):
    def __init__(self, data: Optional[dict["Value", "Value"]] = None):
        self.uses: int = 0
        super().__init__(data)

    def __copy__(self) -> "SharedMapData":
        return SharedMapData({copy(k): copy(v) for k, v in self.data.items()})


class SharedSetData(UserDict["Value", None]):
    def __init__(self, data: Optional[Iterable["Value"]] = None):
        self.uses: int = 0
        if data is not None:
            super().__init__({k: None for k in data})
        else:
            super().__init__()

    def insert(self, element: "Value") -> None:
        super().__setitem__(element, None)

    def remove(self, element: "Value") -> None:
        del self[element]

    def __copy__(self) -> "SharedSetData":
        return SharedSetData([copy(k) for k in self.data.keys()])


ValueType = TypeVar("ValueType", bound="Value")


class Value(ABC):
    meta: Optional["MetaMap"]

    @staticmethod
    @abstractmethod
    def typename() -> str:
        raise NotImplementedError()

    @abstractmethod
    def __hash__(self):
        raise NotImplementedError()

    @abstractmethod
    def __eq__(self, other):
        raise NotImplementedError()

    @abstractmethod
    def __str__(self):
        raise NotImplementedError()

    def __setitem__(self, key: "Value", value: "Value") -> None:
        raise NotImplementedError()  # optionally overridden by subclasses

    def __getitem__(self, key: "Value") -> "Value":
        raise NotImplementedError()  # optionally overridden by subclasses

    def __delitem__(self, key: "Value") -> None:
        raise NotImplementedError()  # optionally overridden by subclasses

    @abstractmethod
    def __copy__(self) -> "Value":
        raise NotImplementedError()

    def cow(self) -> None:
        # Assume no data needs to be uniquely copied by default when a COW
        # operation is being performed. Value metamaps provide an immutable
        # view on meta operations, and therefore do not need to be COWed.
        pass

    def metavalue(self, name: "Value") -> Optional["Value"]:
        if self.meta is None:
            return None
        if name not in self.meta:
            return None
        return self.meta[name]

    def metafunction(self, name: "Value") -> Optional[Union["Function", "Builtin"]]:
        if self.meta is None:
            return None
        if name not in self.meta:
            return None
        function = self.meta[name]
        if not isinstance(function, (Builtin, Function)):
            return None
        return function


@final
@dataclass
class Null(Value):
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "null"

    @staticmethod
    def new() -> "Null":
        return Null(meta=None)

    def __hash__(self):
        return 0

    def __eq__(self, other):
        return type(self) is type(other)

    def __str__(self):
        return "null"

    def __copy__(self) -> "Null":
        return Null(self.meta if self.meta else None)


@final
@dataclass
class Boolean(Value):
    data: bool
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "boolean"

    @staticmethod
    def new(data: bool) -> "Boolean":
        return Boolean(data, _BOOLEAN_META)

    def __hash__(self):
        return hash(self.data)

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return self.data == other.data

    def __str__(self):
        return "true" if self.data else "false"

    def __copy__(self) -> "Boolean":
        return Boolean(self.data, self.meta if self.meta else None)


@final
@dataclass
class Number(Value):
    data: SupportsFloat
    meta: Optional["MetaMap"]

    @staticmethod
    def typename() -> str:
        return "number"

    @staticmethod
    def new(data: SupportsFloat) -> "Number":
        return Number(data, _NUMBER_META)

    def __init__(self, data: SupportsFloat, meta: Optional["MetaMap"] = None):
        # PEP 484 specifies that when an argument is annotated as having type
        # `float`, an argument of type `int` is accepted by the type checker.
        # The mellifera number type is specifically an IEEE-754 double
        # precision floating point number, so a float cast is used to ensure
        # the typed data is actually a Python float.
        self.data = float(data)
        self.meta = meta

    def __hash__(self):
        return hash(self.data)

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return self.data == other.data

    def __int__(self) -> int:
        return int(float(self.data))

    def __float__(self) -> float:
        return float(self.data)

    def __str__(self):
        if math.isnan(self.data):
            return "NaN"
        if self.data == +math.inf:
            return "Inf"
        if self.data == -math.inf:
            return "-Inf"
        string = str(self.data)
        dot = string.find(".")
        end = len(string)
        while string[end - 1] == "0":
            end -= 1  # Remove trailing zeros.
        if dot == end - 1:
            end -= 1  # Remove trailing dot.
        return string[0:end]

    def __copy__(self) -> "Number":
        result = Number.__new__(Number)
        result.data = self.data
        result.meta = self.meta if self.meta else None
        return result


@final
@dataclass
class String(Value):
    data: bytes
    meta: Optional["MetaMap"]

    @staticmethod
    def typename() -> str:
        return "string"

    @staticmethod
    def new(data: Union[bytes, str]) -> "String":
        return String(data, _STRING_META)

    def __init__(self, data: Union[bytes, str], meta: Optional["MetaMap"] = None):
        match data:
            case bytes():
                self.data = data
            case str():
                self.data = data.encode("utf-8")
        self.meta = meta

    def __hash__(self):
        return hash(self.bytes)

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return self.bytes == other.bytes

    def __str__(self):
        return f'"{escape(self.runes)}"'

    def __contains__(self, item) -> bool:
        if isinstance(item, String):
            return item.bytes in self.bytes
        if isinstance(item, bytes):
            return item in self.bytes
        if isinstance(item, str):
            return item in self.runes
        return False

    def __copy__(self) -> "String":
        result = String.__new__(String)
        result.data = self.data
        result.meta = self.meta if self.meta else None
        return result

    @property
    def bytes(self) -> bytes:
        return self.data

    @property
    def runes(self) -> str:
        return self.bytes.decode("utf-8", errors="replace")


@final
class Regexp(Value):
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "regexp"

    def __init__(self, string: bytes, pattern=None, meta: Optional["MetaMap"] = None):
        self.pattern = pattern if pattern is not None else re2.compile(string)
        self.string = string
        self.meta = meta

    @staticmethod
    def new(string: bytes, pattern=None) -> "Regexp":
        return Regexp(string, pattern, _REGEXP_META)

    def __hash__(self):
        return hash(self.string)

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return self.string == other.string

    def __str__(self):
        return f'r"{escape(self.string.decode("utf-8"))}"'

    def __copy__(self) -> "Regexp":
        return Regexp(self.string, self.pattern, self.meta if self.meta else None)


@final
@dataclass
class Vector(Value):
    data: SharedVectorData
    meta: Optional["MetaMap"]

    @staticmethod
    def typename() -> str:
        return "vector"

    @staticmethod
    def new(
        data: Optional[Union[SharedVectorData, Iterable[Value]]] = None,
    ) -> "Vector":
        return Vector(data, _VECTOR_META)

    def __init__(
        self,
        data: Optional[Union[SharedVectorData, Iterable[Value]]] = None,
        meta: Optional["MetaMap"] = None,
    ):
        if data is not None and not isinstance(data, SharedVectorData):
            data = SharedVectorData(data)
        self.data = data if data is not None else SharedVectorData()
        self.data.uses += 1
        self.meta = meta

    def __del__(self):
        assert self.data.uses >= 1
        self.data.uses -= 1

    def __hash__(self):
        return hash(str(self))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        if len(self.data) != len(other.data):
            return False
        for i in range(len(self.data)):
            if self.data[i] != other.data[i]:
                return False
        return True

    def __str__(self):
        elements = ", ".join([str(x) for x in self.data])
        return f"[{elements}]"

    def __contains__(self, item) -> bool:
        return item in self.data

    def __setitem__(self, key: Value, value: Value) -> None:
        if not isinstance(key, Number):
            raise KeyError(f"attempted vector access using non-number key {key}")
        index = float(key.data)
        if not index.is_integer():
            raise KeyError(f"attempted vector access using non-integer number {index}")
        index = int(index)
        if index < 0:
            raise KeyError(f"attempted vector access using a negative index {index}")
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        self.data.__setitem__(index, value)

    def __getitem__(self, key: Value) -> Value:
        if not isinstance(key, Number):
            raise KeyError(f"attempted vector access using non-number key {key}")
        index = float(key.data)
        if not index.is_integer():
            raise KeyError(f"attempted vector access using non-integer number {index}")
        index = int(index)
        if index < 0:
            raise KeyError(f"attempted vector access using a negative index {index}")
        return self.data.__getitem__(index)

    def __delitem__(self, key: Value) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        super().__delitem__(key)

    def __copy__(self) -> "Vector":
        return Vector(self.data, self.meta if self.meta else None)

    def cow(self) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1


@dataclass
class Map(Value):
    data: SharedMapData
    meta: Optional["MetaMap"]

    @staticmethod
    def typename() -> str:
        return "map"

    @staticmethod
    def new(
        data: Optional[Union[SharedMapData, dict[Value, Value]]] = None,
    ) -> "Map":
        return Map(data, _MAP_META)

    def __init__(
        self,
        data: Optional[Union[SharedMapData, dict[Value, Value]]] = None,
        meta: Optional["MetaMap"] = None,
    ):
        if data is not None and not isinstance(data, SharedMapData):
            data = SharedMapData(data)
        self.data = data if data is not None else SharedMapData()
        self.data.uses += 1
        self.meta = meta

    def __del__(self):
        assert self.data.uses >= 1
        self.data.uses -= 1

    def __hash__(self):
        return hash(str(self))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        if len(self.data) != len(other.data):
            return False
        for k, v in self.data.items():
            if k not in other.data or other.data[k] != v:
                return False
        return True

    def __str__(self):
        if len(self.data) == 0:
            return "Map{}"
        elements = ", ".join([f"{str(k)}: {str(v)}" for k, v in self.data.items()])
        return f"{{{elements}}}"

    def __contains__(self, item) -> bool:
        return item in self.data

    def __setitem__(self, key: Value, value: Value) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        try:
            self.data.__setitem__(key, value)
        except KeyError:
            raise InvalidFieldAccess(self, key)

    def __getitem__(self, key: Value) -> Value:
        try:
            return self.data.__getitem__(key)
        except KeyError:
            raise InvalidFieldAccess(self, key)

    def __delitem__(self, key: Value) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        try:
            self.data.__delitem__(key)
        except KeyError:
            raise InvalidFieldAccess(self, key)

    def __copy__(self) -> "Map":
        return Map(self.data, self.meta if self.meta else None)

    def cow(self) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1


@final
@dataclass
class MetaMap(Map):
    def __init__(
        self,
        name: String,
        data: Optional[Union[SharedMapData, dict[Value, Value]]] = None,
    ):
        # Metamaps may not have metamaps themselves.
        super().__init__(data=data, meta=None)
        self.name = name

    def __setitem__(self, key: Value, value: Value) -> None:
        # Attempting to alter a metamap is a runtime error. This allows us to
        # use a single Python reference for each metamap rather than creating
        # new metamaps for each created/copied value.
        raise Exception(f"attempted to modify metamap {self}")

    def __copy__(self) -> "MetaMap":
        return self


@final
@dataclass
class Set(Value):
    data: SharedSetData
    meta: Optional["MetaMap"]

    @staticmethod
    def typename() -> str:
        return "set"

    @staticmethod
    def new(
        data: Optional[Union[SharedSetData, Iterable[Value]]] = None,
    ) -> "Set":
        return Set(data, _SET_META)

    def __init__(
        self,
        data: Optional[Union[SharedSetData, Iterable[Value]]] = None,
        meta: Optional["MetaMap"] = None,
    ):
        if data is not None and not isinstance(data, SharedSetData):
            data = SharedSetData(data)
        self.data = data if data is not None else SharedSetData()
        self.data.uses += 1
        self.meta = meta

    def __del__(self):
        assert self.data.uses >= 1
        self.data.uses -= 1

    def __hash__(self):
        return hash(str(self))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        if len(self.data) != len(other.data):
            return False
        for k in self.data.keys():
            if k not in other.data:
                return False
        return True

    def __str__(self):
        if len(self.data) == 0:
            return "Set{}"
        elements = ", ".join([f"{str(k)}" for k in self.data.keys()])
        return f"{{{elements}}}"

    def __contains__(self, item) -> bool:
        return item in self.data

    def insert(self, element: "Value") -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        self.data.insert(element)

    def remove(self, element: "Value") -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1
        self.data.remove(element)

    def __copy__(self) -> "Set":
        return Set(self.data, self.meta if self.meta else None)

    def cow(self) -> None:
        if self.data.uses > 1:
            self.data.uses -= 1
            self.data = copy(self.data)  # copy-on-write
            self.data.uses += 1


@final
@dataclass
class Reference(Value):
    data: Value
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "reference"

    @staticmethod
    def new(data: Value) -> "Reference":
        return Reference(data, _REFERENCE_META)

    def __hash__(self):
        return hash(id(self.data))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return id(self.data) == id(other.data)

    def __str__(self):
        return f"reference@{hex(id(self.data))}"

    def __copy__(self) -> "Reference":
        return Reference(self.data, self.meta if self.meta else None)

    def cow(self) -> None:
        # We explicitly do *not* copy `self.data` as the copied data should
        # still point to the original referenced Python `Value` object.
        pass


@final
@dataclass
class Function(Value):
    ast: "AstExpressionFunction"
    env: "Environment"
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "function"

    @staticmethod
    def new(ast: "AstExpressionFunction", env: "Environment") -> "Function":
        return Function(ast, env, _FUNCTION_META)

    def __hash__(self):
        return hash(id(self.ast)) + hash(id(self.env))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return id(self.ast) == id(other.ast)

    def __str__(self):
        name = self.ast.name.runes if self.ast.name is not None else "function"
        ugly = any(c not in ascii_letters + digits + "_" + ":" for c in name)
        name = f'"{escape(name)}"' if ugly else name
        if self.ast.location is not None:
            return f"{name}@[{self.ast.location}]"
        return f"{name}"

    def __copy__(self) -> "Function":
        return Function(self.ast, self.env, self.meta if self.meta else None)


@dataclass
class Builtin(Value):
    meta: Optional["MetaMap"] = None

    def __post_init__(self):
        if self.meta is None:
            self.meta = _FUNCTION_META

    @property
    @abstractmethod
    def name(self) -> str:
        """
        Name associated with the builtin.
        Builtin subclasses should add the builtin name as a class property.
        """
        raise NotImplementedError()

    @staticmethod
    def typename() -> str:
        return "function"

    def __hash__(self):
        return hash(id(self.function))

    def __eq__(self, other):
        return type(self) is type(other)

    def __str__(self):
        return f"{self.name}@builtin"

    def __copy__(self) -> "Builtin":
        return self

    def call(self, arguments: list[Value]) -> Union[Value, "Error"]:
        try:
            result = self.function(arguments)
            if isinstance(result, Error):
                return result
            if isinstance(result, Value):
                return result
            # Special cases in which a builtin does not return a mellifera
            # value or error. If None is returned, likely due to a missing
            # return statement, automatically return a null value. If a
            # non-None object is returned, automatically convert that object
            # into an external value wrapping the object.
            return Null.new() if result is None else External.new(result)
        except Exception as e:
            message = f"{e}"
            if len(message) == 0:
                message = f"encountered exception {type(e).__name__}"
            return Error(None, String.new(message))

    @staticmethod
    def expect_argument_count(arguments: list[Value], count: int) -> None:
        if len(arguments) != count:
            raise Exception(
                f"invalid argument count (expected {count}, received {len(arguments)})"
            )

    @staticmethod
    def typed_argument(
        arguments: list[Value], index: int, ty: Type[ValueType]
    ) -> ValueType:
        argument = arguments[index]
        if not isinstance(argument, ty):
            raise Exception(
                f"expected {ty.typename()}-like value for argument {index}, received {typename(argument)}"
            )
        return argument

    @staticmethod
    def typed_argument_reference(
        arguments: list[Value], index: int, ty: Type[ValueType]
    ) -> Tuple[Reference, ValueType]:
        argument = arguments[index]
        if not (isinstance(argument, Reference) and isinstance(argument.data, ty)):
            raise Exception(
                f"expected reference to {ty.typename()}-like value for argument {index}, received {typename(argument)}"
            )
        return (argument, argument.data)

    @abstractmethod
    def function(self, arguments: list[Value]) -> Union[Value, "Error"]:
        raise NotImplementedError()


# Special builtin intended for use as a sentinel value and placeholder.
class BuiltinImplicitUninitialized(Builtin):
    name = "IMPLICIT-UNINITIALIZED"

    def __post_init__(self):
        pass  # Explicitly do not set self.meta.

    def function(self, arguments: list[Value]) -> Union[Value, "Error"]:
        return Error(None, "IMPLICIT-UNINITIALIZED")


# Special builtin intended for use as a sentinel value and placeholder.
class BuiltinExplicitUninitialized(Builtin):
    name = "EXPLICIT-UNINITIALIZED"

    def __post_init__(self):
        pass  # Explicitly do not set self.meta.

    def function(self, arguments: list[Value]) -> Union[Value, "Error"]:
        return Error(None, "EXPLICIT-UNINITIALIZED")


@dataclass
class BuiltinFromSource(Builtin):
    env: Optional["Environment"] = None
    evaluated: Union[Function, Builtin] = BuiltinImplicitUninitialized()

    def __post_init__(self):
        super().__post_init__()
        if not isinstance(self.evaluated, BuiltinExplicitUninitialized):
            self.initialize()

    def __hash__(self):
        return hash(id(self.evaluated))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return self.evaluated == other.evaluated

    def initialize(self):
        evaluated = eval_source(self.source(), self.env)
        if not isinstance(evaluated, (Function, Builtin)):
            raise Exception(f"invalid builtin from source {quote(evaluated)}")
        self.evaluated = evaluated

    def function(self, arguments: list[Value]) -> Union[Value, "Error"]:
        result = call(None, self.evaluated, arguments)
        if isinstance(result, Error):
            # Remove internal trace elements so that the trace appears to start
            # from the builtin, similar to builtins defined using host code.
            result.trace = list()
        return result

    @staticmethod
    @abstractmethod
    def source() -> str:
        raise NotImplementedError()


@dataclass
class External(Value):
    data: Any
    meta: Optional["MetaMap"] = None

    @staticmethod
    def typename() -> str:
        return "external"

    @staticmethod
    def new(data: Any) -> "External":
        return External(data, meta=None)

    def __hash__(self):
        return hash(id(self.data))

    def __eq__(self, other):
        if type(self) is not type(other):
            return False
        return id(self.data) == id(other.data)

    def __str__(self):
        return f"external({repr(self.data)})"

    def __copy__(self) -> "External":
        return External(self.data, self.meta if self.meta else None)

    def cow(self) -> None:
        # We explicitly do *not* copy self.data as the copied data should still
        # point to the original external Python object.
        pass


@dataclass
class SourceLocation:
    filename: Optional[str]
    line: int

    def __str__(self):
        if self.filename is None:
            return f"line {self.line}"
        return f"{self.filename}, line {self.line}"


class TokenKind(enum.Enum):
    # Meta
    ILLEGAL = "illegal"
    EOF = "eof"
    # Identifiers and Literals
    IDENTIFIER = "identifier"
    TEMPLATE = "template"
    NUMBER = "number"
    STRING = "string"
    REGEXP = "regexp"
    # Operators
    ADD = "+"
    SUB = "-"
    MUL = "*"
    DIV = "/"
    REM = "%"
    EQ = "=="
    NE = "!="
    LE = "<="
    GE = ">="
    LT = "<"
    GT = ">"
    EQ_RE = "=~"
    NE_RE = "!~"
    MKREF = ".&"
    DEREF = ".*"
    DOT = "."
    SCOPE = "::"
    ASSIGN = "="
    # Delimiters
    COMMA = ","
    COLON = ":"
    SEMICOLON = ";"
    LPAREN = "("
    RPAREN = ")"
    LBRACE = "{"
    RBRACE = "}"
    LBRACKET = "["
    RBRACKET = "]"
    # Keywords
    TYPE = "type"
    NULL = "null"
    TRUE = "true"
    FALSE = "false"
    MAP = "Map"
    SET = "Set"
    NEW = "new"
    NOT = "not"
    AND = "and"
    OR = "or"
    LET = "let"
    IF = "if"
    ELIF = "elif"
    ELSE = "else"
    FOR = "for"
    IN = "in"
    WHILE = "while"
    BREAK = "break"
    CONTINUE = "continue"
    TRY = "try"
    CATCH = "catch"
    ERROR = "error"
    RETURN = "return"
    FUNCTION = "function"

    def __str__(self):
        return self.value


@dataclass
class Token:
    KEYWORDS = {
        # fmt: off
        str(TokenKind.TYPE):     TokenKind.TYPE,
        str(TokenKind.NULL):     TokenKind.NULL,
        str(TokenKind.TRUE):     TokenKind.TRUE,
        str(TokenKind.FALSE):    TokenKind.FALSE,
        str(TokenKind.MAP):      TokenKind.MAP,
        str(TokenKind.SET):      TokenKind.SET,
        str(TokenKind.NEW):      TokenKind.NEW,
        str(TokenKind.NOT):      TokenKind.NOT,
        str(TokenKind.AND):      TokenKind.AND,
        str(TokenKind.OR):       TokenKind.OR,
        str(TokenKind.LET):      TokenKind.LET,
        str(TokenKind.IF):       TokenKind.IF,
        str(TokenKind.ELIF):     TokenKind.ELIF,
        str(TokenKind.ELSE):     TokenKind.ELSE,
        str(TokenKind.FOR):      TokenKind.FOR,
        str(TokenKind.IN):       TokenKind.IN,
        str(TokenKind.WHILE):    TokenKind.WHILE,
        str(TokenKind.BREAK):    TokenKind.BREAK,
        str(TokenKind.CONTINUE): TokenKind.CONTINUE,
        str(TokenKind.TRY):      TokenKind.TRY,
        str(TokenKind.CATCH):    TokenKind.CATCH,
        str(TokenKind.ERROR):    TokenKind.ERROR,
        str(TokenKind.RETURN):   TokenKind.RETURN,
        str(TokenKind.FUNCTION): TokenKind.FUNCTION,
        # fmt: on
    }

    kind: TokenKind
    literal: str
    location: Optional[SourceLocation] = None
    template: Optional[list[Union[bytes, "AstExpression"]]] = None
    number: Optional[float] = None
    string: Optional[bytes] = None

    def __str__(self):
        if self.kind == TokenKind.EOF:
            return "end-of-file"
        if self.kind == TokenKind.ILLEGAL:

            def prettyable(c):
                return c in printable and c not in whitespace

            def prettyrepr(c):
                return c if prettyable(c) else f"{ord(c):#04x}"

            return "".join(map(prettyrepr, self.literal))
        if self.kind == TokenKind.IDENTIFIER:
            return f"{self.literal}"
        if self.kind == TokenKind.NUMBER:
            return f"{self.literal}"
        if self.kind == TokenKind.STRING:
            return f"{self.literal}"
        if self.kind.value in Token.KEYWORDS:
            return self.kind.value
        return f"{self.kind.value}"

    @staticmethod
    def lookup_identifier(identifier: str) -> TokenKind:
        return Token.KEYWORDS.get(identifier, TokenKind.IDENTIFIER)


class Lexer:
    EOF_LITERAL = ""
    RE_IDENTIFIER = re.compile(r"^[a-zA-Z_]\w*", re.ASCII)
    RE_NUMBER_HEX = re.compile(r"^0x[0-9a-fA-F]+", re.ASCII)
    RE_NUMBER_DEC = re.compile(r"^\d+(\.\d+)?", re.ASCII)

    def __init__(self, source: str, location: Optional[SourceLocation] = None):
        self.source: str = source
        # What position does the source "start" being parsed from.
        # None if the source is being lexed in a location-independent manner.
        self.location: Optional[SourceLocation] = location
        self.position: int = 0

    @staticmethod
    def _is_letter(ch: str) -> bool:
        return ch.isalpha() or ch == "_"

    def _current_character(self) -> str:
        if self.position >= len(self.source):
            return Lexer.EOF_LITERAL
        return self.source[self.position]

    def _peek_character(self) -> str:
        if self.position + 1 >= len(self.source):
            return Lexer.EOF_LITERAL
        return self.source[self.position + 1]

    def _is_eof(self) -> bool:
        return self.position >= len(self.source)

    def _remaining(self) -> str:
        return self.source[self.position :]

    def _advance_character(self) -> None:
        if self._is_eof():
            return
        if self.location is not None:
            self.location.line += int(self.source[self.position] == "\n")
        self.position += 1

    def _expect_character(self, character: str) -> None:
        assert len(character) == 1
        current = self._current_character()
        if self._is_eof():
            raise ParseError(
                self.location,
                f"expected {quote(character)}, found end-of-file",
            )
        if current != character:
            raise ParseError(
                self.location,
                f"expected {quote(character)}, found {quote(current)}",
            )
        self._advance_character()

    def _skip_whitespace(self) -> None:
        while not self._is_eof() and self._current_character() in whitespace:
            self._advance_character()

    def _skip_comment(self) -> None:
        if self._current_character() != "#":
            return
        while not self._is_eof() and self._current_character() != "\n":
            self._advance_character()
        self._advance_character()

    def _skip_whitespace_and_comments(self) -> None:
        while not self._is_eof() and (
            self._current_character() in whitespace or self._current_character() == "#"
        ):
            self._skip_whitespace()
            self._skip_comment()

    def _new_token(self, kind: TokenKind, literal: str, **kwargs) -> Token:
        location = (
            SourceLocation(self.location.filename, self.location.line)
            if self.location is not None
            else None
        )
        return Token(kind, literal, location, **kwargs)

    def _lex_keyword_or_identifier(self) -> Token:
        assert Lexer._is_letter(self._current_character())
        match = Lexer.RE_IDENTIFIER.match(self.source[self.position :])
        assert match is not None  # guaranteed by regexp
        text = match[0]
        self.position += len(text)
        if text in Token.KEYWORDS:
            return self._new_token(Token.KEYWORDS[text], text)
        return self._new_token(TokenKind.IDENTIFIER, text)

    def _lex_number(self) -> Token:
        assert self._current_character() in digits
        match = Lexer.RE_NUMBER_HEX.match(self.source[self.position :])
        if match is not None:
            text = match[0]
            self.position += len(text)
            return self._new_token(TokenKind.NUMBER, text, number=float(int(text, 16)))
        match = Lexer.RE_NUMBER_DEC.match(self.source[self.position :])
        assert match is not None  # guaranteed by regexp
        text = match[0]
        self.position += len(text)
        return self._new_token(TokenKind.NUMBER, text, number=float(text))

    def _lex_string_character(self) -> bytes:
        if self._is_eof():
            raise ParseError(
                self.location,
                "expected character, found end-of-file",
            )

        if self._current_character() == "\n":
            raise ParseError(
                self.location,
                "expected character, found newline",
            )

        if not self._current_character().isprintable():
            raise ParseError(
                self.location,
                f"expected printable character, found {hex(ord(self._current_character()))}",
            )

        if self._current_character() == "\\" and self._peek_character() == "t":
            self._advance_character()
            self._advance_character()
            return b"\t"

        if self._current_character() == "\\" and self._peek_character() == "n":
            self._advance_character()
            self._advance_character()
            return b"\n"

        if self._current_character() == "\\" and self._peek_character() == '"':
            self._advance_character()
            self._advance_character()
            return b'"'

        if self._current_character() == "\\" and self._peek_character() == "\\":
            self._advance_character()
            self._advance_character()
            return b"\\"

        if self._current_character() == "\\" and self._peek_character() == "x":
            self._advance_character()
            self._advance_character()
            nybbles = self._current_character() + self._peek_character()
            self._advance_character()
            self._advance_character()
            sequence = "\\x" + nybbles
            HEX_MAPPING = {
                "0": 0x0,
                "1": 0x1,
                "2": 0x2,
                "3": 0x3,
                "4": 0x4,
                "5": 0x5,
                "6": 0x6,
                "7": 0x7,
                "8": 0x8,
                "9": 0x9,
                "A": 0xA,
                "B": 0xB,
                "C": 0xC,
                "D": 0xD,
                "E": 0xE,
                "F": 0xF,
                "a": 0xA,
                "b": 0xB,
                "c": 0xC,
                "d": 0xD,
                "e": 0xE,
                "f": 0xF,
            }
            if not (nybbles[0] in HEX_MAPPING and nybbles[1] in HEX_MAPPING):
                raise ParseError(
                    self.location,
                    f"expected hexadecimal escape sequence, found {quote(sequence)}",
                )
            byte = (HEX_MAPPING[nybbles[0]] << 4) | HEX_MAPPING[nybbles[1]]
            return bytes([byte])

        if self._current_character() == "\\":
            sequence = escape(self._current_character() + self._peek_character())
            raise ParseError(
                self.location,
                f"expected escape sequence, found {quote(sequence)}",
            )

        character = self._current_character()
        self._advance_character()
        return character.encode("utf-8")

    def _lex_string(self) -> Token:
        start = self.position
        self._expect_character('"')
        string = b""
        while not self._is_eof() and self._current_character() != '"':
            string += self._lex_string_character()
        self._expect_character('"')
        literal = self.source[start : self.position]
        return self._new_token(TokenKind.STRING, literal, string=string)

    def _lex_raw_string_character(self) -> bytes:
        if self._is_eof():
            raise ParseError(
                self.location,
                "expected character, found end-of-file",
            )
        character = self._current_character()
        self._advance_character()
        return character.encode("utf-8")

    def _lex_raw_string(self) -> Token:
        location = copy(self.location)
        start = self.position
        string = b""
        if self._remaining().startswith("```"):
            self._expect_character("`")
            self._expect_character("`")
            self._expect_character("`")
            while not self._is_eof():
                if self._remaining().startswith("```"):
                    break
                string += self._current_character().encode("utf-8")
                self._advance_character()
            self._expect_character("`")
            self._expect_character("`")
            self._expect_character("`")
            literal = self.source[start + 4 : self.position - 3]
            # Future-proof in case I want to add variable-number-of-tick raw
            # string literals in the future.
            if len(literal) == 0:
                raise ParseError(
                    location,
                    "invalid empty multi-tick raw string",
                )
        else:
            self._expect_character("`")
            while not self._is_eof() and self._current_character() != "`":
                string += self._current_character().encode("utf-8")
                self._advance_character()
            self._expect_character("`")
            literal = self.source[start : self.position]
        return self._new_token(TokenKind.STRING, literal, string=string)

    def _lex_template(self) -> Token:
        start = self.position
        location = copy(self.location)
        self._expect_character("$")

        template: list[Union[bytes, "AstExpression"]] = list()
        string = ""  # current text being parsed

        def lex_template_element(default):
            nonlocal string
            if self._remaining().startswith("{{"):
                string += "{"
                self.position += len("{{")
                return
            if self._remaining().startswith("}}"):
                string += "}"
                self.position += len("}}")
                return
            if self._remaining().startswith("{"):
                if len(string) != 0:
                    template.append(string.encode("utf-8"))
                    string = str()
                self.position += len("{")
                try:
                    lexer = Lexer(self._remaining())
                    parser = Parser(lexer)
                    expression = parser.parse_expression()
                except Exception as e:
                    raise ParseError(location, str(e))
                template.append(expression)
                if not parser.current_token.kind == TokenKind.RBRACE:
                    raise ParseError(
                        location,
                        f"expected `}}` to close template expression, found {quote(parser.current_token.kind)}",
                    )
                self.position += self._remaining().rfind(lexer._remaining())
                return
            string += default()

        if self._remaining().startswith("```"):
            self._expect_character("`")
            self._expect_character("`")
            self._expect_character("`")
            while not self._is_eof() and not self._remaining().startswith("```"):
                lex_template_element(
                    lambda: self._lex_raw_string_character().decode("utf-8")
                )
            if len(string) != 0:
                template.append(string.encode("utf-8"))
            self._expect_character("`")
            self._expect_character("`")
            self._expect_character("`")
        elif self._current_character() == "`":
            self._expect_character("`")
            while not self._is_eof() and self._current_character() != "`":
                lex_template_element(
                    lambda: self._lex_raw_string_character().decode("utf-8")
                )
            if len(string) != 0:
                template.append(string.encode("utf-8"))
            self._expect_character("`")
        elif self._current_character() == '"':
            self._expect_character('"')
            while not self._is_eof() and self._current_character() != '"':
                lex_template_element(
                    lambda: self._lex_string_character().decode("utf-8")
                )
            if len(string) != 0:
                template.append(string.encode("utf-8"))
            self._expect_character('"')
        else:
            raise ParseError(
                self.location,
                f'expected template of the form $"...", $`...` or $```...```, found `$` followed by {quote(self._current_character())}',
            )

        literal = self.source[start : self.position]
        return self._new_token(TokenKind.TEMPLATE, literal, template=template)

    def _lex_regexp(self) -> Token:
        start = self.position
        self._expect_character("r")
        string = b""
        if self._current_character() == '"':
            self._expect_character('"')
            while self._current_character() != '"':
                string += self._lex_string_character()
            self._expect_character('"')
        if self._current_character() == "`":
            self._expect_character("`")
            while self._current_character() != "`":
                string += self._lex_raw_string_character()
            self._expect_character("`")
        literal = self.source[start : self.position]
        return self._new_token(TokenKind.REGEXP, literal, string=string)

    def next_token(self) -> Token:
        if self.location is not None:
            file = self.location.filename
            line = self.location.line
            self.location = SourceLocation(file, line)
        self._skip_whitespace_and_comments()

        if self._is_eof():
            return self._new_token(TokenKind.EOF, Lexer.EOF_LITERAL)

        # Literals, Identifiers, and Keywords
        if self._current_character() == '"':
            return self._lex_string()
        if self._current_character() == "`":
            return self._lex_raw_string()
        if self._current_character() == "$":
            return self._lex_template()
        if self._remaining().startswith('r"') or self._remaining().startswith("r`"):
            return self._lex_regexp()
        if Lexer._is_letter(self._current_character()):
            return self._lex_keyword_or_identifier()
        if self._current_character() in digits:
            return self._lex_number()

        # Operators
        if self._current_character() == "+":
            self._advance_character()
            return self._new_token(TokenKind.ADD, str(TokenKind.ADD))
        if self._current_character() == "-":
            self._advance_character()
            return self._new_token(TokenKind.SUB, str(TokenKind.SUB))
        if self._current_character() == "*":
            self._advance_character()
            return self._new_token(TokenKind.MUL, str(TokenKind.MUL))
        if self._current_character() == "/":
            self._advance_character()
            return self._new_token(TokenKind.DIV, str(TokenKind.DIV))
        if self._current_character() == "%":
            self._advance_character()
            return self._new_token(TokenKind.REM, str(TokenKind.REM))
        if self._current_character() == "=" and self._peek_character() == "=":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.EQ, str(TokenKind.EQ))
        if self._current_character() == "!" and self._peek_character() == "=":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.NE, str(TokenKind.NE))
        if self._current_character() == "<" and self._peek_character() == "=":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.LE, str(TokenKind.LE))
        if self._current_character() == ">" and self._peek_character() == "=":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.GE, str(TokenKind.GE))
        if self._current_character() == "<":
            self._advance_character()
            return self._new_token(TokenKind.LT, str(TokenKind.LT))
        if self._current_character() == ">":
            self._advance_character()
            return self._new_token(TokenKind.GT, str(TokenKind.GT))
        if self._current_character() == "=" and self._peek_character() == "~":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.EQ_RE, str(TokenKind.EQ_RE))
        if self._current_character() == "!" and self._peek_character() == "~":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.NE_RE, str(TokenKind.NE_RE))
        if self._current_character() == "." and self._peek_character() == "&":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.MKREF, str(TokenKind.MKREF))
        if self._current_character() == "." and self._peek_character() == "*":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.DEREF, str(TokenKind.DEREF))
        if self._current_character() == ".":
            self._advance_character()
            return self._new_token(TokenKind.DOT, str(TokenKind.DOT))
        if self._current_character() == ":" and self._peek_character() == ":":
            self._advance_character()
            self._advance_character()
            return self._new_token(TokenKind.SCOPE, str(TokenKind.SCOPE))
        if self._current_character() == "=":
            self._advance_character()
            return self._new_token(TokenKind.ASSIGN, str(TokenKind.ASSIGN))

        # Delimiters
        if self._current_character() == ",":
            self._advance_character()
            return self._new_token(TokenKind.COMMA, str(TokenKind.COMMA))
        if self._current_character() == ":":
            self._advance_character()
            return self._new_token(TokenKind.COLON, str(TokenKind.COLON))
        if self._current_character() == ";":
            self._advance_character()
            return self._new_token(TokenKind.SEMICOLON, str(TokenKind.SEMICOLON))
        if self._current_character() == "(":
            self._advance_character()
            return self._new_token(TokenKind.LPAREN, str(TokenKind.LPAREN))
        if self._current_character() == ")":
            self._advance_character()
            return self._new_token(TokenKind.RPAREN, str(TokenKind.RPAREN))
        if self._current_character() == "{":
            self._advance_character()
            return self._new_token(TokenKind.LBRACE, str(TokenKind.LBRACE))
        if self._current_character() == "}":
            self._advance_character()
            return self._new_token(TokenKind.RBRACE, str(TokenKind.RBRACE))
        if self._current_character() == "[":
            self._advance_character()
            return self._new_token(TokenKind.LBRACKET, str(TokenKind.LBRACKET))
        if self._current_character() == "]":
            self._advance_character()
            return self._new_token(TokenKind.RBRACKET, str(TokenKind.RBRACKET))

        token = self._new_token(TokenKind.ILLEGAL, self._current_character())
        self._advance_character()
        return token


@dataclass
class ParseError(Exception):
    location: Optional[SourceLocation]
    why: str

    def __str__(self):
        if self.location is None:
            return f"{self.why}"
        return f"[{self.location}] {self.why}"


class Environment:
    @dataclass
    class Lookup:
        value: Value
        store: Map

    def __init__(self, outer: Optional["Environment"] = None):
        self.outer: Optional["Environment"] = outer
        self.store: Map = Map()

    def let(self, name: String, value: Value) -> None:
        self.store[name] = value

    def get(self, name: String) -> Optional[Value]:
        value = self.store.data.get(name, None)
        if value is None and self.outer is not None:
            return self.outer.get(name)
        return value

    def lookup(self, name: String) -> Optional[Lookup]:
        value = self.store.data.get(name, None)
        if value is None and self.outer is not None:
            return self.outer.lookup(name)
        if value is None:
            return None
        return Environment.Lookup(value, self.store)


@dataclass
class Return:
    value: Value


@dataclass
class Break:
    location: Optional[SourceLocation]


@dataclass
class Continue:
    location: Optional[SourceLocation]


@dataclass
class Error:
    @dataclass
    class TraceElement:
        location: Optional[SourceLocation]
        function: Union[Function, Builtin]

    location: Optional[SourceLocation]
    value: Value
    trace: list[TraceElement]

    def __init__(self, location: Optional[SourceLocation], value: Union[str, Value]):
        self.location = location
        self.value = String.new(value) if isinstance(value, str) else value
        self.trace = list()

    def __str__(self):
        if isinstance(self.value, String):
            return f"{self.value.runes}"
        return f"{self.value}"


ControlFlow = Union[Return, Break, Continue, Error]


CONST_STRING_INTO_STRING = String("into_string")
CONST_STRING_NEXT = String("next")
CONST_STRING_PATH = String("path")
CONST_STRING_FILE = String("file")
CONST_STRING_DIRECTORY = String("directory")
CONST_STRING_MODULE = String("module")


def typename(value: Value) -> str:
    if value.meta is not None:
        return value.meta.name.runes
    return value.typename()


def update_named_functions(map: "AstExpressionMap", prefix: bytes = b""):
    """
    Update the name values of named functions that are children somewhere in
    this map, either direct map-level value or a decendent of another map.
    """
    for k, v in map.elements:
        if isinstance(k, AstExpressionString) and isinstance(v, AstExpressionFunction):
            v.name = String.new(prefix + k.data.bytes)
        if isinstance(k, AstExpressionString) and isinstance(v, AstExpressionMap):
            update_named_functions(
                v, prefix + k.data.bytes + str(TokenKind.SCOPE).encode("utf-8")
            )


class AstNode(ABC):
    location: Optional[SourceLocation]


class AstExpression(AstNode):
    location: Optional[SourceLocation]

    @abstractmethod
    def eval(self, env: Environment) -> Union[Value, Error]:
        raise NotImplementedError()


class AstStatement(AstNode):
    location: Optional[SourceLocation]

    @abstractmethod
    def eval(self, env: Environment) -> Optional[ControlFlow]:
        raise NotImplementedError()


@final
@dataclass
class AstProgram(AstNode):
    location: Optional[SourceLocation]
    statements: list[AstStatement]

    def eval(self, env: Environment) -> Optional[Union[Value, Error]]:
        for statement in self.statements:
            result = statement.eval(env)
            if isinstance(result, Return):
                return result.value
            if isinstance(result, Break):
                return Error(self.location, "attempted to break outside of a loop")
            if isinstance(result, Continue):
                return Error(self.location, "attempted to continue outside of a loop")
            if isinstance(result, Error):
                return result
        return None


@final
@dataclass
class AstIdentifier(AstNode):
    """
    Identifier with no additional behavior attached.
    """

    location: Optional[SourceLocation]
    name: String


@final
@dataclass
class AstExpressionIdentifier(AstExpression):
    """
    Identifier evaluated as an identifier/symbol expression to produce a value.
    """

    location: Optional[SourceLocation]
    name: String

    def eval(self, env: Environment) -> Union[Value, Error]:
        value: Optional[Value] = env.get(self.name)
        if value is None:
            return Error(
                self.location, f"identifier {quote(self.name.runes)} is not defined"
            )
        return value


@final
@dataclass
class AstExpressionTemplate(AstExpression):
    location: Optional[SourceLocation]
    template: list[Union[bytes, "AstExpression"]]

    def eval(self, env: Environment) -> Union[Value, Error]:
        output: list[bytes] = list()
        for element in self.template:
            if isinstance(element, bytes):
                output.append(element)
                continue
            assert isinstance(element, AstExpression)
            result = element.eval(Environment(env))
            if isinstance(result, Error):
                return result
            metafunction = result.metafunction(CONST_STRING_INTO_STRING)
            if metafunction is not None:
                result = call(None, metafunction, [Reference.new(result)])
                if isinstance(result, Error):
                    return result
                if not isinstance(result, String):
                    return Error(
                        None,
                        f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
                    )
                output.append(result.bytes)
            else:
                if isinstance(result, String):
                    output.append(result.bytes)
                    continue
                output.append(str(result).encode("utf-8"))
        return String.new(b"".join(output))


@final
@dataclass
class AstExpressionNull(AstExpression):
    location: Optional[SourceLocation]
    data: Null

    def eval(self, env: Environment) -> Union[Value, Error]:
        return copy(self.data)


@final
@dataclass
class AstExpressionBoolean(AstExpression):
    location: Optional[SourceLocation]
    data: Boolean

    def eval(self, env: Environment) -> Union[Value, Error]:
        return copy(self.data)


@final
@dataclass
class AstExpressionNumber(AstExpression):
    location: Optional[SourceLocation]
    data: Number

    def eval(self, env: Environment) -> Union[Value, Error]:
        return copy(self.data)


@final
@dataclass
class AstExpressionString(AstExpression):
    location: Optional[SourceLocation]
    data: String

    def eval(self, env: Environment) -> Union[Value, Error]:
        return copy(self.data)


@final
@dataclass
class AstExpressionRegexp(AstExpression):
    location: Optional[SourceLocation]
    data: Regexp

    def eval(self, env: Environment) -> Union[Value, Error]:
        return copy(self.data)


@final
@dataclass
class AstExpressionVector(AstExpression):
    location: Optional[SourceLocation]
    elements: list[AstExpression]

    def eval(self, env: Environment) -> Union[Value, Error]:
        values = SharedVectorData()
        for x in self.elements:
            result = x.eval(env)
            if isinstance(result, Error):
                return result
            values.append(copy(result))
        return Vector.new(values)


@final
@dataclass
class AstExpressionMap(AstExpression):
    location: Optional[SourceLocation]
    elements: list[Tuple[AstExpression, AstExpression]]

    def eval(self, env: Environment) -> Union[Value, Error]:
        elements = SharedMapData()
        for k, v in self.elements:
            k_result = k.eval(env)
            if isinstance(k_result, Error):
                return k_result
            v_result = v.eval(env)
            if isinstance(v_result, Error):
                return v_result
            elements[copy(k_result)] = copy(v_result)
        return Map.new(elements)


@final
@dataclass
class AstExpressionSet(AstExpression):
    location: Optional[SourceLocation]
    elements: list[AstExpression]

    def eval(self, env: Environment) -> Union[Value, Error]:
        elements = SharedSetData()
        for x in self.elements:
            result = x.eval(env)
            if isinstance(result, Error):
                return result
            elements.insert(copy(result))
        return Set.new(elements)


@final
@dataclass
class AstExpressionFunction(AstExpression):
    location: Optional[SourceLocation]
    parameters: list[AstIdentifier]
    body: "AstBlock"
    name: Optional[String] = None

    def eval(self, env: Environment) -> Union[Value, Error]:
        return Function.new(self, env)


@final
@dataclass
class AstExpressionType(AstExpression):
    location: Optional[SourceLocation]
    name: String
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        type = self.expression.eval(env)
        if isinstance(type, Error):
            return type
        if not isinstance(type, Map):
            return Error(
                self.expression.location,
                f"expected map-like value, received {typename(type)}",
            )
        return MetaMap(name=self.name, data=type.data)


@final
@dataclass
class AstExpressionNew(AstExpression):
    location: Optional[SourceLocation]
    meta: AstExpression
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        meta = self.meta.eval(env)
        if isinstance(meta, Error):
            return meta
        expression = self.expression.eval(env)
        if isinstance(expression, Error):
            return expression
        if isinstance(meta, MetaMap):
            expression.meta = copy(meta)
            return expression
        if isinstance(meta, Map):
            return Error(
                self.meta.location,
                f"expected map-like value created with the {quote(TokenKind.TYPE)} expression, received regular map value {meta}",
            )
        return Error(
            self.meta.location,
            f"expected map-like value, received {typename(meta)}",
        )


@final
@dataclass
class AstExpressionGrouped(AstExpression):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        return self.expression.eval(env)


@final
@dataclass
class AstExpressionPositive(AstExpression):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        if isinstance(result, Number):
            return Number.new(+float(result.data))
        return Error(
            self.location,
            f"attempted unary + operation with type {quote(typename(result))}",
        )


@final
@dataclass
class AstExpressionNegative(AstExpression):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        if not isinstance(result, Number):
            return Error(
                self.location,
                f"attempted unary - operation with type {quote(typename(result))}",
            )
        return Number.new(-float(result.data))


@final
@dataclass
class AstExpressionNot(AstExpression):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        if isinstance(result, Boolean):
            return Boolean.new(not result.data)
        return Error(
            self.location,
            f"attempted unary not operation with type {quote(typename(result))}",
        )


@final
@dataclass
class AstExpressionAnd(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        if isinstance(lhs, Boolean) and not lhs.data:
            return Boolean.new(False)  # short circuit

        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(rhs, Boolean) and not rhs.data:
            return Boolean.new(False)  # short circuit

        if isinstance(lhs, Boolean) and isinstance(rhs, Boolean):
            return Boolean.new(lhs.data and rhs.data)
        return Error(
            self.location,
            f"attempted binary and operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionOr(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        if isinstance(lhs, Boolean) and lhs.data:
            return Boolean.new(True)  # short circuit

        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(rhs, Boolean) and rhs.data:
            return Boolean.new(True)  # short circuit

        if isinstance(lhs, Boolean) and isinstance(rhs, Boolean):
            return Boolean.new(lhs.data or rhs.data)
        return Error(
            self.location,
            f"attempted binary or operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionEq(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        return Boolean.new(lhs == rhs)


@final
@dataclass
class AstExpressionNe(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        return Boolean.new(lhs != rhs)


@final
@dataclass
class AstExpressionLe(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(lhs, Number) and isinstance(rhs, Number):
            return Boolean.new(float(lhs.data) <= float(rhs.data))
        if isinstance(lhs, String) and isinstance(rhs, String):
            return Boolean.new(lhs.bytes <= rhs.bytes)
        return Error(
            self.location,
            f"attempted <= operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionGe(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(lhs, Number) and isinstance(rhs, Number):
            return Boolean.new(float(lhs.data) >= float(rhs.data))
        if isinstance(lhs, String) and isinstance(rhs, String):
            return Boolean.new(lhs.bytes >= rhs.bytes)
        return Error(
            self.location,
            f"attempted >= operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionLt(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(lhs, Number) and isinstance(rhs, Number):
            return Boolean.new(float(lhs.data) < float(rhs.data))
        if isinstance(lhs, String) and isinstance(rhs, String):
            return Boolean.new(lhs.bytes < rhs.bytes)
        return Error(
            self.location,
            f"attempted < operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionGt(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(lhs, Number) and isinstance(rhs, Number):
            return Boolean.new(float(lhs.data) > float(rhs.data))
        if isinstance(lhs, String) and isinstance(rhs, String):
            return Boolean.new(lhs.bytes > rhs.bytes)
        return Error(
            self.location,
            f"attempted > operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionEqRe(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, String) and isinstance(rhs, Regexp)):
            return Error(
                self.location,
                f"attempted =~ operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        global re_match_result
        re_match_result = rhs.pattern.search(lhs.bytes)
        return Boolean.new(re_match_result is not None)


@final
@dataclass
class AstExpressionNeRe(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, String) and isinstance(rhs, Regexp)):
            return Error(
                self.location,
                f"attempted =~ operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        global re_match_result
        re_match_result = rhs.pattern.search(lhs.bytes)
        return Boolean.new(re_match_result is None)


@final
@dataclass
class AstExpressionAdd(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(lhs, Number) and isinstance(rhs, Number):
            return Number.new(float(lhs.data) + float(rhs.data))
        if isinstance(lhs, String) and isinstance(rhs, String):
            return String.new(lhs.bytes + rhs.bytes)
        if isinstance(lhs, Vector) and isinstance(rhs, Vector):
            return Vector.new([copy(x) for x in lhs.data] + [copy(x) for x in rhs.data])
        return Error(
            self.location,
            f"attempted + operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
        )


@final
@dataclass
class AstExpressionSub(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, Number) and isinstance(rhs, Number)):
            return Error(
                self.location,
                f"attempted - operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        return Number.new(float(lhs.data) - float(rhs.data))


@final
@dataclass
class AstExpressionMul(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, Number) and isinstance(rhs, Number)):
            return Error(
                self.location,
                f"attempted * operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        return Number.new(float(lhs.data) * float(rhs.data))


@final
@dataclass
class AstExpressionDiv(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, Number) and isinstance(rhs, Number)):
            return Error(
                self.location,
                f"attempted / operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        if float(rhs.data) == 0.0:
            return Error(self.location, "division by zero")
        return Number.new(float(lhs.data) / float(rhs.data))


@final
@dataclass
class AstExpressionRem(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        lhs = self.lhs.eval(env)
        if isinstance(lhs, Error):
            return lhs
        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if not (isinstance(lhs, Number) and isinstance(rhs, Number)):
            return Error(
                self.location,
                f"attempted % operation with types {quote(typename(lhs))} and {quote(typename(rhs))}",
            )
        if float(rhs.data) == 0.0:
            return Error(self.location, "remainder with divisor zero")
        # The remainder will have the same sign as the dividend.
        # This behavior is identical to C's remainder operator.
        #   +7 % +3 => +1
        #   +7 % -3 => +1
        #   -7 % +3 => -1
        #   -7 % -3 => -1
        return Number.new(math.fmod(float(lhs.data), float(rhs.data)))


@final
@dataclass
class AstExpressionFunctionCall(AstExpression):
    location: Optional[SourceLocation]
    function: AstExpression
    arguments: list[AstExpression]

    def eval(self, env: Environment) -> Union[Value, Error]:
        self_argument: Optional[Value] = None
        if isinstance(self.function, AstExpressionAccessDot):
            # Special case when dot access is used for a function call. An
            # implicit `self` argument is passed by reference to the function.
            store = self.function.store.eval(env)
            if isinstance(store, Error):
                return store
            self_argument = Reference.new(store)
            try:
                function = store[self.function.field.name]
            except (NotImplementedError, IndexError, KeyError):
                function = None
            try:
                if function is None and store.meta is not None:
                    function = store.meta[self.function.field.name]
            except KeyError:
                function = None
            try:
                if (
                    function is None
                    and isinstance(store, Reference)
                    and store.data.meta is not None
                ):
                    self_argument = store
                    function = store.data.meta[self.function.field.name]
            except (NotImplementedError, IndexError, KeyError):
                function = None
            if function is None:
                return Error(
                    self.location,
                    f"invalid method access with name {self.function.field.name}",
                )
        else:
            result = self.function.eval(env)
            if isinstance(result, Error):
                return result
            function = result
        if not isinstance(function, (Function, Builtin)):
            return Error(
                self.location,
                f"attempted to call non-function type {quote(typename(function))} with value {function}",
            )

        arguments: list[Value] = list()
        if self_argument is not None:
            arguments.append(self_argument)
        for argument in self.arguments:
            result = argument.eval(env)
            if isinstance(result, Error):
                return result
            arguments.append(copy(result))
        return call(self.location, function, arguments)


@final
@dataclass
class AstExpressionAccessIndex(AstExpression):
    location: Optional[SourceLocation]
    store: AstExpression
    field: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        store = self.store.eval(env)
        if isinstance(store, Error):
            return store
        field = self.field.eval(env)
        if isinstance(field, Error):
            return field
        if isinstance(store, Vector):
            try:
                return store[field]
            except (NotImplementedError, IndexError, KeyError):
                return Error(self.location, f"invalid vector access with index {field}")
        if isinstance(store, Map):
            try:
                return store[field]
            except (NotImplementedError, IndexError, KeyError):
                return Error(self.location, f"invalid map access with field {field}")
        return Error(
            self.location,
            f"attempted to access field of type {quote(typename(store))} with type {quote(typename(field))}",
        )


@final
@dataclass
class AstExpressionAccessScope(AstExpression):
    location: Optional[SourceLocation]
    store: AstExpression
    field: AstIdentifier

    def eval(self, env: Environment) -> Union[Value, Error]:
        store = self.store.eval(env)
        if isinstance(store, Error):
            return store
        field = self.field.name
        if not isinstance(store, Map):
            return Error(
                self.location,
                f"attempted to access field of type {quote(typename(store))}",
            )
        try:
            return store[field]
        except KeyError:
            return Error(self.location, f"invalid map access with field {field}")


@final
@dataclass
class AstExpressionAccessDot(AstExpression):
    location: Optional[SourceLocation]
    store: AstExpression
    field: AstIdentifier

    def eval(self, env: Environment) -> Union[Value, Error]:
        store = self.store.eval(env)
        if isinstance(store, Error):
            return store
        field = self.field.name

        # foo.bar
        try:
            return store[field]
        except (NotImplementedError, IndexError, KeyError):
            pass

        # foo.into_bar()
        try:
            if store.meta is not None:
                return store.meta[field]
        except KeyError:
            pass

        # Special case where a reference value is implicitly dereferenced when
        # accessing the target field.
        if isinstance(store, Reference):
            deref_store = store.data

            # foo.*.bar
            try:
                return deref_store[field]
            except (NotImplementedError, IndexError, KeyError):
                pass

            # foo.*.into_bar()
            try:
                if deref_store.meta is not None:
                    return deref_store.meta[field]
            except KeyError:
                pass

            return Error(
                self.location,
                f"invalid {store.typename()} to {deref_store.typename()} access with field {field}",
            )

        return Error(
            self.location,
            f"invalid {store.typename()} access with field {field}",
        )


@final
@dataclass
class AstExpressionMkref(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        result = self.lhs.eval(env)
        if isinstance(result, Error):
            return result
        return Reference.new(result)


@final
@dataclass
class AstExpressionDeref(AstExpression):
    location: Optional[SourceLocation]
    lhs: AstExpression

    def eval(self, env: Environment) -> Union[Value, Error]:
        result = self.lhs.eval(env)
        if isinstance(result, Error):
            return result
        if not isinstance(result, Reference):
            return Error(
                self.location,
                f"attempted dereference of non-reference type {quote(typename(result))}",
            )
        return result.data


@final
@dataclass
class AstBlock(AstNode):
    location: Optional[SourceLocation]
    statements: list[AstStatement]

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        env = Environment(env)  # Blocks execute with a new lexical scope.
        for statement in self.statements:
            result = statement.eval(env)
            if result is not None:
                return result
        return None


@final
@dataclass
class AstConditional(AstNode):
    location: Optional[SourceLocation]
    condition: AstExpression
    body: AstBlock

    def exec(self, env: Environment) -> Tuple[Optional[ControlFlow], bool]:
        result = self.condition.eval(env)
        if isinstance(result, Error):
            return (result, False)
        if not isinstance(result, Boolean):
            return (
                Error(
                    self.location,
                    f"conditional with non-boolean type {quote(typename(result))}",
                ),
                False,
            )
        if result.data:
            return (self.body.eval(Environment(env)), True)
        return (None, False)

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        (result, executed) = self.exec(env)
        return result


@final
@dataclass
class AstStatementLet(AstStatement):
    location: Optional[SourceLocation]
    identifier: AstIdentifier
    expression: AstExpression

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        env.let(self.identifier.name, copy(result))
        return None


@final
@dataclass
class AstStatementFor(AstStatement):
    location: Optional[SourceLocation]
    identifier_k: AstIdentifier
    identifier_v: Optional[AstIdentifier]
    k_is_reference: bool
    v_is_reference: bool
    collection: AstExpression
    block: AstBlock

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        collection = self.collection.eval(env)
        if isinstance(collection, Error):
            return collection
        collection = copy(collection)

        loop_env = Environment(env)
        if metafunction := collection.metafunction(CONST_STRING_NEXT):
            if self.identifier_v is not None:
                return Error(
                    self.location,
                    f"attempted key-value iteration over iterator {quote(typename(collection))}",
                )
            if self.k_is_reference:
                return Error(
                    self.location,
                    f"cannot use a key-reference over iterator {quote(typename(collection))}",
                )
            reference = Reference.new(collection)
            while True:
                iterated = call(self.location, metafunction, [reference])
                if isinstance(iterated, Error):
                    if isinstance(iterated.value, Null):
                        break  # end-of-iteration
                    return iterated
                loop_env.let(self.identifier_k.name, iterated)
                result = self.block.eval(loop_env)
                if isinstance(result, Return):
                    return result
                if isinstance(result, Break):
                    return None
                if isinstance(result, Continue):
                    continue
                if isinstance(result, Error):
                    return result
        elif isinstance(collection, Number):
            if self.identifier_v is not None:
                return Error(
                    self.location,
                    f"attempted key-value iteration over type {quote(typename(collection))}",
                )
            if self.k_is_reference:
                return Error(
                    self.location,
                    f"cannot use a key-reference over type {quote(typename(collection))}",
                )
            if not float(collection.data).is_integer():
                return Error(
                    self.location,
                    f"attempted iteration over non-integer number {quote(collection)}",
                )
            for i in range(int(float(collection.data))):
                loop_env.let(self.identifier_k.name, Number.new(i))
                result = self.block.eval(loop_env)
                if isinstance(result, Return):
                    return result
                if isinstance(result, Break):
                    return None
                if isinstance(result, Continue):
                    continue
                if isinstance(result, Error):
                    return result
        elif isinstance(collection, Vector):
            if self.identifier_v is not None:
                return Error(
                    self.location,
                    f"attempted key-value iteration over type {quote(typename(collection))}",
                )
            # Iterate over a shallow copy of the vector data in order to allow
            # vector modification during iteration.
            for x in list(collection.data):
                loop_env.let(
                    self.identifier_k.name,
                    Reference.new(x) if self.k_is_reference else copy(x),
                )
                result = self.block.eval(loop_env)
                if isinstance(result, Return):
                    return result
                if isinstance(result, Break):
                    return None
                if isinstance(result, Continue):
                    continue
                if isinstance(result, Error):
                    return result
        elif isinstance(collection, Map):
            if self.k_is_reference:
                return Error(
                    self.location,
                    f"cannot use a key-reference over type {quote(typename(collection))}",
                )
            # Iterate over a shallow copy of the map data in order to allow map
            # modification during iteration.
            for k, v in dict(collection.data).items():
                loop_env.let(self.identifier_k.name, copy(k))
                if self.identifier_v is not None:
                    loop_env.let(
                        self.identifier_v.name,
                        Reference.new(v) if self.v_is_reference else copy(v),
                    )
                result = self.block.eval(loop_env)
                if isinstance(result, Return):
                    return result
                if isinstance(result, Break):
                    return None
                if isinstance(result, Continue):
                    continue
                if isinstance(result, Error):
                    return result
        elif isinstance(collection, Set):
            if self.identifier_v is not None:
                return Error(
                    self.location,
                    f"attempted key-value iteration over type {quote(typename(collection))}",
                )
            if self.k_is_reference:
                return Error(
                    self.location,
                    f"cannot use a key-reference over type {quote(typename(collection))}",
                )
            # Iterate over a shallow copy of the set data in order to allow set
            # modification during iteration.
            for x in dict(collection.data).keys():
                loop_env.let(self.identifier_k.name, copy(x))
                result = self.block.eval(loop_env)
                if isinstance(result, Return):
                    return result
                if isinstance(result, Break):
                    return None
                if isinstance(result, Continue):
                    continue
                if isinstance(result, Error):
                    return result
        else:
            return Error(
                self.location,
                f"attempted iteration over type {quote(typename(collection))}",
            )
        return None


@final
@dataclass
class AstStatementWhile(AstStatement):
    location: Optional[SourceLocation]
    expression: AstExpression
    block: AstBlock

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        while True:
            expression = self.expression.eval(env)
            if isinstance(expression, Error):
                return expression
            if not isinstance(expression, Boolean):
                return Error(
                    self.location,
                    f"conditional with non-boolean type {quote(typename(expression))}",
                )
            if not expression.data:
                break
            result = self.block.eval(Environment(env))
            if isinstance(result, Return):
                return result
            if isinstance(result, Break):
                return None
            if isinstance(result, Continue):
                continue
            if isinstance(result, Error):
                return result
        return None


@final
@dataclass
class AstStatementBreak(AstStatement):
    location: Optional[SourceLocation]

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        return Break(self.location)


@final
@dataclass
class AstStatementContinue(AstStatement):
    location: Optional[SourceLocation]

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        return Continue(self.location)


@final
@dataclass
class AstStatementIfElifElse(AstStatement):
    location: Optional[SourceLocation]
    conditionals: list[AstConditional]
    else_block: Optional[AstBlock]

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        for conditional in self.conditionals:
            (result, executed) = conditional.exec(env)
            if result is not None:
                return result
            if executed:
                return result
        if self.else_block is not None:
            return self.else_block.eval(env)
        return None


@final
@dataclass
class AstStatementTry(AstStatement):
    location: Optional[SourceLocation]
    try_block: AstBlock
    catch_identifier: Optional[AstIdentifier]
    catch_block: AstBlock

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        result = self.try_block.eval(env)
        if isinstance(result, Return):
            return result
        if isinstance(result, Break):
            return result
        if isinstance(result, Continue):
            return result
        if isinstance(result, Error):
            env = Environment(env)
            if self.catch_identifier is not None:
                env.let(self.catch_identifier.name, result.value)
            return self.catch_block.eval(env)
        return None


@final
@dataclass
class AstStatementError(AstStatement):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        return Error(self.location, result)


@final
@dataclass
class AstStatementReturn(AstStatement):
    location: Optional[SourceLocation]
    expression: Optional[AstExpression]

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        if self.expression is None:
            return Return(Null.new())
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        return Return(result)


@final
@dataclass
class AstStatementExpression(AstStatement):
    location: Optional[SourceLocation]
    expression: AstExpression

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        result = self.expression.eval(env)
        if isinstance(result, Error):
            return result
        return None


@final
@dataclass
class AstStatementAssignment(AstStatement):
    location: Optional[SourceLocation]
    lhs: AstExpression
    rhs: AstExpression

    def eval(self, env: Environment) -> Optional[ControlFlow]:
        store: Value
        field: Value

        if isinstance(self.lhs, AstExpressionIdentifier):
            lookup = env.lookup(self.lhs.name)
            if lookup is None:
                return Error(
                    self.location,
                    f"identifier {quote(self.lhs.name.runes)} is not defined",
                )
            store = lookup.store
            field = self.lhs.name
        elif isinstance(self.lhs, AstExpressionAccessIndex):
            lhs_store = self.lhs.store.eval(env)
            if isinstance(lhs_store, Error):
                return lhs_store
            lhs_field = self.lhs.field.eval(env)
            if isinstance(lhs_field, Error):
                return lhs_field
            store = lhs_store
            field = lhs_field
        elif isinstance(self.lhs, AstExpressionAccessDot):
            lhs_store = self.lhs.store.eval(env)
            if isinstance(lhs_store, Error):
                return lhs_store
            lhs_field = self.lhs.field.name
            store = lhs_store
            field = lhs_field
        elif isinstance(self.lhs, AstExpressionAccessScope):
            lhs_store = self.lhs.store.eval(env)
            if isinstance(lhs_store, Error):
                return lhs_store
            lhs_field = self.lhs.field.name
            store = lhs_store
            field = lhs_field
        else:
            return Error(self.location, "attempted assignment to non-lvalue")

        rhs = self.rhs.eval(env)
        if isinstance(rhs, Error):
            return rhs
        if isinstance(store, (Vector, Map)):
            try:
                store[field] = copy(rhs)
                return None
            except IndexError:
                return Error(
                    self.location,
                    f"invalid {store.typename()} access with index {field}",
                )
            except Exception as e:
                return Error(self.location, str(e))
        if isinstance(self.lhs, AstExpressionAccessDot) and isinstance(
            store, Reference
        ):
            deref_store = store.data
            try:
                deref_store[field] = copy(rhs)
                return None
            except (NotImplementedError, IndexError, KeyError):
                return Error(
                    self.location,
                    f"invalid {store.typename()} to {deref_store.typename()} access with field {field}",
                )
            except Exception as e:
                return Error(self.location, str(e))
        return Error(
            self.location,
            f"attempted access into type {quote(typename(store))} with type {quote(typename(field))}",
        )


class Precedence(enum.IntEnum):
    # fmt: off
    LOWEST  = enum.auto()
    OR      = enum.auto()  # or
    AND     = enum.auto()  # and
    COMPARE = enum.auto()  # == != <= >= < > =~ !~
    ADD_SUB = enum.auto()  # + -
    MUL_DIV = enum.auto()  # * /
    PREFIX  = enum.auto()  # +x -x
    POSTFIX = enum.auto()  # foo(bar, 123) foo[42] .& .*
    # fmt: on


class Parser:
    ParseNud = Callable[["Parser"], AstExpression]
    ParseLed = Callable[["Parser", AstExpression], AstExpression]

    PRECEDENCES: dict[TokenKind, Precedence] = {
        # fmt: off
        TokenKind.OR:       Precedence.OR,
        TokenKind.AND:      Precedence.AND,
        TokenKind.EQ:       Precedence.COMPARE,
        TokenKind.NE:       Precedence.COMPARE,
        TokenKind.LE:       Precedence.COMPARE,
        TokenKind.GE:       Precedence.COMPARE,
        TokenKind.LT:       Precedence.COMPARE,
        TokenKind.GT:       Precedence.COMPARE,
        TokenKind.EQ_RE:    Precedence.COMPARE,
        TokenKind.NE_RE:    Precedence.COMPARE,
        TokenKind.ADD:      Precedence.ADD_SUB,
        TokenKind.SUB:      Precedence.ADD_SUB,
        TokenKind.MUL:      Precedence.MUL_DIV,
        TokenKind.DIV:      Precedence.MUL_DIV,
        TokenKind.REM:      Precedence.MUL_DIV,
        TokenKind.LPAREN:   Precedence.POSTFIX,
        TokenKind.LBRACKET: Precedence.POSTFIX,
        TokenKind.DOT:      Precedence.POSTFIX,
        TokenKind.SCOPE:    Precedence.POSTFIX,
        TokenKind.MKREF:    Precedence.POSTFIX,
        TokenKind.DEREF:    Precedence.POSTFIX,
        # fmt: on
    }

    def __init__(self, lexer: Lexer):
        self.lexer: Lexer = lexer
        self.current_token: Token = Token(TokenKind.ILLEGAL, "DEFAULT CURRENT TOKEN")

        self._advance_token()

        self.parse_nud_functions: dict[TokenKind, Parser.ParseNud] = dict()
        self.parse_led_functions: dict[TokenKind, Parser.ParseLed] = dict()

        self._register_nud(TokenKind.IDENTIFIER, Parser.parse_expression_identifier)
        self._register_nud(TokenKind.TEMPLATE, Parser.parse_expression_template)
        self._register_nud(TokenKind.NULL, Parser.parse_expression_null)
        self._register_nud(TokenKind.TRUE, Parser.parse_expression_boolean)
        self._register_nud(TokenKind.FALSE, Parser.parse_expression_boolean)
        self._register_nud(TokenKind.NUMBER, Parser.parse_expression_number)
        self._register_nud(TokenKind.STRING, Parser.parse_expression_string)
        self._register_nud(TokenKind.REGEXP, Parser.parse_expression_regexp)
        self._register_nud(TokenKind.LBRACKET, Parser.parse_expression_vector)
        self._register_nud(TokenKind.MAP, Parser.parse_expression_map_or_set)
        self._register_nud(TokenKind.SET, Parser.parse_expression_map_or_set)
        self._register_nud(TokenKind.LBRACE, Parser.parse_expression_map_or_set)
        self._register_nud(TokenKind.FUNCTION, Parser.parse_expression_function)
        self._register_nud(TokenKind.TYPE, Parser.parse_expression_type)
        self._register_nud(TokenKind.NEW, Parser.parse_expression_new)
        self._register_nud(TokenKind.LPAREN, Parser.parse_expression_grouped)
        self._register_nud(TokenKind.ADD, Parser.parse_expression_positive)
        self._register_nud(TokenKind.SUB, Parser.parse_expression_negative)
        self._register_nud(TokenKind.NOT, Parser.parse_expression_not)

        self._register_led(TokenKind.AND, Parser.parse_expression_and)
        self._register_led(TokenKind.OR, Parser.parse_expression_or)
        self._register_led(TokenKind.EQ, Parser.parse_expression_eq)
        self._register_led(TokenKind.NE, Parser.parse_expression_ne)
        self._register_led(TokenKind.LE, Parser.parse_expression_le)
        self._register_led(TokenKind.GE, Parser.parse_expression_ge)
        self._register_led(TokenKind.LT, Parser.parse_expression_lt)
        self._register_led(TokenKind.GT, Parser.parse_expression_gt)
        self._register_led(TokenKind.EQ_RE, Parser.parse_expression_eq_re)
        self._register_led(TokenKind.NE_RE, Parser.parse_expression_ne_re)
        self._register_led(TokenKind.ADD, Parser.parse_expression_add)
        self._register_led(TokenKind.SUB, Parser.parse_expression_sub)
        self._register_led(TokenKind.MUL, Parser.parse_expression_mul)
        self._register_led(TokenKind.DIV, Parser.parse_expression_div)
        self._register_led(TokenKind.REM, Parser.parse_expression_rem)
        self._register_led(TokenKind.LPAREN, Parser.parse_expression_function_call)
        self._register_led(TokenKind.LBRACKET, Parser.parse_expression_access_index)
        self._register_led(TokenKind.DOT, Parser.parse_expression_access_dot)
        self._register_led(TokenKind.SCOPE, Parser.parse_expression_access_scope)
        self._register_led(TokenKind.MKREF, Parser.parse_expression_mkref)
        self._register_led(TokenKind.DEREF, Parser.parse_expression_deref)

    def _register_nud(self, kind: TokenKind, parse: "Parser.ParseNud") -> None:
        self.parse_nud_functions[kind] = parse

    def _register_led(self, kind: TokenKind, parse: "Parser.ParseLed") -> None:
        self.parse_led_functions[kind] = parse

    def _advance_token(self) -> Token:
        current_token = self.current_token
        self.current_token = self.lexer.next_token()
        return current_token

    def _check_current(self, kind: TokenKind) -> bool:
        return self.current_token.kind == kind

    def _expect_current(self, kind: TokenKind) -> Token:
        current = self.current_token
        if current.kind != kind:
            raise ParseError(
                current.location, f"expected {quote(kind)}, found {quote(current)}"
            )
        self._advance_token()
        return current

    def parse_program(self) -> AstProgram:
        location = self.current_token.location
        statements: list[AstStatement] = list()
        while not self._check_current(TokenKind.EOF):
            statements.append(self.parse_statement())
        return AstProgram(location, statements)

    def parse_identifier(self) -> AstIdentifier:
        token = self._expect_current(TokenKind.IDENTIFIER)
        return AstIdentifier(token.location, String(token.literal))

    def parse_expression(
        self, precedence: Precedence = Precedence.LOWEST
    ) -> AstExpression:
        def get_precedence(kind: TokenKind) -> Precedence:
            return Parser.PRECEDENCES.get(kind, Precedence.LOWEST)

        parse_nud = self.parse_nud_functions.get(self.current_token.kind)
        if parse_nud is None:
            raise ParseError(
                self.current_token.location,
                f"expected expression, found {self.current_token}",
            )
        expression = parse_nud(self)
        while precedence < get_precedence(self.current_token.kind):
            parse_led = self.parse_led_functions.get(self.current_token.kind, None)
            if parse_led is None:
                return expression
            expression = parse_led(self, expression)
        return expression

    def parse_expression_identifier(self) -> AstExpressionIdentifier:
        token = self._expect_current(TokenKind.IDENTIFIER)
        return AstExpressionIdentifier(token.location, String(token.literal))

    def parse_expression_template(self) -> AstExpressionTemplate:
        token = self._expect_current(TokenKind.TEMPLATE)
        assert token.template is not None
        return AstExpressionTemplate(token.location, token.template)

    def parse_expression_null(self) -> AstExpressionNull:
        location = self._expect_current(TokenKind.NULL).location
        return AstExpressionNull(location, Null.new())

    def parse_expression_boolean(self) -> AstExpressionBoolean:
        if self._check_current(TokenKind.TRUE):
            location = self._expect_current(TokenKind.TRUE).location
            return AstExpressionBoolean(location, Boolean.new(True))
        if self._check_current(TokenKind.FALSE):
            location = self._expect_current(TokenKind.FALSE).location
            return AstExpressionBoolean(location, Boolean.new(False))
        raise ParseError(
            self.current_token.location,
            f"expected boolean, found {self.current_token}",
        )

    def parse_expression_number(self) -> AstExpressionNumber:
        token = self._expect_current(TokenKind.NUMBER)
        assert token.number is not None
        return AstExpressionNumber(token.location, Number.new(token.number))

    def parse_expression_string(self) -> AstExpressionString:
        token = self._expect_current(TokenKind.STRING)
        assert token.string is not None
        return AstExpressionString(token.location, String.new(token.string))

    def parse_expression_regexp(self) -> AstExpressionRegexp:
        token = self._expect_current(TokenKind.REGEXP)
        assert token.string is not None
        try:
            pattern = re2.compile(token.string)
        except Exception as e:
            raise ParseError(self.current_token.location, str(e))
        return AstExpressionRegexp(token.location, Regexp.new(token.string, pattern))

    def parse_expression_vector(self) -> AstExpressionVector:
        location = self._expect_current(TokenKind.LBRACKET).location
        elements: list[AstExpression] = list()
        while not self._check_current(TokenKind.RBRACKET):
            if len(elements) != 0:
                self._expect_current(TokenKind.COMMA)
            if self._check_current(TokenKind.RBRACKET):
                break
            elements.append(self.parse_expression())
        self._expect_current(TokenKind.RBRACKET)
        return AstExpressionVector(location, elements)

    def parse_expression_map_or_set(self) -> Union[AstExpressionMap, AstExpressionSet]:
        ParseMapOrSet = enum.Enum("ParseMapOrSet", ["UNKNOWN", "MAP", "SET"])
        map_or_set = ParseMapOrSet.UNKNOWN
        if self._check_current(TokenKind.MAP):
            map_or_set = ParseMapOrSet.MAP
            self._advance_token()
        elif self._check_current(TokenKind.SET):
            map_or_set = ParseMapOrSet.SET
            self._advance_token()
        map_elements: list[Tuple[AstExpression, AstExpression]] = list()
        set_elements: list[AstExpression] = list()

        location = self._expect_current(TokenKind.LBRACE).location
        while not self._check_current(TokenKind.RBRACE):
            if len(map_elements) != 0 or len(set_elements):
                self._expect_current(TokenKind.COMMA)
            if self._check_current(TokenKind.RBRACE):
                break

            if self._check_current(TokenKind.DOT):
                if map_or_set == ParseMapOrSet.UNKNOWN:
                    map_or_set = ParseMapOrSet.MAP
                if map_or_set == ParseMapOrSet.SET:
                    raise ParseError(
                        self.current_token.location,
                        f"expected expression, found {self.current_token}",
                    )
                assert map_or_set == ParseMapOrSet.MAP
                self._expect_current(TokenKind.DOT)
                identifier = self.parse_expression_identifier()
                expression: AstExpression = AstExpressionString(
                    identifier.location, identifier.name
                )
            else:
                expression = self.parse_expression()

            if map_or_set == ParseMapOrSet.UNKNOWN:
                if self._check_current(TokenKind.COLON) or self._check_current(
                    TokenKind.ASSIGN
                ):
                    map_or_set = ParseMapOrSet.MAP
                else:
                    map_or_set = ParseMapOrSet.SET

            assert map_or_set != ParseMapOrSet.UNKNOWN
            match map_or_set:
                case ParseMapOrSet.MAP:
                    if self._check_current(TokenKind.COLON):
                        self._expect_current(TokenKind.COLON)
                    elif self._check_current(TokenKind.ASSIGN):
                        self._expect_current(TokenKind.ASSIGN)
                    else:
                        raise ParseError(
                            self.current_token.location,
                            f"expected {TokenKind.COLON} or {TokenKind.ASSIGN}, found {self.current_token}",
                        )
                    map_elements.append((expression, self.parse_expression()))
                case ParseMapOrSet.SET:
                    set_elements.append(expression)

        self._expect_current(TokenKind.RBRACE)
        match map_or_set:
            case ParseMapOrSet.UNKNOWN:
                raise ParseError(location, "ambiguous empty map or set")
            case ParseMapOrSet.MAP:
                result = AstExpressionMap(location, map_elements)
                update_named_functions(result)
                return result
            case ParseMapOrSet.SET:
                return AstExpressionSet(location, set_elements)

    def parse_expression_function(self) -> AstExpressionFunction:
        location = self._expect_current(TokenKind.FUNCTION).location
        parameters: list[AstIdentifier] = list()
        self._expect_current(TokenKind.LPAREN)
        while not self._check_current(TokenKind.RPAREN):
            if len(parameters) != 0:
                self._expect_current(TokenKind.COMMA)
            parameters.append(self.parse_identifier())
        self._expect_current(TokenKind.RPAREN)
        body = self.parse_block()
        for i in range(len(parameters)):
            for j in range(i + 1, len(parameters)):
                if parameters[i].name == parameters[j].name:
                    raise ParseError(
                        parameters[j].location,
                        f"duplicate function paramter {quote(parameters[i].name.runes)}",
                    )
        return AstExpressionFunction(location, parameters, body)

    def parse_expression_grouped(self) -> AstExpressionGrouped:
        location = self._expect_current(TokenKind.LPAREN).location
        expression = self.parse_expression()
        self._expect_current(TokenKind.RPAREN)
        return AstExpressionGrouped(location, expression)

    def parse_expression_type(self) -> AstExpressionType:
        location = self._expect_current(TokenKind.TYPE).location
        expression = self.parse_expression()
        return AstExpressionType(location, String.new("type@[{location}]"), expression)

    def parse_expression_new(self) -> AstExpressionNew:
        location = self._expect_current(TokenKind.NEW).location
        meta = self.parse_expression()
        expression = self.parse_expression()
        return AstExpressionNew(location, meta, expression)

    def parse_expression_positive(self) -> AstExpressionPositive:
        location = self._expect_current(TokenKind.ADD).location
        expression = self.parse_expression(Precedence.PREFIX)
        return AstExpressionPositive(location, expression)

    def parse_expression_negative(self) -> AstExpressionNegative:
        location = self._expect_current(TokenKind.SUB).location
        expression = self.parse_expression(Precedence.PREFIX)
        return AstExpressionNegative(location, expression)

    def parse_expression_not(self) -> AstExpressionNot:
        location = self._expect_current(TokenKind.NOT).location
        expression = self.parse_expression(Precedence.PREFIX)
        return AstExpressionNot(location, expression)

    def parse_expression_and(self, lhs: AstExpression) -> AstExpressionAnd:
        location = self._expect_current(TokenKind.AND).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.AND])
        return AstExpressionAnd(location, lhs, rhs)

    def parse_expression_or(self, lhs: AstExpression) -> AstExpressionOr:
        location = self._expect_current(TokenKind.OR).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.OR])
        return AstExpressionOr(location, lhs, rhs)

    def parse_expression_eq(self, lhs: AstExpression) -> AstExpressionEq:
        location = self._expect_current(TokenKind.EQ).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.EQ])
        return AstExpressionEq(location, lhs, rhs)

    def parse_expression_ne(self, lhs: AstExpression) -> AstExpressionNe:
        location = self._expect_current(TokenKind.NE).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.NE])
        return AstExpressionNe(location, lhs, rhs)

    def parse_expression_le(self, lhs: AstExpression) -> AstExpressionLe:
        location = self._expect_current(TokenKind.LE).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.LE])
        return AstExpressionLe(location, lhs, rhs)

    def parse_expression_ge(self, lhs: AstExpression) -> AstExpressionGe:
        location = self._expect_current(TokenKind.GE).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.GE])
        return AstExpressionGe(location, lhs, rhs)

    def parse_expression_lt(self, lhs: AstExpression) -> AstExpressionLt:
        location = self._expect_current(TokenKind.LT).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.LT])
        return AstExpressionLt(location, lhs, rhs)

    def parse_expression_gt(self, lhs: AstExpression) -> AstExpressionGt:
        location = self._expect_current(TokenKind.GT).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.GT])
        return AstExpressionGt(location, lhs, rhs)

    def parse_expression_eq_re(self, lhs: AstExpression) -> AstExpressionEqRe:
        location = self._expect_current(TokenKind.EQ_RE).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.EQ_RE])
        return AstExpressionEqRe(location, lhs, rhs)

    def parse_expression_ne_re(self, lhs: AstExpression) -> AstExpressionNeRe:
        location = self._expect_current(TokenKind.NE_RE).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.NE_RE])
        return AstExpressionNeRe(location, lhs, rhs)

    def parse_expression_add(self, lhs: AstExpression) -> AstExpressionAdd:
        location = self._expect_current(TokenKind.ADD).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.ADD])
        return AstExpressionAdd(location, lhs, rhs)

    def parse_expression_sub(self, lhs: AstExpression) -> AstExpressionSub:
        location = self._expect_current(TokenKind.SUB).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.SUB])
        return AstExpressionSub(location, lhs, rhs)

    def parse_expression_mul(self, lhs: AstExpression) -> AstExpressionMul:
        location = self._expect_current(TokenKind.MUL).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.MUL])
        return AstExpressionMul(location, lhs, rhs)

    def parse_expression_div(self, lhs: AstExpression) -> AstExpressionDiv:
        location = self._expect_current(TokenKind.DIV).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.DIV])
        return AstExpressionDiv(location, lhs, rhs)

    def parse_expression_rem(self, lhs: AstExpression) -> AstExpressionRem:
        location = self._expect_current(TokenKind.REM).location
        rhs = self.parse_expression(Parser.PRECEDENCES[TokenKind.REM])
        return AstExpressionRem(location, lhs, rhs)

    def parse_expression_function_call(
        self, lhs: AstExpression
    ) -> AstExpressionFunctionCall:
        location = self._expect_current(TokenKind.LPAREN).location
        arguments: list[AstExpression] = list()
        while not self._check_current(TokenKind.RPAREN):
            if len(arguments) != 0:
                self._expect_current(TokenKind.COMMA)
            if self._check_current(TokenKind.RPAREN):
                break
            arguments.append(self.parse_expression())
        self._expect_current(TokenKind.RPAREN)
        return AstExpressionFunctionCall(location, lhs, arguments)

    def parse_expression_access_index(
        self, lhs: AstExpression
    ) -> AstExpressionAccessIndex:
        location = self._expect_current(TokenKind.LBRACKET).location
        field = self.parse_expression()
        self._expect_current(TokenKind.RBRACKET)
        return AstExpressionAccessIndex(location, lhs, field)

    def parse_expression_access_dot(self, lhs: AstExpression) -> AstExpressionAccessDot:
        location = self._expect_current(TokenKind.DOT).location
        field = self.parse_identifier()
        return AstExpressionAccessDot(location, lhs, field)

    def parse_expression_access_scope(
        self, lhs: AstExpression
    ) -> AstExpressionAccessScope:
        location = self._expect_current(TokenKind.SCOPE).location
        field = self.parse_identifier()
        return AstExpressionAccessScope(location, lhs, field)

    def parse_expression_mkref(self, lhs: AstExpression) -> AstExpressionMkref:
        location = self._expect_current(TokenKind.MKREF).location
        return AstExpressionMkref(location, lhs)

    def parse_expression_deref(self, lhs: AstExpression) -> AstExpressionDeref:
        location = self._expect_current(TokenKind.DEREF).location
        return AstExpressionDeref(location, lhs)

    def parse_block(self) -> AstBlock:
        location = self._expect_current(TokenKind.LBRACE).location
        statements: list[AstStatement] = list()
        while not self._check_current(TokenKind.RBRACE):
            statements.append(self.parse_statement())
        self._expect_current(TokenKind.RBRACE)
        return AstBlock(location, statements)

    def parse_statement(self) -> AstStatement:
        if self._check_current(TokenKind.LET):
            return self.parse_statement_let()
        if self._check_current(TokenKind.IF):
            return self.parse_statement_if_elif_else()
        if self._check_current(TokenKind.FOR):
            return self.parse_statement_for()
        if self._check_current(TokenKind.WHILE):
            return self.parse_statement_while()
        if self._check_current(TokenKind.BREAK):
            return self.parse_statement_break()
        if self._check_current(TokenKind.CONTINUE):
            return self.parse_statement_continue()
        if self._check_current(TokenKind.TRY):
            return self.parse_statement_try()
        if self._check_current(TokenKind.ERROR):
            return self.parse_statement_error()
        if self._check_current(TokenKind.RETURN):
            return self.parse_statement_return()
        return self.parse_statement_expression_or_assignment()

    def parse_statement_let(self) -> AstStatementLet:
        location = self._expect_current(TokenKind.LET).location
        identifier = self.parse_identifier()
        self._expect_current(TokenKind.ASSIGN)
        expression = self.parse_expression()
        self._expect_current(TokenKind.SEMICOLON)
        if isinstance(expression, AstExpressionFunction):
            expression.name = identifier.name
        if isinstance(expression, AstExpressionType):
            expression.name = identifier.name
        if isinstance(expression, AstExpressionMap):
            update_named_functions(
                expression,
                identifier.name.bytes + str(TokenKind.SCOPE).encode("utf-8"),
            )
        if isinstance(expression, AstExpressionType) and isinstance(
            expression.expression, AstExpressionMap
        ):
            update_named_functions(
                expression.expression,
                identifier.name.bytes + str(TokenKind.SCOPE).encode("utf-8"),
            )
        return AstStatementLet(location, identifier, expression)

    def parse_statement_if_elif_else(self) -> AstStatementIfElifElse:
        assert self.current_token.kind == TokenKind.IF
        location = self.current_token.location

        def parse_conditional() -> AstConditional:
            location = self._advance_token().location
            condition = self.parse_expression()
            body = self.parse_block()
            return AstConditional(location, condition, body)

        conditionals: list[AstConditional] = list()
        while self._check_current(
            TokenKind.ELIF if len(conditionals) else TokenKind.IF
        ):
            conditionals.append(parse_conditional())
        if self._check_current(TokenKind.ELSE):
            self._expect_current(TokenKind.ELSE)
            else_block = self.parse_block()
        else:
            else_block = None

        return AstStatementIfElifElse(location, conditionals, else_block)

    def parse_statement_try(self) -> AstStatementTry:
        location = self._expect_current(TokenKind.TRY).location
        try_block = self.parse_block()
        self._expect_current(TokenKind.CATCH)
        if self._check_current(TokenKind.IDENTIFIER):
            catch_identifier = self.parse_identifier()
        else:
            catch_identifier = None
        catch_block = self.parse_block()
        return AstStatementTry(location, try_block, catch_identifier, catch_block)

    def parse_statement_error(self) -> AstStatementError:
        location = self._expect_current(TokenKind.ERROR).location
        expression = self.parse_expression()
        self._expect_current(TokenKind.SEMICOLON)
        return AstStatementError(location, expression)

    def parse_statement_for(self) -> AstStatementFor:
        location = self._expect_current(TokenKind.FOR).location
        identifier_k = self.parse_identifier()
        k_is_reference = False
        v_is_reference = False
        if self._check_current(TokenKind.MKREF):
            self._expect_current(TokenKind.MKREF)
            k_is_reference = True
        identifier_v: Optional[AstIdentifier] = None
        if self._check_current(TokenKind.COMMA):
            self._expect_current(TokenKind.COMMA)
            identifier_v = self.parse_identifier()
            if self._check_current(TokenKind.MKREF):
                self._expect_current(TokenKind.MKREF)
                v_is_reference = True
        self._expect_current(TokenKind.IN)
        collection = self.parse_expression()
        block = self.parse_block()
        if identifier_v is not None and identifier_k.name == identifier_v.name:
            raise ParseError(
                identifier_k.location,
                f"duplicate iterator name {quote(identifier_k.name.runes)}",
            )
        return AstStatementFor(
            location,
            identifier_k,
            identifier_v,
            k_is_reference,
            v_is_reference,
            collection,
            block,
        )

    def parse_statement_while(self) -> AstStatementWhile:
        location = self._expect_current(TokenKind.WHILE).location
        expression = self.parse_expression()
        block = self.parse_block()
        return AstStatementWhile(location, expression, block)

    def parse_statement_break(self) -> AstStatementBreak:
        location = self._expect_current(TokenKind.BREAK).location
        self._expect_current(TokenKind.SEMICOLON)
        return AstStatementBreak(location)

    def parse_statement_continue(self) -> AstStatementContinue:
        location = self._expect_current(TokenKind.CONTINUE).location
        self._expect_current(TokenKind.SEMICOLON)
        return AstStatementContinue(location)

    def parse_statement_return(self) -> AstStatementReturn:
        location = self._expect_current(TokenKind.RETURN).location
        expression: Optional[AstExpression] = None
        if not self._check_current(TokenKind.SEMICOLON):
            expression = self.parse_expression()
        self._expect_current(TokenKind.SEMICOLON)
        return AstStatementReturn(location, expression)

    def parse_statement_expression_or_assignment(
        self,
    ) -> Union[AstStatementExpression, AstStatementAssignment]:
        expression = self.parse_expression()
        if not self._check_current(TokenKind.ASSIGN):
            self._expect_current(TokenKind.SEMICOLON)
            return AstStatementExpression(expression.location, expression)
        location = self._expect_current(TokenKind.ASSIGN).location
        rhs = self.parse_expression()
        self._expect_current(TokenKind.SEMICOLON)
        return AstStatementAssignment(location, expression, rhs)


def call(
    location: Optional[SourceLocation],
    function: Union[Function, Builtin],
    arguments: list[Value],
) -> Union[Value, Error]:
    if isinstance(function, Builtin):
        produced = function.call(arguments)
        if isinstance(produced, Error):
            produced.trace.append(Error.TraceElement(location, function))
        return produced
    assert isinstance(function, Function)
    if len(arguments) != len(function.ast.parameters):
        return Error(
            location,
            f"invalid function argument count (expected {len(function.ast.parameters)}, received {len(arguments)})",
        )
    env = Environment(function.env)
    for i in range(len(function.ast.parameters)):
        env.let(function.ast.parameters[i].name, arguments[i])
    result = function.ast.body.eval(env)
    if isinstance(result, Return):
        return result.value
    if isinstance(result, Break):
        return Error(result.location, "attempted to break outside of a loop")
    if isinstance(result, Continue):
        return Error(result.location, "attempted to continue outside of a loop")
    if isinstance(result, Error):
        result.trace.append(Error.TraceElement(location, function))
        return result
    return Null.new()


# Used to indicate reference arguments when defining builtin functions.
@dataclass
class ReferenceType:
    type: Type[Value]


# ReferenceTo(TYPE)
def ReferenceTo(type: Type[Value]) -> ReferenceType:
    return ReferenceType(type)


# @builtin("vector::slice", [ReferenceTo(Vector), Number, Number])
# def builtin_vector_slice(
#     self: Reference, vector: Vector, bgn: Number, end: Number
# ) -> Union[Value, Error]: ...
def builtin(nameof: str, args: Optional[list] = None):
    def decorator(func: Callable) -> Type[Builtin]:
        class GeneratedBuiltin(Builtin):
            name = nameof

            def function(self, arguments: list[Value]) -> Union[Value, Error]:
                if args is not None:
                    Builtin.expect_argument_count(arguments, len(args))

                processed_args = []
                for i, arg_type in enumerate(args or []):
                    try:
                        # Special case of: ReferenceTo(Type)
                        if isinstance(arg_type, ReferenceType):
                            ref, data = Builtin.typed_argument_reference(
                                arguments, i, arg_type.type
                            )
                            processed_args.extend([ref, data])
                            continue
                        # Nominal case of: Type
                        value = Builtin.typed_argument(arguments, i, arg_type)
                        processed_args.append(value)
                    except Exception as e:
                        return Error(None, str(e))

                return func(*processed_args)

        GeneratedBuiltin.__name__ = f"Builtin_{func.__name__}"
        return GeneratedBuiltin

    return decorator


# @builtin_from_source("min")
# def builtin_min():
#     return """
#     let min = function(a, b) {
#         ...
#     };
#     return min;
#     """
def builtin_from_source(nameof: str):
    def decorator(func: Callable[[], str]) -> Callable[..., BuiltinFromSource]:
        class GeneratedBuiltinFromSource(BuiltinFromSource):
            name = nameof

            @staticmethod
            def source() -> str:
                return func()

        GeneratedBuiltinFromSource.__name__ = f"Builtin_{func.__name__}"

        # Factory function that is responsible for actually creating the
        # builtin from source instance. The factory is given the builtin name
        # for clarity when debugging.
        def factory(
            env: Optional[Environment] = None,
            evaluated: Union[Function, Builtin] = BuiltinImplicitUninitialized(),
        ) -> BuiltinFromSource:
            return GeneratedBuiltinFromSource(env=env, evaluated=evaluated)

        factory.__name__ = func.__name__
        return factory

    return decorator


@builtin("boolean::init", [Value])
def builtin_boolean_init(value: Value) -> Union[Value, Error]:
    if isinstance(value, Boolean):
        return Boolean.new(value.data)
    if isinstance(value, Number):
        underlying = float(value.data)
        return Boolean.new(not (math.isnan(underlying) or underlying == 0))
    if isinstance(value, String) and value.bytes == b"true":
        return Boolean.new(True)
    if isinstance(value, String) and value.bytes == b"false":
        return Boolean.new(False)
    return Error(None, f"cannot convert value {value} to boolean")


@builtin("number::init", [Value])
def builtin_number_init(value: Value) -> Union[Value, Error]:
    if isinstance(value, Number):
        return Number.new(float(value.data))
    if isinstance(value, Boolean):
        return Number.new(1 if value.data else 0)
    if isinstance(value, String):
        try:
            data = value.runes
            if data.startswith("+"):
                sign = +1
                data = data[1:]
            elif data.startswith("-"):
                sign = -1
                data = data[1:]
            else:
                sign = +1

            if data == "Inf":
                return Number.new(sign * math.inf)
            if data == "NaN":
                return Number.new(sign * math.nan)
            match_hex = Lexer.RE_NUMBER_HEX.fullmatch(data)
            match_dec = Lexer.RE_NUMBER_DEC.fullmatch(data)
            if match_hex is not None or match_dec is not None:
                return Number.new(sign * float(data))
        except ValueError:
            # Fallthough to end-of-function error case.
            pass
    return Error(None, f"cannot convert value {value} to number")


@builtin("number::is_nan", [ReferenceTo(Number)])
def builtin_number_is_nan(self: Reference, number: Number) -> Union[Value, Error]:
    return Boolean.new(math.isnan(number.data))


@builtin("number::is_inf", [ReferenceTo(Number)])
def builtin_number_is_inf(self: Reference, number: Number) -> Union[Value, Error]:
    return Boolean.new(math.isinf(number.data))


@builtin("number::is_integer", [ReferenceTo(Number)])
def builtin_number_is_integer(self: Reference, number: Number) -> Union[Value, Error]:
    return Boolean.new(float(number.data).is_integer())


@builtin("number::fixed", [ReferenceTo(Number), Number])
def builtin_number_fixed(
    self: Reference, number: Number, precision: Number
) -> Union[Value, Error]:
    if not float(precision.data).is_integer() or int(float(precision.data)) < 0:
        return Error(None, f"expected non-negative integer, received {precision}")
    return Number.new(round(float(number.data), ndigits=int(float(precision.data))))


@builtin("number::trunc", [ReferenceTo(Number)])
def builtin_number_trunc(self: Reference, number: Number) -> Union[Value, Error]:
    return Number.new(math.trunc(float(number.data)))


@builtin("number::round", [ReferenceTo(Number)])
def builtin_number_round(self: Reference, number: Number) -> Union[Value, Error]:
    return Number.new(round(float(number.data)))


@builtin("number::floor", [ReferenceTo(Number)])
def builtin_number_floor(self: Reference, number: Number) -> Union[Value, Error]:
    return Number.new(math.floor(float(number.data)))


@builtin("number::ceil", [ReferenceTo(Number)])
def builtin_number_ceil(self: Reference, number: Number) -> Union[Value, Error]:
    return Number.new(math.ceil(float(number.data)))


@builtin("string::init", [Value])
def builtin_string_init(value: Value) -> Union[Value, Error]:
    metafunction = value.metafunction(CONST_STRING_INTO_STRING)
    if metafunction is not None:
        result = call(None, metafunction, [Reference.new(value)])
        if isinstance(result, Error):
            return result
        if not isinstance(result, String):
            return Error(
                None,
                f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
            )
        return result
    if isinstance(value, String):
        return String.new(value.bytes)
    return String.new(str(value))


@builtin("string::bytes", [ReferenceTo(String)])
def builtin_string_bytes(self: Reference, string: String) -> Union[Value, Error]:
    return Vector.new([String.new(bytes([byte])) for byte in string.bytes])


@builtin("string::runes", [ReferenceTo(String)])
def builtin_string_runes(self: Reference, string: String) -> Union[Value, Error]:
    return Vector.new([String.new(rune) for rune in string.runes])


@builtin("string::count", [ReferenceTo(String)])
def builtin_string_count(self: Reference, string: String) -> Union[Value, Error]:
    return Number.new(len(string.bytes))


@builtin("string::contains", [ReferenceTo(String), String])
def builtin_string_contains(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    return Boolean.new(target.bytes in string.bytes)


@builtin("string::starts_with", [ReferenceTo(String), String])
def builtin_string_starts_with(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    return Boolean.new(string.bytes.startswith(target.bytes))


@builtin("string::ends_with", [ReferenceTo(String), String])
def builtin_string_ends_with(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    return Boolean.new(string.bytes.endswith(target.bytes))


@builtin("string::trim", [ReferenceTo(String)])
def builtin_string_trim(self: Reference, string: String) -> Union[Value, Error]:
    return String.new(string.bytes.strip())


@builtin("string::find", [ReferenceTo(String), String])
def builtin_string_find(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    found = string.bytes.find(target.bytes)
    if found == -1:
        return Null.new()
    return Number.new(found)


@builtin("string::rfind", [ReferenceTo(String), String])
def builtin_string_rfind(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    found = string.bytes.rfind(target.bytes)
    if found == -1:
        return Null.new()
    return Number.new(found)


@builtin("string::slice", [ReferenceTo(String), Number, Number])
def builtin_string_slice(
    self: Reference, string: String, bgn: Number, end: Number
) -> Union[Value, Error]:
    if not float(bgn.data).is_integer():
        return Error(None, f"expected integer index, received {bgn}")
    if not float(end.data).is_integer():
        return Error(None, f"expected integer index, received {end}")
    bgn_index = int(float(bgn.data))
    end_index = int(float(end.data))
    if bgn_index < 0:
        return Error(None, "slice begin is less than zero")
    if bgn_index > len(string.bytes):
        return Error(None, "slice begin is greater than the string length")
    if end_index < 0:
        return Error(None, "slice end is less than zero")
    if end_index > len(string.bytes):
        return Error(None, "slice end is greater than the string length")
    if end_index < bgn_index:
        return Error(None, "slice end is less than slice begin")
    return String.new(string.bytes[bgn_index:end_index])


@builtin("string::split", [ReferenceTo(String), String])
def builtin_string_split(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    if len(target.bytes) == 0:
        return Vector.new([String.new(x.to_bytes()) for x in string.bytes])
    split = string.bytes.split(target.bytes)
    return Vector.new([String.new(x) for x in split])


@builtin("string::join", [ReferenceTo(String), Vector])
def builtin_string_join(
    self: Reference, string: String, vector: Vector
) -> Union[Value, Error]:
    data = bytes()
    for index, value in enumerate(vector.data):
        if not isinstance(value, String):
            return Error(
                None,
                f"expected string-like value for vector element at index {index}, received {typename(value)}",
            )
        if index != 0:
            data += string.bytes
        data += value.bytes
    return String.new(data)


@builtin("string::cut", [ReferenceTo(String), String])
def builtin_string_cut(
    self: Reference, string: String, target: String
) -> Union[Value, Error]:
    found = string.bytes.find(target.bytes)
    if found == -1:
        return Null.new()
    prefix = String.new(string.bytes[0:found])
    suffix = String.new(string.bytes[found + len(target.bytes) :])
    return Map.new(
        {
            String.new("prefix"): prefix,
            String.new("suffix"): suffix,
        }
    )


@builtin("string::replace", [ReferenceTo(String), String, String])
def builtin_string_replace(
    self: Reference, string: String, target: String, replacement: String
) -> Union[Value, Error]:
    return String.new(string.bytes.replace(target.bytes, replacement.bytes))


@builtin("string::to_title", [ReferenceTo(String)])
def builtin_string_to_title(self: Reference, string: String) -> Union[Value, Error]:
    return String.new(string.runes.title())


@builtin("string::to_upper", [ReferenceTo(String)])
def builtin_string_to_upper(self: Reference, string: String) -> Union[Value, Error]:
    return String.new(string.runes.upper())


@builtin("string::to_lower", [ReferenceTo(String)])
def builtin_string_to_lower(self: Reference, string: String) -> Union[Value, Error]:
    return String.new(string.runes.lower())


@builtin("vector::init", [Value])
def builtin_vector_init(value: Value) -> Union[Value, Error]:
    if metafunction := value.metafunction(CONST_STRING_NEXT):
        reference = Reference.new(value)
        elements: list[Value] = list()
        while True:
            iterated = call(None, metafunction, [reference])
            if isinstance(iterated, Error):
                if isinstance(iterated.value, Null):
                    break  # end-of-iteration
                return iterated
            elements.append(iterated)
        return Vector.new([copy(x) for x in elements])
    if isinstance(value, Vector):
        return Vector.new([copy(x) for x in value.data])
    if isinstance(value, Map):
        return Vector.new(
            [Vector.new([copy(k), copy(v)]) for k, v in value.data.items()]
        )
    if isinstance(value, Set):
        return Vector.new([copy(x) for x in value.data])
    return Error(None, f"cannot convert value {value} to vector")


@builtin("vector::count", [ReferenceTo(Vector)])
def builtin_vector_count(self: Reference, vector: Vector) -> Union[Value, Error]:
    return Number.new(len(vector.data))


@builtin("vector::contains", [ReferenceTo(Vector), Value])
def builtin_vector_contains(
    self: Reference, vector: Vector, target: Value
) -> Union[Value, Error]:
    return Boolean.new(target in vector)


@builtin("vector::find", [ReferenceTo(Vector), Value])
def builtin_vector_find(
    self: Reference, vector: Vector, target: Value
) -> Union[Value, Error]:
    for index, value in enumerate(vector.data):
        if value == target:
            return Number.new(index)
    return Null.new()


@builtin("vector::rfind", [ReferenceTo(Vector), Value])
def builtin_vector_rfind(
    self: Reference, vector: Vector, target: Value
) -> Union[Value, Error]:
    for index, value in reversed(list(enumerate(vector.data))):
        if value == target:
            return Number.new(index)
    return Null.new()


@builtin("vector::push", [ReferenceTo(Vector), Value])
def builtin_vector_push(
    self: Reference, vector: Vector, value: Value
) -> Union[Value, Error]:
    if vector.data.uses > 1:
        vector.cow()  # copy-on-write
    vector.data.append(value)
    return Null.new()


@builtin("vector::pop", [ReferenceTo(Vector)])
def builtin_vector_pop(self: Reference, vector: Vector) -> Union[Value, Error]:
    if vector.data.uses > 1:
        vector.cow()  # copy-on-write
    try:
        return copy(vector.data.pop())
    except IndexError:
        return Error(None, "attempted vector::pop on an empty vector")


@builtin("vector::insert", [ReferenceTo(Vector), Number, Value])
def builtin_vector_insert(
    self: Reference, vector: Vector, index: Number, value: Value
) -> Union[Value, Error]:
    if not float(index.data).is_integer():
        return Error(None, f"expected integer index, received {index}")
    if vector.data.uses > 1:
        vector.cow()  # copy-on-write
    vector.data.insert(int(float(index.data)), value)
    return Null.new()


@builtin("vector::remove", [ReferenceTo(Vector), Number])
def builtin_vector_remove(
    self: Reference, vector: Vector, index: Number
) -> Union[Value, Error]:
    if not float(index.data).is_integer():
        return Error(None, f"expected integer index, received {index}")
    if vector.data.uses > 1:
        vector.cow()  # copy-on-write
    idx = int(float(index.data))
    try:
        element = copy(vector.data[idx])
        del vector.data[idx]
        return element
    except IndexError:
        return Error(
            None,
            f"attempted vector::remove with invalid index {index}",
        )


@builtin("vector::slice", [ReferenceTo(Vector), Number, Number])
def builtin_vector_slice(
    self: Reference, vector: Vector, bgn: Number, end: Number
) -> Union[Value, Error]:
    if not float(bgn.data).is_integer():
        return Error(None, f"expected integer index, received {bgn}")
    if not float(end.data).is_integer():
        return Error(None, f"expected integer index, received {end}")
    bgn_index = int(float(bgn.data))
    end_index = int(float(end.data))
    if bgn_index < 0:
        return Error(None, "slice begin is less than zero")
    if bgn_index > len(vector.data):
        return Error(None, "slice begin is greater than the vector length")
    if end_index < 0:
        return Error(None, "slice end is less than zero")
    if end_index > len(vector.data):
        return Error(None, "slice end is greater than the vector length")
    if end_index < bgn_index:
        return Error(None, "slice end is less than slice begin")
    # Copy underlying data as the update will alter all Python objects
    # holding references to the underlying `SharedVectorData` object.
    underlying = copy(vector.data)
    return Vector.new(SharedVectorData(underlying[bgn_index:end_index]))


@builtin("vector::reversed", [ReferenceTo(Vector)])
def builtin_vector_reversed(self: Reference, vector: Vector) -> Union[Value, Error]:
    # Copy underlying data as the update will alter all Python objects
    # holding references to the underlying `SharedVectorData` object.
    underlying = copy(vector.data)
    return Vector.new(SharedVectorData(reversed(underlying)))


@builtin_from_source("vector::sorted")
def builtin_vector_sorted():
    return """
    let sort = function(x) {
        if x.count() <= 1 {
            return x;
        }
        let mid = (x.count() / 2).trunc();
        let lo = sort(x.slice(0, mid));
        let hi = sort(x.slice(mid, x.count()));
        let lo_index = 0;
        let hi_index = 0;
        let result = [];
        for _ in x.count() {
            if lo_index == lo.count() {
                result.push(hi[hi_index]);
                hi_index = hi_index + 1;
            }
            elif hi_index == hi.count() {
                result.push(lo[lo_index]);
                lo_index = lo_index + 1;
            }
            elif lo[lo_index] < hi[hi_index] {
                result.push(lo[lo_index]);
                lo_index = lo_index + 1;
            }
            else {
                result.push(hi[hi_index]);
                hi_index = hi_index + 1;
            }
        }
        return result;
    };
    return function(self) {
        if not ty::is_reference(self) {
            error $"expected reference to vector-like value for argument 0, received {typename(self)}";
        }
        if not ty::is_vector(self.*) {
            error $"expected reference to vector-like value for argument 0, received reference to {typename(self.*)}";
        }
        try { return sort(self.*); } catch err { error err; }
    };
    """


@builtin_from_source("vector::iterator")
def builtin_vector_iterator():
    return """
    return function(self) {
        let vector_iterator = type extends(iterator, {
            .next = function(self) {
                if self.index >= self.vector.*.count() {
                    return iterator::eoi();
                }
                let current = self.vector.*[self.index];
                self.index = self.index + 1;
                return current;
            },
        });
        return new vector_iterator {
            .vector = self,
            .index = 0,
        };
    };
    """


@builtin("map::count", [ReferenceTo(Map)])
def builtin_map_count(self: Reference, map: Map) -> Union[Value, Error]:
    return Number.new(len(map.data))


@builtin("map::contains", [ReferenceTo(Map), Value])
def builtin_map_contains(
    self: Reference, map: Map, target: Value
) -> Union[Value, Error]:
    return Boolean.new(target in map)


@builtin("map::insert", [ReferenceTo(Map), Value, Value])
def builtin_map_insert(
    self: Reference, map: Map, k: Value, v: Value
) -> Union[Value, Error]:
    map[k] = v
    return Null.new()


@builtin("map::remove", [ReferenceTo(Map), Value])
def builtin_map_remove(self: Reference, map: Map, k: Value) -> Union[Value, Error]:
    try:
        value = copy(map[k])
        del map[k]
        return value
    except KeyError:
        return Error(
            None,
            f"attempted map::remove on a map without key {k}",
        )


@builtin_from_source("map::union")
def builtin_map_union():
    return """
    return function(a, b) {
        try { a = a.*; } catch { } # &map -> map
        if not ty::is_map(a) or not ty::is_map(b) {
            error $"attempted map::union of values {repr(a)} and {repr(b)}";
        }

        let result = Map{};
        for k, v in a {
            map::insert(result.&, k, v);
        }
        for k, v in b {
            map::insert(result.&, k, v);
        }
        return result;
    };
    """


@builtin("set::count", [ReferenceTo(Set)])
def builtin_set_count(self: Reference, set: Set) -> Union[Value, Error]:
    return Number.new(len(set.data))


@builtin("set::contains", [ReferenceTo(Set), Value])
def builtin_set_contains(
    self: Reference, set: Set, target: Value
) -> Union[Value, Error]:
    return Boolean.new(target in set)


@builtin("set::insert", [ReferenceTo(Set), Value])
def builtin_set_insert(
    self: Reference, set: Set, element: Value
) -> Union[Value, Error]:
    set.insert(element)
    return Null.new()


@builtin("set::remove", [ReferenceTo(Set), Value])
def builtin_set_remove(
    self: Reference, set: Set, element: Value
) -> Union[Value, Error]:
    try:
        set.remove(element)
        return Null.new()
    except KeyError:
        return Error(
            None,
            f"attempted set::remove on a set without element {element}",
        )


@builtin_from_source("set::union")
def builtin_set_union():
    return """
    return function(a, b) {
        try { a = a.*; } catch { } # &set -> set
        if not ty::is_set(a) or not ty::is_set(b) {
            error $"attempted set::union of values {repr(a)} and {repr(b)}";
        }

        let result = Set{};
        for x in a {
            set::insert(result.&, x);
        }
        for x in b {
            set::insert(result.&, x);
        }
        return result;
    };
    """


@builtin_from_source("set::intersection")
def builtin_set_intersection():
    return """
    return function(a, b) {
        try { a = a.*; } catch { } # &set -> set
        if not ty::is_set(a) or not ty::is_set(b) {
            error $"attempted set::intersection of values {repr(a)} and {repr(b)}";
        }

        let result = Set{};
        for x in a {
            if b.contains(x) {
                set::insert(result.&, x);
            }
        }
        return result;
    };
    """


@builtin_from_source("set::difference")
def builtin_set_difference():
    return """
    return function(a, b) {
        try { a = a.*; } catch { } # &set -> set
        if not ty::is_set(a) or not ty::is_set(b) {
            error $"attempted set::difference of values {repr(a)} and {repr(b)}";
        }

        let result = Set{};
        for x in a {
            if not b.contains(x) {
                set::insert(result.&, x);
            }
        }
        return result;
    };
    """


@builtin("exit", [Number])
def builtin_exit(code: Number):
    if not float(code).is_integer():
        return Error(None, f"expected integer exit code, received {code}")
    sys.exit(int(code))


@builtin_from_source("assert")
def builtin_assert():
    return """
    let assert = function(condition) {
        if not condition {
            error "assertion failure";
        }
    };
    return assert;
    """


@builtin("typeof", [Value])
def builtin_typeof(value: Value) -> Union[Value, Error]:
    if value.meta is None:
        return Null.new()
    return value.meta


@builtin("typename", [Value])
def builtin_typename(value: Value) -> Union[Value, Error]:
    return String.new(typename(value))


@builtin_from_source("extends")
def builtin_extends():
    return """
    let extend = function(super, t) {
        return map::union(super, t);
    };
    return extend;
    """


@builtin("repr", [Value])
def builtin_repr(value: Value) -> Union[Value, Error]:
    return String.new(str(value))


@builtin("input", [])
def builtin_input() -> Union[Value, Error]:
    return String.new(sys.stdin.read())


@builtin("inputln", [])
def builtin_inputln() -> Union[Value, Error]:
    line = sys.stdin.readline()
    if len(line) == 0:
        return Null.new()
    return String.new(line[:-1] if line[-1] == "\n" else line)


@builtin("dump", [Value])
def builtin_dump(value: Value) -> Union[Value, Error]:
    print(str(value), end="")
    return Null.new()


@builtin("dumpln", [Value])
def builtin_dumpln(value: Value) -> Union[Value, Error]:
    print(str(value), end="\n")
    return Null.new()


@builtin("print", [Value])
def builtin_print(value: Value) -> Union[Value, Error]:
    metafunction = value.metafunction(CONST_STRING_INTO_STRING)
    if metafunction is not None:
        result = call(None, metafunction, [Reference.new(value)])
        if isinstance(result, Error):
            return result
        if not isinstance(result, String):
            return Error(
                None,
                f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
            )
        print(result.runes, end="")
    elif isinstance(value, String):
        print(value.runes, end="")
    else:
        print(str(value), end="")
    return Null.new()


@builtin("println", [Value])
def builtin_println(value: Value) -> Union[Value, Error]:
    metafunction = value.metafunction(CONST_STRING_INTO_STRING)
    if metafunction is not None:
        result = call(None, metafunction, [Reference.new(value)])
        if isinstance(result, Error):
            return result
        if not isinstance(result, String):
            return Error(
                None,
                f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
            )
        print(result.runes, end="\n")
    elif isinstance(value, String):
        print(value.runes, end="\n")
    else:
        print(str(value), end="\n")
    return Null.new()


@builtin("eprint", [Value])
def builtin_eprint(value: Value) -> Union[Value, Error]:
    metafunction = value.metafunction(CONST_STRING_INTO_STRING)
    if metafunction is not None:
        result = call(None, metafunction, [Reference.new(value)])
        if isinstance(result, Error):
            return result
        if not isinstance(result, String):
            return Error(
                None,
                f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
            )
        print(result.runes, end="", file=sys.stderr)
    elif isinstance(value, String):
        print(value.runes, end="", file=sys.stderr)
    else:
        print(str(value), end="", file=sys.stderr)
    return Null.new()


@builtin("eprintln", [Value])
def builtin_eprintln(value: Value) -> Union[Value, Error]:
    metafunction = value.metafunction(CONST_STRING_INTO_STRING)
    if metafunction is not None:
        result = call(None, metafunction, [Reference.new(value)])
        if isinstance(result, Error):
            return result
        if not isinstance(result, String):
            return Error(
                None,
                f"metafunction {quote(CONST_STRING_INTO_STRING.runes)} returned {result}",
            )
        print(result.runes, end="\n", file=sys.stderr)
    elif isinstance(value, String):
        print(value.runes, end="\n", file=sys.stderr)
    else:
        print(str(value), end="\n", file=sys.stderr)
    return Null.new()


@builtin_from_source("range")
def builtin_range():
    return """
    let range_iterator = type extends(iterator, {
        "init": function(bgn, end) {
            if end < bgn {
                error $"end-of-range {repr(end)} is less than beginning-of-range {repr(bgn)}";
            }
            return new range_iterator {
                .cur = bgn,
                .end = end,
            };
        },
        "next": function(self) {
            if self.cur >= self.end {
                error null; # end-of-iteration
            }
            let result = self.cur;
            self.cur = self.cur + 1;
            return result;
        },
    });
    let range = function(bgn, end) {
        return range_iterator::init(bgn, end);
    };
    return range;
    """


@builtin_from_source("min")
def builtin_min():
    return """
    let min = function(a, b) {
        if a <= b {
            return a;
        }
        return b;
    };
    return min;
    """


@builtin_from_source("max")
def builtin_max():
    return """
    let max = function(a, b) {
        if a >= b {
            return a;
        }
        return b;
    };
    return max;
    """


@builtin("import", [Value])
def builtin_import(target: String) -> Union[Value, Error]:
    env = Environment(BASE_ENVIRONMENT)
    module = env.get(CONST_STRING_MODULE)
    assert module is not None, "expected `module` to be in the environment"
    module_path = module[CONST_STRING_PATH]
    module_file = module[CONST_STRING_FILE]
    module_directory = module[CONST_STRING_DIRECTORY]
    assert isinstance(module_directory, String)
    # Always search the current module directory first
    paths: list[str] = [module_directory.runes]
    MELLIFERA_SEARCH_PATH = os.environ.get("MELLIFERA_SEARCH_PATH")
    if MELLIFERA_SEARCH_PATH is not None:
        paths += MELLIFERA_SEARCH_PATH.split(":")
    for p in paths:
        path = Path(p) / target.runes
        if path.is_dir():
            # If the path is a directory, such as in the case of a library,
            # load the entry point to the library and/or group of files, using
            # the name `<directory>/lib.mf` by convention.
            path = path / "lib.mf"
        absolute = str(path.absolute())
        module[String.new("path")] = String.new(absolute)
        module[String.new("file")] = String.new(os.path.basename(absolute))
        module[String.new("directory")] = String.new(os.path.dirname(absolute))
        try:
            result = eval_file(path, env)
            break
        except FileNotFoundError:
            pass
    else:
        result = Error(None, f"module {target} not found")
    # Always restore module fields
    module[CONST_STRING_PATH] = module_path
    module[CONST_STRING_FILE] = module_file
    module[CONST_STRING_DIRECTORY] = module_directory
    if isinstance(result, Error):
        return result
    if result is None:
        return Null.new()
    return result


@builtin("baseenv", [])
def builtin_baseenv() -> Union[Value, Error]:
    return Map.new(copy(BASE_ENVIRONMENT.store.data))


@builtin("fs::read", [String])
def builtin_fs_read(path: String) -> Union[Value, Error]:
    try:
        with open(path.runes, "rb") as f:
            data = f.read()
        return String.new(data)
    except Exception:
        return Error(None, f"failed to read file {path}")


@builtin("fs::write", [String, String])
def builtin_fs_write(path: String, data: String) -> Union[Value, Error]:
    try:
        with open(path.runes, "wb") as f:
            f.write(data.bytes)
        return Null.new()
    except Exception:
        return Error(None, f"failed write to file {path}")


@builtin("fs::append", [String, String])
def builtin_fs_append(path: String, data: String) -> Union[Value, Error]:
    try:
        with open(path.runes, "ab") as f:
            f.write(data.bytes)
        return Null.new()
    except Exception:
        return Error(None, f"failed append to file {path}")


@builtin("html::escape", [String])
def builtin_html_escape(value: String) -> Union[Value, Error]:
    return String.new(html.escape(value.runes))


def json_encode(value: Value):
    if isinstance(value, Null):
        return None
    if isinstance(value, Boolean):
        return value.data
    if isinstance(value, Number):
        if math.isnan(value) or math.isinf(value):
            raise ValueError(f"cannot JSON-encode value {value}")
        return int(value) if float(value).is_integer() else float(value)
    if isinstance(value, String):
        try:
            return value.bytes.decode("utf-8")
        except UnicodeDecodeError:
            raise ValueError(
                f"cannot JSON-encode string with invalid UTF-8 encoding {value}"
            )
    if isinstance(value, Vector):
        return list(value.data)
    if isinstance(value, Map):
        map = dict()
        for k, v in value.data.items():
            if not isinstance(k, String):
                raise ValueError(f"cannot JSON-encode map with key {k}")
            map[k.runes] = v
        return map
    raise TypeError(f"cannot JSON-encode value {value} of type {typename(value)}")


@builtin("json::encode", [Value])
def builtin_json_encode(value: Value) -> Union[Value, Error]:
    return String.new(
        json.dumps(value, default=json_encode, allow_nan=False, ensure_ascii=False)
    )


def json_decode(value: Any):
    if value is None:
        return Null.new()
    if isinstance(value, bool):
        return Boolean.new(value)
    if isinstance(value, (int, float)):
        assert not (math.isnan(value) or math.isinf(value))
        return Number.new(value)
    if isinstance(value, str):
        return String.new(value)
    if isinstance(value, list):
        return Vector.new([json_decode(x) for x in value])
    if isinstance(value, dict):
        return Map.new({json_decode(k): json_decode(v) for k, v in value.items()})
    raise TypeError(f"cannot JSON-decode type {type(value).__name__}")


@builtin("json::decode", [String])
def builtin_json_decode(value: String) -> Union[Value, Error]:
    def parse_constant(constant: str):
        # ECMA-404
        # > Numeric values that cannot be represented as sequences of digits
        # > (such as Infinity and NaN) are not permitted.
        raise ValueError(f"constant {value} is not permitted")

    try:
        return json_decode(json.loads(value.runes, parse_constant=parse_constant))
    except (TypeError, ValueError):
        raise TypeError(f"cannot JSON-decode string {value}")


@builtin("math::is_nan", [Number])
def builtin_math_is_nan(value: Number) -> Union[Value, Error]:
    return Boolean.new(math.isnan(value.data))


@builtin("math::is_inf", [Number])
def builtin_math_is_inf(value: Number) -> Union[Value, Error]:
    return Boolean.new(math.isinf(value.data))


@builtin("math::is_integer", [Number])
def builtin_math_is_integer(value: Number) -> Union[Value, Error]:
    return Boolean.new(float(value.data).is_integer())


@builtin("math::trunc", [Number])
def builtin_math_trunc(value: Number) -> Union[Value, Error]:
    return Number.new(math.trunc(float(value.data)))


@builtin("math::round", [Number])
def builtin_math_round(value: Number) -> Union[Value, Error]:
    return Number.new(round(float(value.data)))


@builtin("math::floor", [Number])
def builtin_math_floor(value: Number) -> Union[Value, Error]:
    return Number.new(math.floor(float(value.data)))


@builtin("math::ceil", [Number])
def builtin_math_ceil(value: Number) -> Union[Value, Error]:
    return Number.new(math.ceil(float(value.data)))


@builtin("math::abs", [Number])
def builtin_math_abs(value: Number) -> Union[Value, Error]:
    return Number.new(math.fabs(float(value.data)))


@builtin("math::exp", [Number])
def builtin_math_exp(value: Number) -> Union[Value, Error]:
    return Number.new(math.exp(float(value.data)))


@builtin("math::exp2", [Number])
def builtin_math_exp2(value: Number) -> Union[Value, Error]:
    return Number.new(math.exp2(float(value.data)))


@builtin("math::exp10", [Number])
def builtin_math_exp10(value: Number) -> Union[Value, Error]:
    return Number.new(math.pow(10, float(value.data)))


@builtin("math::log", [Number])
def builtin_math_log(value: Number) -> Union[Value, Error]:
    if float(value) == 0:
        return Number.new(-math.inf)
    try:
        return Number.new(math.log(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::log2", [Number])
def builtin_math_log2(value: Number) -> Union[Value, Error]:
    if float(value) == 0:
        return Number.new(-math.inf)
    try:
        return Number.new(math.log2(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::log10", [Number])
def builtin_math_log10(value: Number) -> Union[Value, Error]:
    if float(value) == 0:
        return Number.new(-math.inf)
    try:
        return Number.new(math.log10(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::pow", [Number, Number])
def builtin_math_pow(value: Number, power: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.pow(float(value.data), float(power.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::sqrt", [Number])
def builtin_math_sqrt(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.sqrt(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::cbrt", [Number])
def builtin_math_cbrt(value: Number) -> Union[Value, Error]:
    return Number.new(math.cbrt(float(value.data)))


@builtin_from_source("math::clamp")
def builtin_math_clamp():
    return """
    let clamp = function(value, min, max) {
        if value < min {
            return min;
        }
        if value > max {
            return max;
        }
        return value;
    };
    return clamp;
    """


@builtin("math::sin", [Number])
def builtin_math_sin(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.sin(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::cos", [Number])
def builtin_math_cos(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.cos(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::tan", [Number])
def builtin_math_tan(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.tan(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::asin", [Number])
def builtin_math_asin(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.asin(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::acos", [Number])
def builtin_math_acos(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.acos(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::atan", [Number])
def builtin_math_atan(value: Number) -> Union[Value, Error]:
    return Number.new(math.atan(float(value.data)))


@builtin("math::atan2", [Number, Number])
def builtin_math_atan2(y: Number, x: Number) -> Union[Value, Error]:
    return Number.new(math.atan2(float(y.data), float(x.data)))


@builtin("math::sinh", [Number])
def builtin_math_sinh(value: Number) -> Union[Value, Error]:
    return Number.new(math.sinh(float(value.data)))


@builtin("math::cosh", [Number])
def builtin_math_cosh(value: Number) -> Union[Value, Error]:
    return Number.new(math.cosh(float(value.data)))


@builtin("math::tanh", [Number])
def builtin_math_tanh(value: Number) -> Union[Value, Error]:
    return Number.new(math.tanh(float(value.data)))


@builtin("math::asinh", [Number])
def builtin_math_asinh(value: Number) -> Union[Value, Error]:
    return Number.new(math.asinh(float(value.data)))


@builtin("math::acosh", [Number])
def builtin_math_acosh(value: Number) -> Union[Value, Error]:
    try:
        return Number.new(math.acosh(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("math::atanh", [Number])
def builtin_math_atanh(value: Number) -> Union[Value, Error]:
    if float(value).is_integer() and int(value) == +1:
        return Number.new(+math.inf)
    if float(value).is_integer() and int(value) == -1:
        return Number.new(-math.inf)
    try:
        return Number.new(math.atanh(float(value.data)))
    except ValueError:
        return Number.new(math.nan)


@builtin("py::exec", [Value])
def builtin_py_exec(source: String) -> Union[Value, Error]:
    try:
        exec(source.runes, globals())
    except Exception:
        return Error(None, String.new(traceback.format_exc()))
    return Null.new()


@builtin("random::seed", [Value])
def builtin_random_seed(seed: Value) -> Union[Value, Error]:
    rng.seed(hash(seed))
    return Null.new()


@builtin("random::number", [Number, Number])
def builtin_random_number(a: Number, b: Number) -> Union[Value, Error]:
    return Number.new(rng.uniform(float(a.data), float(b.data)))


@builtin("random::integer", [Number, Number])
def builtin_random_integer(a: Number, b: Number) -> Union[Value, Error]:
    if not float(a).is_integer():
        return Error(None, f"expected integer, received {a}")
    if not float(b).is_integer():
        return Error(None, f"expected integer, received {b}")
    return Number.new(rng.randint(int(a), int(b)))


@builtin("re::group", [Number])
def builtin_re_group(n: Number) -> Union[Value, Error]:
    if not float(n).is_integer():
        return Error(None, f"expected integer, received {n}")
    if re_match_result is None:
        return Error(None, "regular expression did not match")
    try:
        if re_match_result.group(int(n)) is None:
            return Null.new()
        return String.new(re_match_result.group(int(n)))
    except IndexError:
        return Error(
            None,
            f"out-of-bounds regular expression capture group {int(n)}",
        )


@builtin("ty::is", [Value, Value])
def builtin_ty_is(value: Value, type: Value) -> Union[Value, Error]:
    if isinstance(type, Null):
        return Boolean.new(value.meta is None)
    if isinstance(type, MetaMap):
        return Boolean.new(value.meta is type)
    raise Exception(
        f"expected null or map value created with the `type` keyword, received {type}"
    )


@builtin("ty::is_null", [Value])
def builtin_ty_is_null(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Null))


@builtin("ty::is_boolean", [Value])
def builtin_ty_is_boolean(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Boolean))


@builtin("ty::is_number", [Value])
def builtin_ty_is_number(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Number))


@builtin("ty::is_string", [Value])
def builtin_ty_is_string(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, String))


@builtin("ty::is_regexp", [Value])
def builtin_ty_is_regexp(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Regexp))


@builtin("ty::is_vector", [Value])
def builtin_ty_is_vector(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Vector))


@builtin("ty::is_map", [Value])
def builtin_ty_is_map(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Map))


@builtin("ty::is_set", [Value])
def builtin_ty_is_set(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Set))


@builtin("ty::is_reference", [Value])
def builtin_ty_is_reference(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, Reference))


@builtin("ty::is_function", [Value])
def builtin_ty_is_function(value: Value) -> Union[Value, Error]:
    return Boolean.new(isinstance(value, (Function, Builtin)))


def eval_source(
    source: str,
    env: Optional[Environment] = None,
    loc: Optional[SourceLocation] = None,
) -> Optional[Union[Value, Error]]:
    lexer = Lexer(source, loc)
    parser = Parser(lexer)
    program = parser.parse_program()
    return program.eval(env or Environment(BASE_ENVIRONMENT))


def eval_file(
    path: Union[str, os.PathLike],
    env: Optional[Environment] = None,
    argv: Optional[list[str]] = None,
) -> Optional[Union[Value, Error]]:
    with open(path, "r", encoding="utf-8") as f:
        source = f.read()
    return eval_source(source, env, SourceLocation(str(path), 1))


# The base environment *may* be modified after program startup. Altering the
# base environment is explicitly permitted in order to allow modifications to
# the runtime from within `py::exec` function invocations.
BASE_ENVIRONMENT = Environment()

# Result of the last regular expression operation (=~, !~). Either an re2 Match
# or None. Non-None implies that the last pattern was a successful match.
re_match_result = None

# Metamaps for fundamental types *must* not be modified after program startup.
# These metamaps are used during AST construction, and values created via the
# `TYPE.new` initializers share these metamap references as an optimization.
#
# The function metamap is created first, as all other builtin functions will
# use _FUNCTION_META for their type/metamap.
_FUNCTION_META = MetaMap(name=String(Function.typename()))
_BOOLEAN_META = MetaMap(
    name=String(Boolean.typename()),
    data={
        String("init"): builtin_boolean_init(),
    },
)
_NUMBER_META = MetaMap(
    name=String(Number.typename()),
    data={
        String("init"): builtin_number_init(),
        String("is_nan"): builtin_number_is_nan(),
        String("is_inf"): builtin_number_is_inf(),
        String("is_integer"): builtin_number_is_integer(),
        String("fixed"): builtin_number_fixed(),
        String("trunc"): builtin_number_trunc(),
        String("round"): builtin_number_round(),
        String("floor"): builtin_number_floor(),
        String("ceil"): builtin_number_ceil(),
    },
)
_STRING_META = MetaMap(
    name=String(String.typename()),
    data={
        String("init"): builtin_string_init(),
        String("bytes"): builtin_string_bytes(),
        String("runes"): builtin_string_runes(),
        String("count"): builtin_string_count(),
        String("contains"): builtin_string_contains(),
        String("starts_with"): builtin_string_starts_with(),
        String("ends_with"): builtin_string_ends_with(),
        String("trim"): builtin_string_trim(),
        String("find"): builtin_string_find(),
        String("rfind"): builtin_string_rfind(),
        String("slice"): builtin_string_slice(),
        String("split"): builtin_string_split(),
        String("join"): builtin_string_join(),
        String("cut"): builtin_string_cut(),
        String("replace"): builtin_string_replace(),
        String("to_title"): builtin_string_to_title(),
        String("to_upper"): builtin_string_to_upper(),
        String("to_lower"): builtin_string_to_lower(),
    },
)
_REGEXP_META = MetaMap(name=String(Function.typename()))
_VECTOR_META = MetaMap(
    name=String(Vector.typename()),
    data={
        String.new("init"): builtin_vector_init(),
        String("count"): builtin_vector_count(),
        String("contains"): builtin_vector_contains(),
        String("find"): builtin_vector_find(),
        String("rfind"): builtin_vector_rfind(),
        String("push"): builtin_vector_push(),
        String("pop"): builtin_vector_pop(),
        String("insert"): builtin_vector_insert(),
        String("remove"): builtin_vector_remove(),
        String("slice"): builtin_vector_slice(),
        String("reversed"): builtin_vector_reversed(),
        String("sorted"): builtin_vector_sorted(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
        String("iterator"): builtin_vector_iterator(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
    },
)
_MAP_META = MetaMap(
    name=String(Map.typename()),
    data={
        String("count"): builtin_map_count(),
        String("contains"): builtin_map_contains(),
        String("insert"): builtin_map_insert(),
        String("remove"): builtin_map_remove(),
        String("union"): builtin_map_union(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
    },
)
_SET_META = MetaMap(
    name=String(Set.typename()),
    data={
        String("count"): builtin_set_count(),
        String("contains"): builtin_set_contains(),
        String("insert"): builtin_set_insert(),
        String("remove"): builtin_set_remove(),
        String("union"): builtin_set_union(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
        String("intersection"): builtin_set_intersection(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
        String("difference"): builtin_set_difference(
            Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
        ),
    },
)
_REFERENCE_META = MetaMap(name=String(Reference.typename()))

_ITERATOR_SOURCE = """
let iterator = type {
    .eoi = function() {
        error null; # end-of-iteration
    },
    .next = function(self) {
        error "unimplemented iterator::next";
    },
    .count = function(self) {
        let count = 0;
        for _ in self.* {
            count = count + 1;
        }
        return count;
    },
    .contains = function(self, value) {
        for x in self.* {
            if x == value {
                return true;
            }
        }
        return false;
    },
    .any = function(self, func) {
        for x in self.* {
            if func(x) {
                return true;
            }
        }
        return false;
    },
    .all = function(self, func) {
        for x in self.* {
            if not func(x) {
                return false;
            }
        }
        return true;
    },
    .filter = function(self, func) {
        let filter_iterator = type extends(iterator, {
            .next = function(self) {
                let current = self.base.next();
                while not func(current) {
                    current = self.base.next();
                }
                return current;
            },
        });
        return new filter_iterator {
            .base = self,
        };
    },
    .transform = function(self, func) {
        let transform_iterator = type extends(iterator, {
            .next = function(self) {
                return func(self.base.next());
            },
        });
        return new transform_iterator {
            .base = self,
        };
    },
    .into_vector = function(self) {
        let result = [];
        for x in self.* {
            result.push(x);
        }
        return result;
    },
};
return iterator;
"""
_ITERATOR = eval_source(_ITERATOR_SOURCE)
_ITERATOR_META = MetaMap(
    name=String("iterator"),
    data=_ITERATOR.data if isinstance(_ITERATOR, Map) else dict(),
)

BASE_ENVIRONMENT.let(String.new("boolean"), _BOOLEAN_META)
BASE_ENVIRONMENT.let(String.new("number"), _NUMBER_META)
BASE_ENVIRONMENT.let(String.new("string"), _STRING_META)
BASE_ENVIRONMENT.let(String.new("regexp"), _REGEXP_META)
BASE_ENVIRONMENT.let(String.new("vector"), _VECTOR_META)
BASE_ENVIRONMENT.let(String.new("map"), _MAP_META)
BASE_ENVIRONMENT.let(String.new("set"), _SET_META)
BASE_ENVIRONMENT.let(String.new("reference"), _REFERENCE_META)
BASE_ENVIRONMENT.let(String.new("iterator"), _ITERATOR_META)
BASE_ENVIRONMENT.let(String.new("NaN"), Number.new(float("NaN")))
BASE_ENVIRONMENT.let(String.new("Inf"), Number.new(float("Inf")))
BASE_ENVIRONMENT.let(String.new("exit"), builtin_exit())
BASE_ENVIRONMENT.let(String.new("assert"), builtin_assert())
BASE_ENVIRONMENT.let(String.new("typeof"), builtin_typeof())
BASE_ENVIRONMENT.let(String.new("typename"), builtin_typename())
BASE_ENVIRONMENT.let(String.new("extends"), builtin_extends())
BASE_ENVIRONMENT.let(String.new("repr"), builtin_repr())
BASE_ENVIRONMENT.let(String.new("input"), builtin_input())
BASE_ENVIRONMENT.let(String.new("inputln"), builtin_inputln())
BASE_ENVIRONMENT.let(String.new("dump"), builtin_dump())
BASE_ENVIRONMENT.let(String.new("dumpln"), builtin_dumpln())
BASE_ENVIRONMENT.let(String.new("print"), builtin_print())
BASE_ENVIRONMENT.let(String.new("println"), builtin_println())
BASE_ENVIRONMENT.let(String.new("eprint"), builtin_eprint())
BASE_ENVIRONMENT.let(String.new("eprintln"), builtin_eprintln())
BASE_ENVIRONMENT.let(
    String.new("range"),
    builtin_range(
        Environment(BASE_ENVIRONMENT), evaluated=BuiltinExplicitUninitialized()
    ),
)
BASE_ENVIRONMENT.let(String.new("min"), builtin_min())
BASE_ENVIRONMENT.let(String.new("max"), builtin_max())
BASE_ENVIRONMENT.let(String.new("import"), builtin_import())
BASE_ENVIRONMENT.let(String.new("baseenv"), builtin_baseenv())
BASE_ENVIRONMENT.let(
    String.new("fs"),
    Map.new(
        {
            String.new("read"): builtin_fs_read(),
            String.new("write"): builtin_fs_write(),
            String.new("append"): builtin_fs_append(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("html"),
    Map.new(
        {
            String.new("escape"): builtin_html_escape(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("json"),
    Map.new(
        {
            String.new("encode"): builtin_json_encode(),
            String.new("decode"): builtin_json_decode(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("math"),
    Map.new(
        {
            String.new("e"): Number.new(math.e),
            String.new("pi"): Number.new(math.pi),
            String.new("is_nan"): builtin_math_is_nan(),
            String.new("is_inf"): builtin_math_is_inf(),
            String.new("is_integer"): builtin_math_is_integer(),
            String.new("trunc"): builtin_math_trunc(),
            String.new("round"): builtin_math_round(),
            String.new("floor"): builtin_math_floor(),
            String.new("ceil"): builtin_math_ceil(),
            String.new("abs"): builtin_math_abs(),
            String.new("exp"): builtin_math_exp(),
            String.new("exp2"): builtin_math_exp2(),
            String.new("exp10"): builtin_math_exp10(),
            String.new("log"): builtin_math_log(),
            String.new("log2"): builtin_math_log2(),
            String.new("log10"): builtin_math_log10(),
            String.new("pow"): builtin_math_pow(),
            String.new("sqrt"): builtin_math_sqrt(),
            String.new("cbrt"): builtin_math_cbrt(),
            String.new("clamp"): builtin_math_clamp(),
            String.new("sin"): builtin_math_sin(),
            String.new("cos"): builtin_math_cos(),
            String.new("tan"): builtin_math_tan(),
            String.new("asin"): builtin_math_asin(),
            String.new("acos"): builtin_math_acos(),
            String.new("atan"): builtin_math_atan(),
            String.new("atan2"): builtin_math_atan2(),
            String.new("sinh"): builtin_math_sinh(),
            String.new("cosh"): builtin_math_cosh(),
            String.new("tanh"): builtin_math_tanh(),
            String.new("asinh"): builtin_math_asinh(),
            String.new("acosh"): builtin_math_acosh(),
            String.new("atanh"): builtin_math_atanh(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("py"),
    Map.new(
        {
            String.new("exec"): builtin_py_exec(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("module"),
    Map.new(
        {
            String.new("path"): Null.new(),
            String.new("file"): Null.new(),
            String.new("directory"): String.new(os.getcwd()),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("random"),
    Map.new(
        {
            String.new("seed"): builtin_random_seed(),
            String.new("number"): builtin_random_number(),
            String.new("integer"): builtin_random_integer(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("re"),
    Map.new(
        {
            String.new("group"): builtin_re_group(),
        }
    ),
)
BASE_ENVIRONMENT.let(
    String.new("ty"),
    Map.new(
        {
            String.new("is"): builtin_ty_is(),
            String.new("is_null"): builtin_ty_is_null(),
            String.new("is_boolean"): builtin_ty_is_boolean(),
            String.new("is_number"): builtin_ty_is_number(),
            String.new("is_string"): builtin_ty_is_string(),
            String.new("is_regexp"): builtin_ty_is_regexp(),
            String.new("is_vector"): builtin_ty_is_vector(),
            String.new("is_map"): builtin_ty_is_map(),
            String.new("is_set"): builtin_ty_is_set(),
            String.new("is_reference"): builtin_ty_is_reference(),
            String.new("is_function"): builtin_ty_is_function(),
        }
    ),
)


# Clean up and remove sentinel builtins.
def initialize_builtin_from_source(value: Value):
    assert isinstance(value, BuiltinFromSource)
    value.initialize()


initialize_builtin_from_source(_VECTOR_META[String("sorted")])
initialize_builtin_from_source(_VECTOR_META[String("iterator")])
initialize_builtin_from_source(_MAP_META[String("union")])
initialize_builtin_from_source(_SET_META[String("union")])
initialize_builtin_from_source(_SET_META[String("intersection")])
initialize_builtin_from_source(_SET_META[String("difference")])
initialize_builtin_from_source(BASE_ENVIRONMENT.store[String("range")])


class Repl(code.InteractiveConsole):
    def __init__(self, env: Optional[Environment] = None):
        super().__init__()
        self.env = env if env is not None else Environment(BASE_ENVIRONMENT)

    def runsource(self, source, filename="<input>", symbol="single"):
        lexer = Lexer(source)
        parser = Parser(lexer)
        try:
            program = parser.parse_program()
        except ParseError as e:
            if not source.endswith("\n"):
                # Assume the user has not finished entering their program, and
                # wait for an additional newline before producing an error.
                return True
            print(f"error: {e}")
            return False
        # If the program is valid, but did not end in a semicolon or additional
        # newline, then assume that there may be additional source to process,
        # e.g. the else clause of an if-elif-else statement.
        if not (source.endswith("\n") or source.rstrip().endswith(";")):
            return True
        result = program.eval(self.env)
        if isinstance(result, Value):
            print(result)
        if isinstance(result, Error):
            print(f"error: {result}")
        return False


def main() -> None:
    description = "The Mellifera Programming Language"
    parser = ArgumentParser(description=description)
    parser.add_argument("file", type=str, nargs="?", default=None)
    args, argv = parser.parse_known_args()

    if args.file is not None:
        argv.insert(0, args.file)
        env = Environment(BASE_ENVIRONMENT)
        module = env.get(CONST_STRING_MODULE)
        assert module is not None, "expected `module` to be in the environment"
        path = os.path.realpath(args.file)
        module[String.new("path")] = String.new(path)
        module[String.new("file")] = String.new(os.path.basename(path))
        module[String.new("directory")] = String.new(os.path.dirname(path))
        env.let(
            String.new("argv"),
            Vector.new([String.new(x) for x in argv]),
        )
        try:
            result = eval_file(args.file, env)
        except AssertionError:
            raise
        except Exception as e:
            print(e, file=sys.stderr)
            sys.exit(1)
        if isinstance(result, Return):
            print(result.value)
        if isinstance(result, Error):
            if result.location is not None:
                print(f"[{result.location}] error: {result}", file=sys.stderr)
            else:
                print(f"error: {result}", file=sys.stderr)
            for element in result.trace:
                s = f"...within {element.function}"
                if element.location is not None:
                    s += f" called from {element.location}"
                print(s, file=sys.stderr)
            sys.exit(1)
    else:
        HOME = os.environ.get("MELLIFERA_HOME", Path.home())
        HISTFILE = Path(HOME) / ".mellifera-history"
        HISTFILE_SIZE = 4096
        if readline and os.path.exists(HISTFILE):
            readline.read_history_file(HISTFILE)
        repl = Repl()
        repl.interact(banner="", exitmsg="")
        if readline:
            readline.set_history_length(HISTFILE_SIZE)
            readline.write_history_file(HISTFILE)


if __name__ == "__main__":
    main()
