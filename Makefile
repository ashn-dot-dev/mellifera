.POSIX:
.PHONY: build install check lint format clean

MELLIFERA_HOME = $$HOME/.mellifera

all: lint format check

bin/mf: mf.py
	python3 -m nuitka mf.py \
		--no-deployment-flag=self-execution \
		--output-filename="$$(pwd)/bin/mf" \
		--remove-output \
		--disable-ccache

build: bin/mf

install: build
	mkdir -p "$(MELLIFERA_HOME)/bin"
	mkdir -p "$(MELLIFERA_HOME)/lib"
	cp -r bin "$(MELLIFERA_HOME)"
	cp -r lib "$(MELLIFERA_HOME)"
	cp mf.py env "$(MELLIFERA_HOME)"

check:
	MELLIFERA_HOME="$(realpath .)" sh bin/mf-test

# Flake8 Ignored Errors:
#   E203 - Conflicts with Black.
#   E221 - Disabled for manual vertically-aligned code.
#   E241 - Disabled for manual vertically-aligned code.
#   E501 - Conflicts with Black.
#   W503 - Conflicts with Black.
lint:
	python3 -m mypy --check-untyped-defs mf.py bin/mf-test.py
	python3 -m flake8 --ignore=E203,E221,E241,E501,W503 mf.py bin/mf-test.py

format:
	python3 -m black mf.py bin/mf-test.py

clean:
	rm -f .mellifera-history
	rm -f bin/mf
	rm -rf __pycache__
	rm -rf .mypy_cache
