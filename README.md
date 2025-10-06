The Mellifera Programming Language
==================================

Mellifera is a simple, modern, batteries-included scripting language featuring
value semantics, structural equality, copy-on-write data sharing, strong
dynamic typing, explicit references, and a lightweight nominal type system with
structural protocols.

Some small Mellifera example programs are provided below. A more comprehensive
language overview can be found in `overview.mf`, and additional examples can be
found under the `examples` directory.

## Examples

### Hello World

```mellifera
println("Hello, world!");
```

```sh
$ mf examples/hello-world.mf
Hello, world!
```

### Basic Types and Values

```mellifera
# null
dumpln( null );

# number: IEEE-754 double
dumpln( 123.456 );
dumpln( Inf );
dumpln( NaN );

# boolean: true or false
dumpln( true );
dumpln( false );

# string: byte strings with assumed UTF-8 encoding
dumpln( "hello 🍔" );
dumpln( "hello\t🍔" ); # string with escape sequences
dumpln( `hello\t🍔` ); # raw string where escape sequences are not processed
let burger = "🍔";
dumpln( $"hello\t{burger}" ); # template string with string interpolation
dumpln( $`hello\t{burger}` ); # template string using raw string literal

# regexp: regular expressions
dumpln( r`(\w+) (\w+)` );

# vector: ordered collection of elements
dumpln( [] );
dumpln( ["foo", "bar", "baz"] );

# map: collection of key-value pairs with unique keys
dumpln( Map{} ); # empty map requires Map to disambiguate from the empty set
dumpln( {"foo": 123, "bar": 456, "baz": ["abc", 789]} );
dumpln( {.foo = 123, .bar = 456, .baz = ["abc", 789]} ); # alternate syntax

# set: collection unique elements
dumpln( Set{} ); # empty set requires Set to disambiguate from the empty map
dumpln( {"foo", "bar", "baz", ["abc", 789]} );

# reference: pointer-like construct to allow in-place mutation and data sharing
dumpln( 123.& ); # .& is the postfix addressof operator

# function: first-class function values
dumpln( function() { println("hello"); } );
let add = function(a, b) { return a + b; };
dumpln( add );
println($"add(123, 456) is {add(123, 456)}");
```

```sh
$ mf examples/basic-types-and-values.mf
null
123.456
Inf
NaN
true
false
"hello 🍔"
"hello\t🍔"
"hello\\t🍔"
"hello\t🍔"
"hello\\t🍔"
r"(\\w+) (\\w+)"
[]
["foo", "bar", "baz"]
Map{}
{"foo": 123, "bar": 456, "baz": ["abc", 789]}
{"foo": 123, "bar": 456, "baz": ["abc", 789]}
Set{}
{"foo", "bar", "baz", ["abc", 789]}
reference@0x103826330
function@[examples/basic-types-and-values.mf, line 41]
add@[examples/basic-types-and-values.mf, line 42]
add(123, 456) is 579
```

### Value Semantics and Structural Equality

```mellifera
let x = ["foo", {"bar": 123}, "baz"];
let y = x; # x is assigned to y by copy
println($`x is {x}`);
println($`y is {y}`);
# x and y are separate values that are structurally equal
println($`x == y is {x == y}`);

print("\n");

# updates to x and y do not affect each other, because they are separate values
x[0] = "abc";
y[1]["bar"] = "xyz";
println($`x is {x}`);
println($`y is {y}`);
# x and y are no longer structurally equal as their contents' now differ
println($`x == y is {x == y}`);

print("\n");

let z = ["foo", {"bar": "xyz"}, "baz"];
println($`z is {z}`);
# y and z are separate values with structural equality
println($`y == z is {y == z}`);
```

```sh
$ mf examples/value-semantics-and-structural-equality.mf
x is ["foo", {"bar": 123}, "baz"]
y is ["foo", {"bar": 123}, "baz"]
x == y is true

x is ["abc", {"bar": 123}, "baz"]
y is ["foo", {"bar": "xyz"}, "baz"]
x == y is false

z is ["foo", {"bar": "xyz"}, "baz"]
y == z is true
```

### Explicit References

```mellifera
let pass_by_value = function(val) {
    println($"[inside pass_by_value] val starts as {val}");

    # Here val is a copy, so the mutation performed by `val.push` will not
    # change the original value outside of this function's lexical scope.
    val.push(123);

    println($"[inside pass_by_value] val ends as {val}");
};

let pass_by_reference = function(ref) {
    println($"[inside pass_by_reference] ref starts as {ref} with referenced value {ref.*}");

    # Here ref is a reference to the original value outside of this function's
    # lexical scope, so mutation performed by `ref.*.push` will change that
    # original value.
    ref.*.push(123); # .* is the postfix dereference operator

    println($"[inside pass_by_reference] ref ends as {ref} with referenced value {ref.*}");
};

let x = ["foo", "bar", "baz"];
println($"[outside pass_by_value] x starts as {x}");
pass_by_value(x);
println($"[outside pass_by_value] x ends as {x}");

print("\n");

let y = ["abc", "def", "hij"];
println($"[outside pass_by_reference] y starts as {y}");
pass_by_reference(y.&); # .& is the postfix addressof operator
println($"[outside pass_by_reference] y ends as {y}");
```

```sh
$ mf examples/explicit-references.mf
[outside pass_by_value] x starts as ["foo", "bar", "baz"]
[inside pass_by_value] val starts as ["foo", "bar", "baz"]
[inside pass_by_value] val ends as ["foo", "bar", "baz", 123]
[outside pass_by_value] x ends as ["foo", "bar", "baz"]

[outside pass_by_reference] y starts as ["abc", "def", "hij"]
[inside pass_by_reference] ref starts as reference@0x101f4c290 with referenced value ["abc", "def", "hij"]
[inside pass_by_reference] ref ends as reference@0x101f4c290 with referenced value ["abc", "def", "hij", 123]
[outside pass_by_reference] y ends as ["abc", "def", "hij", 123]
```

### Regular Expression Matching

```mellifera
let strings = [
    "Johnny Appleseed",
    "Some long text that does not match!",
    "Isaac Newton"
];
for s in strings {
    if s =~ r`^(\w+) (\w+)$` {
        println($"First Name = {repr(re::group(1))}, Last Name = {repr(re::group(2))}");
    }
}
```

```sh
$ mf examples/regular-expressions.mf
First Name = "Johnny", Last Name = "Appleseed"
First Name = "Isaac", Last Name = "Newton"
```

### Custom User-Defined Types

```mellifera
let vec2 = type {
    "fixed": function(self, ndigits) {
        return new vec2 {
            .x = self.x.fixed(ndigits),
            .y = self.y.fixed(ndigits),
        };
    },
    "magnitude": function(self) {
        return math::sqrt(self.x * self.x + self.y * self.y);
    },
};

let va = new vec2 {.x = 3, .y = 4};
println($`va is {va} with type {typename(va)}`);
println($`va.magnitude() is {va.magnitude()}`);

print("\n");

let vb = new vec2 {.x = math::e, .y = -math::pi};
println($`vb is {vb} with type {typename(vb)}`);
println($`vb.magnitude() is {vb.magnitude()}`);
println($`vb.fixed(3) is {vb.fixed(3)}`);
```

```sh
$ mf examples/user-defined-types.mf
va is {"x": 3, "y": 4} with type vec2
va.magnitude() is 5

vb is {"x": 2.718281828459045, "y": -3.141592653589793} with type vec2
vb.magnitude() is 4.154354402313313
vb.fixed(3) is {"x": 2.718, "y": -3.142}
```

### Custom User-Defined Iterators

```mellifera
let fizzbuzzer = type extends(iterator, {
    "init": function(n, max) {
        return new fizzbuzzer {"n": n, "max": max};
    },
    "next": function(self) {
        let n = self.n;
        if n > self.max {
            error null; # error null signals end-of-iteration
        }
        self.n = self.n + 1;
        if n % 3 == 0 and n % 5 == 0 {
            return "fizzbuzz";
        }
        if n % 3 == 0 {
            return "fizz";
        }
        if n % 5 == 0 {
            return "buzz";
        }
        return n;
    },
});

let fb = fizzbuzzer::init(1, 15);
for x in fb {
    println(x);
}
```

```sh
$ mf examples/user-defined-iterators.mf
1
2
fizz
4
buzz
fizz
7
8
fizz
buzz
11
fizz
13
14
fizzbuzz
```

### Exception-Based Error Handling With Pleasant Top-Level Error Traces

```mellifera
try {
    fs::read("/path/to/file/that/does/not/exist.txt");
}
catch err {
    println($"error: {err}");
}

print("\n");

let g = function() {
    let f = function() {
        error "oopsie";
    };
    f();
};
function() {
    let h = function() {
        g();
    };
    h();
}();
```

```sh
$ mf examples/exceptions.mf
error: failed to read file "/path/to/file/that/does/not/exist.txt"

[examples/exceptions.mf, line 12] error: oopsie
...within f@[examples/exceptions.mf, line 11] called from examples/exceptions.mf, line 14
...within g@[examples/exceptions.mf, line 10] called from examples/exceptions.mf, line 18
...within h@[examples/exceptions.mf, line 17] called from examples/exceptions.mf, line 20
...within function@[examples/exceptions.mf, line 16] called from examples/exceptions.mf, line 21
```

## Development

```sh
python3 -m venv .venv-mellifera
. .venv-mellifera/bin/activate
python3 -m pip install -r requirements.txt
MELLIFERA_HOME=$(pwd)

make check   # run tests
make lint    # lint with mypy and flake8
make format  # format using black
make build   # build standalone executable
make install # install standalone mellifera tooling
```

## Installing

Install Mellifera and associated tooling into the directory specified by
`MELLIFERA_HOME` (default `$HOME/.mellifera`):

```sh
make install                               # Install to the default $HOME/.mellifera
make install MELLIFERA_HOME=/opt/mellifera # Install to /opt/mellifera
```

Then, add the following snippet to your `.profile`, replacing `$HOME/.mellifera`
with your chosen `MELLIFERA_HOME` directory if installing to a non-default
`MELLIFERA_HOME` location:

```sh
export MELLIFERA_HOME="$HOME/.mellifera"
if [ -e "$MELLIFERA_HOME/env" ]; then
    . "$MELLIFERA_HOME/env"
fi
```

After sourcing your `.profile`, verify that the standalone Mellifera
interpreter `mf` was installed with:

```sh
$ printf 'println("Hello world!");' | mf /dev/stdin
Hello world!
```

## License

All content in this repository, unless otherwise noted, is licensed under the
Zero-Clause BSD license.

See LICENSE for more information.
