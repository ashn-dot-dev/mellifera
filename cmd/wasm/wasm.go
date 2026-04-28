package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
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

func JsValueFromMelliferaValue(ctx *mellifera.Context, mfValue mellifera.Value) (js.Value, error) {
	switch v := mfValue.(type) {
	case *mellifera.Null:
		return js.Null(), nil
	case *mellifera.Boolean:
		return js.ValueOf(v.Data()), nil
	case *mellifera.Number:
		return js.ValueOf(v.Data()), nil
	case *mellifera.String:
		return js.ValueOf(v.Data()), nil
	default:
		return js.Null(), mellifera.NewError(nil, ctx.NewStringf("unable to represent Mellifera value %v in JavaScript", mfValue))
	}
}

func JsValueIntoMelliferaValue(ctx *mellifera.Context, jsValue js.Value) (mellifera.Value, error) {
	switch jsValue.Type() {
	case js.TypeNull:
		return ctx.NewNull(), nil
	case js.TypeBoolean:
		return ctx.NewBoolean(jsValue.Bool()), nil
	case js.TypeNumber:
		return ctx.NewNumber(jsValue.Float()), nil
	case js.TypeString:
		return ctx.NewString(jsValue.String()), nil
	default:
		return nil, mellifera.NewError(nil, ctx.NewStringf("unable to represent JavaScript value %v in Mellifera", jsValue))
	}
}

func BuiltinJsFromMellifera(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::from_mellifera", []mellifera.Type{
		mellifera.TVal(mellifera.ANY),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, err := JsValueFromMelliferaValue(ctx, arguments[0])
		if err != nil {
			return nil, err
		}
		return ctx.NewExternalWithType(JsValueType, jsValue), nil
	})
}

func BuiltinJsIntoMellifera(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::into_mellifera", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value: %v", arguments[0]))
		}
		return JsValueIntoMelliferaValue(ctx, jsValue)
	})
}

var JsValueType *mellifera.Map = nil // js::value

func BuiltinJsValueGet(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::get", []mellifera.Type{
		mellifera.TRef(mellifera.TVal(mellifera.EXTERNAL)),
		mellifera.TVal(mellifera.STRING),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		self := arguments[0].(*mellifera.Reference)
		delf := self.Data().(*mellifera.External)
		property := arguments[1].(*mellifera.String)

		delfJsValue, ok := delf.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		if delfJsValue.Type() != js.TypeObject {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript object", delf))
		}

		lookup := delfJsValue.Get(property.Data())
		return ctx.NewExternalWithType(JsValueType, lookup), nil
	})
}

func BuiltinJsValueSet(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::set", []mellifera.Type{
		mellifera.TRef(mellifera.TVal(mellifera.EXTERNAL)),
		mellifera.TVal(mellifera.STRING),
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		self := arguments[0].(*mellifera.Reference)
		delf := self.Data().(*mellifera.External)
		property := arguments[1].(*mellifera.String)
		value := arguments[2].(*mellifera.External)

		delfJsValue, ok := delf.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		if delfJsValue.Type() != js.TypeObject {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript object", delf))
		}

		valueJsValue, ok := value.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		delfJsValue.Set(property.Data(), valueJsValue)
		return ctx.NewNull(), nil
	})
}

func BuiltinJsValueGetIndex(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::get_index", []mellifera.Type{
		mellifera.TRef(mellifera.TVal(mellifera.EXTERNAL)),
		mellifera.TVal(mellifera.NUMBER),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		self := arguments[0].(*mellifera.Reference)
		delf := self.Data().(*mellifera.External)
		index := arguments[1].(*mellifera.Number)

		delfJsValue, ok := delf.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		if delfJsValue.Type() != js.TypeObject {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript object", delf))
		}

		indexInt, err := mellifera.ValueAsInt(index)
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
		}

		lookup := delfJsValue.Index(indexInt)
		return ctx.NewExternalWithType(JsValueType, lookup), nil
	})
}

func BuiltinJsValueSetIndex(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::set_index", []mellifera.Type{
		mellifera.TRef(mellifera.TVal(mellifera.EXTERNAL)),
		mellifera.TVal(mellifera.NUMBER),
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		self := arguments[0].(*mellifera.Reference)
		delf := self.Data().(*mellifera.External)
		index := arguments[1].(*mellifera.Number)
		value := arguments[2].(*mellifera.External)

		delfJsValue, ok := delf.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		valueJsValue, ok := value.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		if delfJsValue.Type() != js.TypeObject {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript object", delf))
		}

		indexInt, err := mellifera.ValueAsInt(index)
		if err != nil {
			return nil, mellifera.NewError(nil, ctx.NewString(err.Error()))
		}

		delfJsValue.SetIndex(indexInt, valueJsValue)
		return ctx.NewNull(), nil
	})
}

func BuiltinJsValueCall(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::call", []mellifera.Type{
		mellifera.TRef(mellifera.TVal(mellifera.EXTERNAL)),
		mellifera.TVal(mellifera.STRING),
		mellifera.TVal(mellifera.VECTOR),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		self := arguments[0].(*mellifera.Reference)
		delf := self.Data().(*mellifera.External)
		method := arguments[1].(*mellifera.String)
		args := arguments[2].(*mellifera.Vector)

		delfJsValue, ok := delf.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", delf))
		}

		argsSlice := []any{}
		for i, arg := range args.Elements() {
			argExternal, ok := arg.(*mellifera.External)
			if !ok {
				return nil, mellifera.NewError(nil, ctx.NewStringf("argument %v with value %v is not an external JavaScript value", i, arg))
			}
			argJsValue, ok := argExternal.Data().(js.Value)
			if !ok {
				return nil, mellifera.NewError(nil, ctx.NewStringf("argument %v with value %v is not an external JavaScript value", i, arg))
			}
			argsSlice = append(argsSlice, argJsValue)
		}

		result := delfJsValue.Call(method.Data(), argsSlice...)
		return ctx.NewExternalWithType(JsValueType, result), nil
	})
}

func BuiltinJsGlobal(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::global", []mellifera.Type{}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		return ctx.NewExternalWithType(JsValueType, js.Global()), nil
	})
}

func eval(source string, stdout, stderr io.Writer) (mellifera.Value, error) {
	fmt.Printf("[mellifera.eval] stdout=%+v, stderr=%+v\n", stdout, stderr)
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[mellifera.eval] encountered panic: %v\n", r)
			debug.PrintStack()
		}
	}()

	ctx := mellifera.NewContext()
	ctx.Stdout = stdout
	ctx.Stderr = stderr

	JsValueType = ctx.NewMetaMap("js::value", []mellifera.MapPair{
		{ctx.NewString("get"), BuiltinJsValueGet(&ctx)},
		{ctx.NewString("set"), BuiltinJsValueSet(&ctx)},
		{ctx.NewString("get_index"), BuiltinJsValueGetIndex(&ctx)},
		{ctx.NewString("set_index"), BuiltinJsValueSetIndex(&ctx)},
		{ctx.NewString("call"), BuiltinJsValueCall(&ctx)},
	}).Freeze().(*mellifera.Map)
	ctx.BaseEnvironment.Let("js", ctx.NewMap([]mellifera.MapPair{
		{ctx.NewString("value"), JsValueType},
		{ctx.NewString("from_mellifera"), BuiltinJsFromMellifera(&ctx)},
		{ctx.NewString("into_mellifera"), BuiltinJsIntoMellifera(&ctx)},
		{ctx.NewString("global"), BuiltinJsGlobal(&ctx)},
	}).Freeze())

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
