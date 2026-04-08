package main

import (
	"fmt"
	"io"
	"os"
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
	mf := js.Global().Get("mellifera")
	mf.Set("eval", js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) == 1 {
			args = append(args, js.ValueOf(js.ValueOf(map[string]any{}))) // empty options
		} else if len(args) > 2 {
			fmt.Fprintf(os.Stderr, "error: expected one or two arguments for mellifera.eval(source, options), received %v arguments\n", len(args))
			return nil
		}

		if args[0].Type() != js.TypeString {
			fmt.Fprintf(os.Stderr, "error: expected source to be a string, received %v\n", args[0])
			return nil
		}
		source := args[0].String()

		if args[1].Type() != js.TypeObject {
			fmt.Fprintf(os.Stderr, "error: expected options to be an object, received %v\n", args[1])
			return nil
		}
		options := args[1]

		var stdout io.Writer = os.Stdout
		if !options.Get("stdout").IsUndefined() {
			if options.Get("stdout").Type() != js.TypeString {
				fmt.Fprintf(os.Stderr, "error: expected options.stdout to be a string, received %v\n", options.Get("stdout"))
				return nil
			}
			stdoutTextarea := textarea{options.Get("stdout").String()}
			stdoutTextarea.clear()
			stdout = stdoutTextarea
		}
		var stderr io.Writer = os.Stderr
		if !options.Get("stderr").IsUndefined() {
			if options.Get("stderr").Type() != js.TypeString {
				fmt.Fprintf(os.Stderr, "error: expected options.stderr to be a string, received %v\n", options.Get("stderr"))
				return nil
			}
			stderrTextarea := textarea{options.Get("stderr").String()}
			stderrTextarea.clear()
			stderr = stderrTextarea
		}

		_, err := eval(source, stdout, stderr)
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
