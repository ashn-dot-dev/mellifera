![The Mellifera Programming Language](https://ashn.dev/mellifera/mellifera-logo.svg)

Mellifera is a simple, batteries-included scripting language featuring value
semantics, structural equality, copy-on-write data sharing, strong dynamic
typing, explicit references, and a lightweight nominal type system with
structural protocols.

```mellifera
println("Hello, world!");
```

```sh
$ mf examples/hello-world.mf
Hello, world!
```

Mellifera aims to be an enjoyable language for writing small scripts and CLI
utilities while still providing reasonable tools to develop larger projects.
Here is an example word counting program in Mellifera showcasing some of the
syntax and semantics of the language.

```mellifera
#!/usr/bin/env mf
# usage: cat FILE | word-count-simple.mf
let words = re::split(input(), r`\b+`)
    .into_iterator()
    .map(function(word) {
        return re::replace(word.to_lower(), r`[^\w]`, "");
    })
    .filter(function(word) {
        return word.count() != 0;
    });

let counts = Map{};
for word in words {
    try { counts[word] = counts[word] + 1; }
    catch { counts[word] = 1; }
}

let ordered = counts
    .pairs()
    .sorted_by(function(lhs, rhs) {
        return rhs.value - lhs.value;
    });
for pair in ordered {
    println($"{pair.key} {pair.value}");
}
```

```sh
curl -s https://www.gutenberg.org/files/71/71-0.txt | mf examples/word-count-simple.mf | head -n 5
the 700
to 441
and 421
of 391
a 295
```

Mellifera features value semantics, meaning assignment operations copy the
contents of a value when executed. After an assignment statement such as `a =
b`, `a` and `b` will contain separate copies of the same value. Mellifera also
performs equality comparisons based on structural equality, so two values are
considered to be equal if they have the same contents.

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
# x and y are no longer structurally equal as their contents now differ
println($`x == y is {x == y}`);

print("\n");

let z = ["foo", {"bar": "xyz"}, "baz"];
println($`z is {z}`);
# y and z are separate values that are structurally equal
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

Unlike most scripting languages, in which reference semantics are the default
way of assigning and passing around composite data, Mellifera uses value
semantics with explicit references. One may obtain a reference to a value using
the postfix `.&` operator, then later dereference that reference-value to get
the original referenced value using the postfix `.*` operator.

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

Mellifera contains a small number of core built-in types, all of which have
first-class language support: null, boolean, number, string, regular
expression, vector, map, set, reference, and function. These built-in types are
sufficient for most simple programs, but when the need arises for more
structured data, users have the option to define custom types with specific
behavior.

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

Mellifera is intended to be a practical language with reasonable
exception-based error handling and pleasant top-level error traces for when
things go wrong.

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

A brief language overview can be found in `overview.mf`, and additional example
programs can be found under the `examples` directory.

## Installing

Build the Mellifera interpreter and associated tooling and install Mellifera to
the location specified by `MELLIFERA_HOME` (default `$HOME/.mellifera`):

```sh
make build install                               # Install to $HOME/.mellifera
make build install MELLIFERA_HOME=/opt/mellifera # Install to /opt/mellifera
```

Then, add the following snippet to your `.bashrc` (or equivalent shell
configuration file), replacing `$HOME/.mellifera` with your chosen
`MELLIFERA_HOME` directory if installing to a non-default `MELLIFERA_HOME`
location:

```sh
export PATH="$HOME/.mellifera/bin:$PATH"
```

Finally, open a new interactive shell and verify the Mellifera interpreter was
successfully installed with:

```sh
$ printf 'println("Hello world!");' | mf /dev/stdin
Hello world!
```

## Development

Mellifera currently has two implementations: a library & interpreter written in
Go (`mellifera.go` and `cmd/mf/mf.go`) which serves as the primary language
implementation, and an older reference interpreter written in Python (`mf.py`),
which served as the original language implementation during prototyping. In
order to ensure that Mellifera does not depend on a particular host language,
both implementations are updated to support the same core set of language
features and standard library builtins. Most users will want to use the Go
implementation, and most `make` targets will default to the Go version of that
target. However, specific `*-go` and `*-py` versions of targets should be used
when hacking on either the Go implementation or Python implementation,
respectively.

### General Development (Default Go Implementation)

```sh
make build   # build standalone interpreter executable
make check   # run unit tests and interpreter golden tests
make format  # format sources
make install # install standalone Mellifera tooling
```

### Development on the Go Library & Interpreter

```sh
make build-go  # build standalone interpreter executable
make check-go  # run unit tests interpreter golden tests
make format-go # format sources using go fmt
make install   # install standalone Mellifera tooling
```

### Development on the Python Reference Interpreter

```sh
python3 -m venv .venv-mellifera
. .venv-mellifera/bin/activate
python3 -m pip install -r requirements.txt

make check-py  # run interpreter golden tests
make lint-py   # lint with mypy and flake8
make format-py # format using black
```

## License and Credits

All content in this repository, unless otherwise noted, is licensed under the
Zero-Clause BSD license.

See LICENSE for more information.

The Mellifera logo and Melli character were designed by Natalie Jara.
