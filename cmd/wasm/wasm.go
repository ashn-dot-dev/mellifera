package main

import (
	"fmt"
	"io"
	"syscall/js"

	"ashn.dev/mellifera"
)

type textarea struct {
	id string
}

func (self textarea) clear() {
	element := js.Global().Get("document").Call("getElementById", self.id)
	element.Set("value", "")
}

func (self textarea) Write(p []byte) (n int, err error) {
	element := js.Global().Get("document").Call("getElementById", self.id)
	value := element.Get("value").String()
	element.Set("value", value+string(p))
	element.Set("scrollTop", element.Get("scrollHeight")) // focus on bottom
	return len(p), nil
}

func eval(source string, stdout, stderr io.Writer) (mellifera.Value, error) {
	fmt.Printf("[mellifera.eval] stdout=%+v, stderr=%+v\n", stdout, stderr)
	ctx := mellifera.NewContext()
	ctx.Stdout = stdout
	ctx.Stderr = stderr

	lexer := mellifera.NewLexer(&ctx, source, &mellifera.SourceLocation{"<program>", 1})
	parser, err := mellifera.NewParser(&lexer)
	if err != nil {
		return nil, err
	}

	program, err := parser.ParseProgram()
	if err != nil {
		return nil, err
	}

	return program.Eval(&ctx, &ctx.BaseEnvironment)
}

func main() {
	fmt.Println("Initialized Mellifera Wasm module...")
	object := js.Global().Get("mellifera")
	object.Set("eval", js.FuncOf(func(this js.Value, args []js.Value) any {
		stdout := textarea{args[1].String()}
		stderr := textarea{args[2].String()}

		stdout.clear()
		stderr.clear()

		_, err := eval(args[0].String(), stdout, stderr)
		if err != nil {
			if e, ok := err.(mellifera.ParseError); ok && e.Location != nil {
				fmt.Fprintf(stderr, "[%v, line %v] error: %v\n", e.Location.File, e.Location.Line, err)
			} else if e, ok := err.(mellifera.Error); ok {
				if e.Location != nil {
					fmt.Fprintf(stderr, "[%v, line %v] error: %v\n", e.Location.File, e.Location.Line, err)
				} else {
					fmt.Fprintf(stderr, "error: %v\n", err)
				}
				for _, element := range e.Trace {
					s := fmt.Sprintf("...within %v", element.FuncName)
					if element.Location != nil {
						s += fmt.Sprintf(" called from %s, line %v", element.Location.File, element.Location.Line)
					}
					fmt.Fprintf(stderr, "%s\n", s)
				}
			} else {
				fmt.Fprintf(stderr, "%v\n", err)
			}
		}

		return nil
	}))

	// Prevent the Wasm program from exiting.
	c := make(chan struct{}, 0)
	<-c
}
