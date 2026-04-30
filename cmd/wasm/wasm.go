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
	case *mellifera.Vector:
		result := js.Global().Get("Array").New(v.Count())
		for i, element := range v.Elements() {
			elementJsValue, err := JsValueFromMelliferaValue(ctx, element)
			if err != nil {
				return js.Null(), mellifera.NewError(nil, ctx.NewStringf("unable to represent Mellifera vector element %v at index %v in JavaScript", element, i))
			}
			result.SetIndex(i, elementJsValue)
		}
		return result, nil
	case *mellifera.Map:
		result := js.Global().Get("Object").New()
		for _, pair := range v.Pairs() {
			keyString, ok := pair.Key.(*mellifera.String)
			if !ok {
				return js.Null(), mellifera.NewError(nil, ctx.NewStringf("unable to represent Mellifera map key %v as JavaScript string key", pair.Key))
			}
			valueJsValue, err := JsValueFromMelliferaValue(ctx, pair.Value)
			if err != nil {
				return js.Null(), mellifera.NewError(nil, ctx.NewStringf("unable to represent Mellifera map value %v in JavaScript", pair.Value))
			}
			result.Set(keyString.Data(), valueJsValue)
		}
		return result, nil
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
	case js.TypeObject:
		isArray := js.Global().Get("Array").Call("isArray", jsValue).Bool()
		if isArray {
			elements := []mellifera.Value{}
			for i := range jsValue.Length() {
				element, err := JsValueIntoMelliferaValue(ctx, jsValue.Index(i))
				if err != nil {
					return nil, mellifera.NewError(nil, ctx.NewStringf("unable to represent JavaScript array element %v at index %v in Mellifera", jsValue.Index(i), i))
				}
				elements = append(elements, element)
			}
			return ctx.NewVector(elements), nil
		}
		pairs := []mellifera.MapPair{}
		entries := js.Global().Get("Object").Call("entries", jsValue)
		for i := range entries.Length() {
			keyJsValue := entries.Index(i).Index(0)
			keyMfValue, err := JsValueIntoMelliferaValue(ctx, keyJsValue)
			if err != nil {
				return nil, mellifera.NewError(nil, ctx.NewStringf("unable to represent JavaScript object key %v in Mellifera", keyJsValue))
			}
			valueJsValue := entries.Index(i).Index(1)
			valueMfValue, err := JsValueIntoMelliferaValue(ctx, valueJsValue)
			if err != nil {
				return nil, mellifera.NewError(nil, ctx.NewStringf("unable to represent JavaScript object value %v in Mellifera", valueJsValue))
			}
			pairs = append(pairs, mellifera.MapPair{keyMfValue, valueMfValue})
		}
		return ctx.NewMap(pairs), nil
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
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return JsValueIntoMelliferaValue(ctx, jsValue)
	})
}

func BuiltinJsIsUndefined(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_undefined", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeUndefined), nil
	})
}

func BuiltinJsIsNull(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_null", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeNull), nil
	})
}

func BuiltinJsIsBoolean(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_boolean", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeBoolean), nil
	})
}

func BuiltinJsIsNumber(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_number", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeNumber), nil
	})
}

func BuiltinJsIsString(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_string", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeString), nil
	})
}

func BuiltinJsIsSymbol(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_symbol", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeSymbol), nil
	})
}

func BuiltinJsIsArray(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_array", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(js.Global().Get("Array").Call("isArray", jsValue).Bool()), nil
	})
}

func BuiltinJsIsObject(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_object", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeObject), nil
	})
}

func BuiltinJsIsFunction(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::is_function", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}
		return ctx.NewBoolean(jsValue.Type() == js.TypeFunction), nil
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

func BuiltinJsValueDelete(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::value::delete", []mellifera.Type{
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

		delfJsValue.Delete(property.Data())
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

		if delfJsValue.Type() != js.TypeObject {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript object", delf))
		}

		methodJsValue := delfJsValue.Get(method.Data())
		if methodJsValue.Type() != js.TypeFunction {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is does not have method %s (found %v)", delf, method.Data(), ctx.NewExternalWithType(JsValueType, methodJsValue)))
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

func BuiltinJsCall(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::call", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
		mellifera.TVal(mellifera.VECTOR),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		function := arguments[0].(*mellifera.External)
		args := arguments[1].(*mellifera.Vector)

		functionJsValue, ok := function.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", functionJsValue))
		}

		if functionJsValue.Type() != js.TypeFunction {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript function", functionJsValue))
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

		result := functionJsValue.Invoke(argsSlice...)
		return ctx.NewExternalWithType(JsValueType, result), nil
	})
}

func BuiltinJsCallNew(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::call_new", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
		mellifera.TVal(mellifera.VECTOR),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		function := arguments[0].(*mellifera.External)
		args := arguments[1].(*mellifera.Vector)

		functionJsValue, ok := function.Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript value", functionJsValue))
		}

		if functionJsValue.Type() != js.TypeFunction {
			return nil, mellifera.NewError(nil, ctx.NewStringf("external value %v is not a JavaScript function", functionJsValue))
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

		result := functionJsValue.New(argsSlice...)
		return ctx.NewExternalWithType(JsValueType, result), nil
	})
}

func BuiltinJsGlobal(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::global", []mellifera.Type{}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		return ctx.NewExternalWithType(JsValueType, js.Global()), nil
	})
}

func BuiltinJsTypeof(ctx *mellifera.Context) *mellifera.Builtin {
	return ctx.NewBuiltin("js::typeof", []mellifera.Type{
		mellifera.TVal(mellifera.EXTERNAL),
	}, func(ctx *mellifera.Context, arguments []mellifera.Value) (mellifera.Value, error) {
		jsValue, ok := arguments[0].(*mellifera.External).Data().(js.Value)
		if !ok {
			return nil, mellifera.NewError(nil, ctx.NewStringf("invalid JavaScript value %v", arguments[0]))
		}

		// XXX: At the time of writing (Go version 1.26.2) attempting to call
		// js.Value.Type() on a BigInt instance will cause a runtime panic[1],
		// so switching on jsValue.Type() below has the potential to blow up
		// the current goroutine. It is basically impossible to work around
		// this issue at the moment, as preemptively checking the instance's
		// .constructor property with:
		//
		//	constructor := jsValue.Get("constructor")
		//	if constructor.Equal(js.Global().Get("BigInt")) {
		//		return ctx.NewString("bigint"), nil
		//	}
		//
		// will blow up if a non-object type is passed in, and attempting to
		// manually check if jsValue instanceof BigInt with:
		//
		//	jsValue.InstanceOf(js.Global().Get("BigInt")
		//
		// will fail with the Go runtime erronously returning false for this
		// expression.
		//
		// [1]: https://github.com/golang/go/issues/72050#issuecomment-4346819262

		// https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/typeof#description
		switch jsValue.Type() {
		case js.TypeUndefined:
			return ctx.NewString("undefined"), nil
		case js.TypeNull:
			// https://developer.mozilla.org/en-US/docs/Glossary/Null
			// Due to historical reasons, typeof null is "object".
			return ctx.NewString("object"), nil
		case js.TypeBoolean:
			return ctx.NewString("boolean"), nil
		case js.TypeNumber:
			return ctx.NewString("number"), nil
		case js.TypeString:
			return ctx.NewString("string"), nil
		case js.TypeSymbol:
			return ctx.NewString("symbol"), nil
		case js.TypeFunction:
			return ctx.NewString("function"), nil
		case js.TypeObject:
			return ctx.NewString("object"), nil
		default:
			panic("unknown JavaScript type")
		}
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
		{ctx.NewString("delete"), BuiltinJsValueDelete(&ctx)},
		{ctx.NewString("get_index"), BuiltinJsValueGetIndex(&ctx)},
		{ctx.NewString("set_index"), BuiltinJsValueSetIndex(&ctx)},
		{ctx.NewString("call"), BuiltinJsValueCall(&ctx)},
	}).Freeze().(*mellifera.Map)
	ctx.BaseEnvironment.Let("js", ctx.NewMap([]mellifera.MapPair{
		{ctx.NewString("value"), JsValueType},
		{ctx.NewString("from_mellifera"), BuiltinJsFromMellifera(&ctx)},
		{ctx.NewString("into_mellifera"), BuiltinJsIntoMellifera(&ctx)},
		{ctx.NewString("is_undefined"), BuiltinJsIsUndefined(&ctx)},
		{ctx.NewString("is_null"), BuiltinJsIsNull(&ctx)},
		{ctx.NewString("is_boolean"), BuiltinJsIsBoolean(&ctx)},
		{ctx.NewString("is_number"), BuiltinJsIsNumber(&ctx)},
		{ctx.NewString("is_string"), BuiltinJsIsString(&ctx)},
		{ctx.NewString("is_symbol"), BuiltinJsIsSymbol(&ctx)},
		{ctx.NewString("is_array"), BuiltinJsIsArray(&ctx)},
		{ctx.NewString("is_object"), BuiltinJsIsObject(&ctx)},
		{ctx.NewString("is_function"), BuiltinJsIsFunction(&ctx)},
		{ctx.NewString("call"), BuiltinJsCall(&ctx)},
		{ctx.NewString("call_new"), BuiltinJsCallNew(&ctx)},
		{ctx.NewString("global"), BuiltinJsGlobal(&ctx)},
		{ctx.NewString("typeof"), BuiltinJsTypeof(&ctx)},
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
