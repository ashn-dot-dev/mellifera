.POSIX:
.PHONY: \
	default all \
	build \
	wasm \
	install \
	check \
	format \
	clean

MELLIFERA_HOME = $$HOME/.mellifera

default: build

all: build wasm format check

bin/mf: build

build:
	go build -o=bin/ cmd/mf/mf.go

wasm:
	GOARCH=wasm GOOS=js go build -o mellifera.wasm cmd/wasm/wasm.go
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" .

install: bin/mf
	mkdir -p "$(MELLIFERA_HOME)/bin"
	mkdir -p "$(MELLIFERA_HOME)/lib"
	cp -r bin "$(MELLIFERA_HOME)"
	cp -r lib "$(MELLIFERA_HOME)"

check: bin/mf
	MELLIFERA_HOME="$(realpath .)" sh bin/mf-test

format:
	go fmt
	go fmt cmd/mf/*.go
	go fmt cmd/wasm/*.go

clean:
	rm -f bin/mf
	rm -f mellifera.wasm
	rm -f wasm_exec.js
