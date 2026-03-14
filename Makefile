.POSIX:
.PHONY: \
	all all-go all-py
	build build-go build-py \
	install \
	check check-go check-py \
	lint \
	format format-go format-py \
	clean

MELLIFERA_HOME = $$HOME/.mellifera

all: build format check

all-go: format-go check-go

all-py: format-py lint-py check-py

bin/mf: build-go

build: build-go

build-go:
	go build -o=bin/ cmd/mf/mf.go

build-py: mf.py
	python3 -m nuitka mf.py \
		--no-deployment-flag=self-execution \
		--output-filename="$$(pwd)/bin/mf" \
		--remove-output \
		--disable-ccache

install: build
	mkdir -p "$(MELLIFERA_HOME)/bin"
	mkdir -p "$(MELLIFERA_HOME)/lib"
	cp -r bin "$(MELLIFERA_HOME)"
	cp -r lib "$(MELLIFERA_HOME)"
	cp mf.py env "$(MELLIFERA_HOME)"

check: check-go

check-go: bin/mf
	go test
	MELLIFERA_HOME="$(realpath .)" sh bin/mf-test

check-py:
	MELLIFERA_HOME="$(realpath .)" sh bin/mf-test --py

# Flake8 Ignored Errors:
#   E203 - Conflicts with Black.
#   E221 - Disabled for manual vertically-aligned code.
#   E241 - Disabled for manual vertically-aligned code.
#   E501 - Conflicts with Black.
#   W503 - Conflicts with Black.
lint-py:
	python3 -m mypy --check-untyped-defs mf.py
	python3 -m flake8 --ignore=E203,E221,E241,E501,W503 mf.py

format: format-go

format-go:
	go fmt
	go fmt cmd/mf/*.go

format-py:
	python3 -m black mf.py

clean:
	rm -f .mellifera-history
	rm -f bin/mf
	rm -rf __pycache__
	rm -rf .mypy_cache
