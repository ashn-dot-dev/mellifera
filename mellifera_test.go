package mellifera

import (
	"math"
	"strings"
	"testing"
)

// Asserts a == b
func AssertEq[T comparable](t *testing.T, a T, b T) {
	t.Helper()
	if !(a == b) {
		t.Fatalf("expected a == b, received...\na: %+v\nb: %+v", a, b)
	}
}

// Asserts a != b
func AssertNe[T comparable](t *testing.T, a T, b T) {
	t.Helper()
	if !(a != b) {
		t.Fatalf("expected a != b, received...\na: %+v\nb: %+v", a, b)
	}
}

func TestNullTypename(t *testing.T) {
	ctx := NewContext()
	null := ctx.NewNull()
	AssertEq(t, "null", null.Typename())
}

func TestNullString(t *testing.T) {
	ctx := NewContext()
	null := ctx.NewNull()
	AssertEq(t, "null", null.String())
}

func TestNullCopy(t *testing.T) {
	ctx := NewContext()
	null := ctx.NewNull()
	AssertEq(t, null, null.Copy().(*Null))
}

func TestNullCombEncode(t *testing.T) {
	ctx := NewContext()
	null := ctx.NewNull()
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := null.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "null", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := null.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "null", sb.String())
	}
}

func TestBooleanData(t *testing.T) {
	ctx := NewContext()
	{
		boolean := ctx.NewBoolean(true)
		AssertEq(t, boolean.Data(), true)
	}
	{
		boolean := ctx.NewBoolean(false)
		AssertEq(t, boolean.Data(), false)
	}
}

func TestBooleanTypename(t *testing.T) {
	ctx := NewContext()
	{
		boolean := ctx.NewBoolean(true)
		AssertEq(t, "boolean", boolean.Typename())
	}
	{
		boolean := ctx.NewBoolean(false)
		AssertEq(t, "boolean", boolean.Typename())
	}
}

func TestBooleanString(t *testing.T) {
	ctx := NewContext()
	{
		boolean := ctx.NewBoolean(true)
		AssertEq(t, "true", boolean.String())
	}
	{
		boolean := ctx.NewBoolean(false)
		AssertEq(t, "false", boolean.String())
	}
}

func TestBooleanCopy(t *testing.T) {
	ctx := NewContext()
	{
		boolean := ctx.NewBoolean(true)
		AssertEq(t, boolean, boolean.Copy().(*Boolean))
	}
	{
		boolean := ctx.NewBoolean(false)
		AssertEq(t, boolean, boolean.Copy().(*Boolean))
	}
}

func TestBooleanCombEncode(t *testing.T) {
	ctx := NewContext()
	boolean := ctx.NewBoolean(true)
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := boolean.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "true", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := boolean.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "true", sb.String())
	}
}

func TestNumberData(t *testing.T) {
	ctx := NewContext()
	number := ctx.NewNumber(123.456)
	AssertEq(t, number.Data(), 123.456)
}

func TestNumberTypename(t *testing.T) {
	ctx := NewContext()
	number := ctx.NewNumber(123.456)
	AssertEq(t, "number", number.Typename())
}

func TestNumberString(t *testing.T) {
	ctx := NewContext()
	{
		number := ctx.NewNumber(0)
		AssertEq(t, "0", number.String())
	}
	{
		number := ctx.NewNumber(+1)
		AssertEq(t, "1", number.String())
	}
	{
		number := ctx.NewNumber(-1)
		AssertEq(t, "-1", number.String())
	}
	{
		number := ctx.NewNumber(+123.456)
		AssertEq(t, "123.456", number.String())
	}
	{
		number := ctx.NewNumber(-123.456)
		AssertEq(t, "-123.456", number.String())
	}
	{
		number := ctx.NewNumber(float64(0xdeadbeef))
		AssertEq(t, "3735928559", number.String())
	}
	{
		number := ctx.NewNumber(math.NaN())
		AssertEq(t, "NaN", number.String())
	}
	{
		number := ctx.NewNumber(math.Inf(+1))
		AssertEq(t, "Inf", number.String())
	}
	{
		number := ctx.NewNumber(math.Inf(-1))
		AssertEq(t, "-Inf", number.String())
	}
}

func TestNumberCopy(t *testing.T) {
	ctx := NewContext()
	number := ctx.NewNumber(123.456)
	AssertEq(t, number, number.Copy().(*Number))
}

func TestNumberCombEncode(t *testing.T) {
	ctx := NewContext()
	number := ctx.NewNumber(123.456)
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := number.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "123.456", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := number.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "123.456", sb.String())
	}

	{
		nan := ctx.NewNumber(math.NaN())
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := nan.CombEncode(e)
		AssertNe(t, err, nil)
		AssertEq(t, "invalid comb value NaN", err.Error())
	}
	{
		positiveInf := ctx.NewNumber(math.Inf(+1))
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := positiveInf.CombEncode(e)
		AssertNe(t, err, nil)
		AssertEq(t, "invalid comb value Inf", err.Error())
	}
	{
		negativeInf := ctx.NewNumber(math.Inf(-1))
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := negativeInf.CombEncode(e)
		AssertNe(t, err, nil)
		AssertEq(t, "invalid comb value -Inf", err.Error())
	}
}

func TestStringData(t *testing.T) {
	ctx := NewContext()
	string := ctx.NewString("foo")
	AssertEq(t, string.Data(), "foo")
}

func TestStringTypename(t *testing.T) {
	ctx := NewContext()
	string := ctx.NewString("foo")
	AssertEq(t, "string", string.Typename())
}

func TestStringString(t *testing.T) {
	ctx := NewContext()
	string := ctx.NewString("foo\t\n\"\\bar")
	AssertEq(t, "\"foo\\t\\n\\\"\\\\bar\"", string.String())
}

func TestStringCopy(t *testing.T) {
	ctx := NewContext()
	string := ctx.NewString("foo")
	AssertEq(t, string, string.Copy().(*String))
}

func TestStringCombEncode(t *testing.T) {
	ctx := NewContext()
	string := ctx.NewString("foo\nbar")
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := string.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `"foo\nbar"`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := string.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `"foo\nbar"`, sb.String())
	}
}

func TestRegexpData(t *testing.T) {
	ctx := NewContext()
	regexp, _ := ctx.NewRegexp(`^\w+$`)
	AssertEq(t, regexp.Data().String(), `^\w+$`)
}

func TestRegexpTypename(t *testing.T) {
	ctx := NewContext()
	regexp, err := ctx.NewRegexp(`^\w+$`)
	AssertEq(t, err, nil)
	AssertEq(t, "regexp", regexp.Typename())
}

func TestRegexpString(t *testing.T) {
	ctx := NewContext()
	regexp, err := ctx.NewRegexp(`^\w+$`)
	AssertEq(t, err, nil)
	AssertEq(t, `r"^\\w+$"`, regexp.String())
}

func TestRegexpCopy(t *testing.T) {
	ctx := NewContext()
	regexp, err := ctx.NewRegexp(`^\w+$`)
	AssertEq(t, err, nil)
	AssertEq(t, regexp, regexp.Copy().(*Regexp))
}

func TestRegexpCombEncode(t *testing.T) {
	ctx := NewContext()
	regexp, _ := ctx.NewRegexp("^.*$")
	var sb strings.Builder
	e := NewCombEncoder(&sb)
	err := regexp.CombEncode(e)
	AssertNe(t, err, nil)
	AssertEq(t, `invalid comb value r"^.*$"`, err.Error())
}

func TestRegexpInvalidText(t *testing.T) {
	ctx := NewContext()
	_, err := ctx.NewRegexp(`\q`)
	AssertNe(t, err, nil)
	AssertEq(t, err.Error(), `invalid regular expression "\\q"`)
}

func TestVectorConstructorNilElements(t *testing.T) {
	ctx := NewContext()
	vector := ctx.NewVector(nil)
	AssertEq(t, 0, vector.Count())
}

func TestVectorConstructorNonNilElements(t *testing.T) {
	ctx := NewContext()
	{
		vector := ctx.NewVector([]Value{})
		AssertEq(t, 0, vector.Count())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		AssertEq(t, 3, vector.Count())
	}
}

func TestVectorTypename(t *testing.T) {
	ctx := NewContext()
	vector := ctx.NewVector(nil)
	AssertEq(t, "vector", vector.Typename())
}

func TestVectorString(t *testing.T) {
	ctx := NewContext()
	{
		vector := ctx.NewVector(nil)
		AssertEq(t, "[]", vector.String())
	}
	{
		vector := ctx.NewVector([]Value{})
		AssertEq(t, "[]", vector.String())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		AssertEq(t, `["foo", "bar", "baz"]`, vector.String())
	}
}

func TestVectorCopy(t *testing.T) {
	ctx := NewContext()
	{
		vector := ctx.NewVector(nil)
		AssertEq(t, vector.Count(), vector.Copy().(*Vector).Count())
	}
	{
		vector := ctx.NewVector([]Value{})
		AssertEq(t, vector.Count(), vector.Copy().(*Vector).Count())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		AssertEq(t, vector.Count(), vector.Copy().(*Vector).Count())
		AssertEq(t, "foo", vector.Copy().(*Vector).Get(0).(*String).data)
		AssertEq(t, "bar", vector.Copy().(*Vector).Get(1).(*String).data)
		AssertEq(t, "baz", vector.Copy().(*Vector).Get(2).(*String).data)
	}
}

func TestVectorCopyOnWrite(t *testing.T) {
	ctx := NewContext()
	a := ctx.NewVector([]Value{
		ctx.NewString("foo"),
		ctx.NewString("bar"),
		ctx.NewString("baz"),
	})
	b := a.Copy().(*Vector)
	AssertEq(t, a.Count(), b.Count())
	AssertEq(t, 2, a.data.uses)
	AssertEq(t, 2, b.data.uses)
	AssertEq(t, a.data, b.data)

	b.Set(1, ctx.NewNumber(123.456))
	AssertEq(t, a.Count(), b.Count())
	AssertEq(t, 1, a.data.uses)
	AssertEq(t, 1, b.data.uses)
	AssertNe(t, a.data, b.data)

	AssertEq(t, "foo", a.Get(0).(*String).data)
	AssertEq(t, "bar", a.Get(1).(*String).data)
	AssertEq(t, "baz", a.Get(2).(*String).data)

	AssertEq(t, "foo", b.Get(0).(*String).data)
	AssertEq(t, 123.456, b.Get(1).(*Number).data)
	AssertEq(t, "baz", b.Get(2).(*String).data)
}

func TestVectorCombEncode(t *testing.T) {
	ctx := NewContext()

	empty := ctx.NewVector(nil)
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "[]", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "[]", sb.String())
	}

	nonEmpty := ctx.NewVector([]Value{
		ctx.NewNull(),
		ctx.NewBoolean(false),
		ctx.NewNumber(123.456),
		ctx.NewString("foo"),
		ctx.NewVector(nil),
		ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		}),
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `[null,false,123.456,"foo",[],["foo","bar","baz"]]`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Separator = " "
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `[null, false, 123.456, "foo", [], ["foo", "bar", "baz"]]`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `[
	null,
	false,
	123.456,
	"foo",
	[],
	[
		"foo",
		"bar",
		"baz"
	]
]`, sb.String())
	}

	deeplyNested := ctx.NewVector([]Value{
		ctx.NewVector([]Value{
			ctx.NewVector([]Value{
				ctx.NewVector([]Value{
					ctx.NewString("foo"),
				}),
			}),
		}),
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `[[[["foo"]]]]`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `[
	[
		[
			[
				"foo"
			]
		]
	]
]`, sb.String())
	}
}

func TestMapConstructorNilElements(t *testing.T) {
	ctx := NewContext()
	m := ctx.NewMap(nil)
	AssertEq(t, 0, m.Count())

	AssertEq(t, m.Get(ctx.NewNull()), nil)
	AssertEq(t, m.Get(ctx.NewNumber(456.123)), nil)
}

func TestMapConstructorNonNilElements(t *testing.T) {
	ctx := NewContext()
	{
		m := ctx.NewMap([]MapPair{})
		AssertEq(t, 0, m.Count())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		AssertEq(t, 3, m.Count())

		AssertEq(t, m.Get(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")), true)
		AssertEq(t, m.Get(ctx.NewString("foo")).Equal(ctx.NewString("def")), true)
		AssertEq(t, m.Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)

		AssertEq(t, m.Get(ctx.NewNull()), nil)
		AssertEq(t, m.Get(ctx.NewNumber(456.123)), nil)
	}
}

func TestMapTypename(t *testing.T) {
	{
		ctx := NewContext()
		m := ctx.NewMap(nil)
		AssertEq(t, "map", m.Typename())
	}
	{
		ctx := NewContext()
		meta := ctx.NewMetaMap("meta", nil)
		m := ctx.NewMapWithType(meta, nil)
		AssertEq(t, "meta", m.Typename())
	}
}

func TestMapString(t *testing.T) {
	ctx := NewContext()
	{
		m := ctx.NewMap(nil)
		AssertEq(t, "Map{}", m.String())
	}
	{
		m := ctx.NewMap([]MapPair{})
		AssertEq(t, "Map{}", m.String())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		AssertEq(t, `{123.456: "abc", "foo": "def", []: "hij"}`, m.String())
	}
}

func TestMapCopy(t *testing.T) {
	ctx := NewContext()
	{
		m := ctx.NewMap(nil)
		AssertEq(t, m.Count(), m.Copy().(*Map).Count())
	}
	{
		m := ctx.NewMap([]MapPair{})
		AssertEq(t, m.Count(), m.Copy().(*Map).Count())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		AssertEq(t, m.Count(), m.Copy().(*Map).Count())

		AssertEq(t, m.Copy().(*Map).Get(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")), true)
		AssertEq(t, m.Copy().(*Map).Get(ctx.NewString("foo")).Equal(ctx.NewString("def")), true)
		AssertEq(t, m.Copy().(*Map).Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)

		AssertEq(t, m.Copy().(*Map).Get(ctx.NewNull()), nil)
		AssertEq(t, m.Copy().(*Map).Get(ctx.NewNumber(456.123)), nil)
	}
}

func TestMapCopyOnWrite(t *testing.T) {
	ctx := NewContext()
	a := ctx.NewMap([]MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	b := a.Copy().(*Map)
	AssertEq(t, a.Count(), b.Count())
	AssertEq(t, 2, a.data.uses)
	AssertEq(t, 2, b.data.uses)
	AssertEq(t, a.data, b.data)

	b.Insert(ctx.NewNumber(123.456), ctx.NewNull())
	AssertEq(t, a.Count(), b.Count())
	AssertEq(t, 1, a.data.uses)
	AssertEq(t, 1, b.data.uses)
	AssertNe(t, a.data, b.data)

	AssertEq(t, a.Get(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")), true)
	AssertEq(t, a.Get(ctx.NewString("foo")).Equal(ctx.NewString("def")), true)
	AssertEq(t, a.Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)

	AssertEq(t, b.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNull()), true)
	AssertEq(t, b.Get(ctx.NewString("foo")).Equal(ctx.NewString("def")), true)
	AssertEq(t, b.Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)

	c := a.Copy().(*Map)
	c.Remove(ctx.NewString("foo"))

	AssertEq(t, a.Count()-1, c.Count())
	AssertEq(t, 1, a.data.uses)
	AssertEq(t, 1, c.data.uses)
	AssertNe(t, a.data, c.data)

	AssertEq(t, a.Get(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")), true)
	AssertEq(t, a.Get(ctx.NewString("foo")).Equal(ctx.NewString("def")), true)
	AssertEq(t, a.Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)

	AssertEq(t, c.Get(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")), true)
	AssertEq(t, c.Get(ctx.NewString("foo")), nil)
	AssertEq(t, c.Get(ctx.NewVector(nil)).Equal(ctx.NewString("hij")), true)
}

func TestMapCombEncode(t *testing.T) {
	ctx := NewContext()

	empty := ctx.NewMap(nil)
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "Map{}", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "Map{}", sb.String())
	}

	nonEmpty := ctx.NewMap([]MapPair{
		{ctx.NewNull(), ctx.NewNull()},
		{ctx.NewBoolean(false), ctx.NewBoolean(false)},
		{ctx.NewNumber(123.456), ctx.NewNumber(123.456)},
		{ctx.NewString("foo"), ctx.NewString("foo")},
		{ctx.NewString("empty"), ctx.NewMap(nil)},
		{ctx.NewString("non-empty"), ctx.NewMap([]MapPair{
			{ctx.NewString("abc"), ctx.NewString("foo")},
			{ctx.NewString("def"), ctx.NewString("bar")},
			{ctx.NewString("hij"), ctx.NewString("baz")},
		})},
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{null:null,false:false,123.456:123.456,"foo":"foo","empty":Map{},"non-empty":{"abc":"foo","def":"bar","hij":"baz"}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Separator = " "
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{null: null, false: false, 123.456: 123.456, "foo": "foo", "empty": Map{}, "non-empty": {"abc": "foo", "def": "bar", "hij": "baz"}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		e.Separator = " "
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{
	null: null,
	false: false,
	123.456: 123.456,
	"foo": "foo",
	"empty": Map{},
	"non-empty": {
		"abc": "foo",
		"def": "bar",
		"hij": "baz"
	}
}`, sb.String())
	}

	deeplyNested := ctx.NewMap([]MapPair{
		{ctx.NewString("foo"), ctx.NewMap([]MapPair{
			{ctx.NewString("bar"), ctx.NewMap([]MapPair{
				{ctx.NewString("baz"), ctx.NewMap([]MapPair{
					{ctx.NewString("qux"), ctx.NewMap(nil)},
				})},
			})},
		})},
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{"foo":{"bar":{"baz":{"qux":Map{}}}}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		e.Separator = " "
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{
	"foo": {
		"bar": {
			"baz": {
				"qux": Map{}
			}
		}
	}
}`, sb.String())
	}
}

func TestMapInsert(t *testing.T) {
	ctx := NewContext()

	a := ctx.NewMap([]MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	aInsertErr := a.Insert(ctx.NewString("xyz"), ctx.NewString("inserted"))
	AssertEq(t, aInsertErr, nil)
	AssertEq(t, `{123.456: "abc", "foo": "def", []: "hij", "xyz": "inserted"}`, a.String())

	b := ctx.NewMetaMap("meta", []MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	bInsertErr := b.Insert(ctx.NewString("xyz"), ctx.NewString("inserted"))
	AssertNe(t, bInsertErr, nil)
	AssertEq(t, `attempted to modify immutable map {123.456: "abc", "foo": "def", []: "hij"}`, bInsertErr.Error())
	AssertEq(t, `{123.456: "abc", "foo": "def", []: "hij"}`, b.String())
}

func TestMapRemove(t *testing.T) {
	ctx := NewContext()

	a := ctx.NewMap([]MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	aRemoveErr := a.Remove(ctx.NewString("foo"))
	AssertEq(t, aRemoveErr, nil)
	AssertEq(t, `{123.456: "abc", []: "hij"}`, a.String())

	b := ctx.NewMetaMap("meta", []MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	bRemoveErr := b.Remove(ctx.NewString("foo"))
	AssertNe(t, bRemoveErr, nil)
	AssertEq(t, `attempted to modify immutable map {123.456: "abc", "foo": "def", []: "hij"}`, bRemoveErr.Error())
	AssertEq(t, `{123.456: "abc", "foo": "def", []: "hij"}`, b.String())
}

func TestSetConstructorNilElements(t *testing.T) {
	ctx := NewContext()
	set := ctx.NewSet(nil)
	AssertEq(t, 0, set.Count())

	AssertEq(t, set.Get(ctx.NewNull()), nil)
	AssertEq(t, set.Get(ctx.NewNumber(456.123)), nil)
}

func TestSetConstructorNonNilElements(t *testing.T) {
	ctx := NewContext()
	{
		set := ctx.NewSet([]Value{})
		AssertEq(t, 0, set.Count())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		AssertEq(t, 3, set.Count())

		AssertEq(t, set.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
		AssertEq(t, set.Get(ctx.NewString("foo")).Equal(ctx.NewString("foo")), true)
		AssertEq(t, set.Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)

		AssertEq(t, set.Get(ctx.NewNull()), nil)
		AssertEq(t, set.Get(ctx.NewNumber(456.123)), nil)
	}
}

func TestSetTypename(t *testing.T) {
	ctx := NewContext()
	set := ctx.NewSet(nil)
	AssertEq(t, "set", set.Typename())
}

func TestSetString(t *testing.T) {
	ctx := NewContext()
	{
		set := ctx.NewSet(nil)
		AssertEq(t, "Set{}", set.String())
	}
	{
		set := ctx.NewSet([]Value{})
		AssertEq(t, "Set{}", set.String())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		AssertEq(t, `{123.456, "foo", []}`, set.String())
	}
}

func TestSetCopy(t *testing.T) {
	ctx := NewContext()
	{
		set := ctx.NewSet(nil)
		AssertEq(t, set.Count(), set.Copy().(*Set).Count())
	}
	{
		set := ctx.NewSet([]Value{})
		AssertEq(t, set.Count(), set.Copy().(*Set).Count())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		AssertEq(t, set.Count(), set.Copy().(*Set).Count())

		AssertEq(t, set.Copy().(*Set).Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
		AssertEq(t, set.Copy().(*Set).Get(ctx.NewString("foo")).Equal(ctx.NewString("foo")), true)
		AssertEq(t, set.Copy().(*Set).Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)

		AssertEq(t, set.Copy().(*Set).Get(ctx.NewNull()), nil)
		AssertEq(t, set.Copy().(*Set).Get(ctx.NewNumber(456.123)), nil)
	}
}

func TestSetCopyOnWrite(t *testing.T) {
	ctx := NewContext()
	a := ctx.NewSet([]Value{
		ctx.NewNumber(123.456),
		ctx.NewString("foo"),
		ctx.NewVector(nil),
	})
	b := a.Copy().(*Set)
	AssertEq(t, a.Count(), b.Count())
	AssertEq(t, 2, a.data.uses)
	AssertEq(t, 2, b.data.uses)
	AssertEq(t, a.data, b.data)

	b.Insert(ctx.NewString("bar"))
	AssertEq(t, a.Count()+1, b.Count())
	AssertEq(t, 1, a.data.uses)
	AssertEq(t, 1, b.data.uses)
	AssertNe(t, a.data, b.data)

	AssertEq(t, a.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
	AssertEq(t, a.Get(ctx.NewString("foo")).Equal(ctx.NewString("foo")), true)
	AssertEq(t, a.Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)

	AssertEq(t, b.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
	AssertEq(t, b.Get(ctx.NewString("foo")).Equal(ctx.NewString("foo")), true)
	AssertEq(t, b.Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)
	AssertEq(t, b.Get(ctx.NewString("bar")).Equal(ctx.NewString("bar")), true)

	c := a.Copy().(*Set)
	c.Remove(ctx.NewString("foo"))

	AssertEq(t, a.Count()-1, c.Count())
	AssertEq(t, 1, a.data.uses)
	AssertEq(t, 1, c.data.uses)
	AssertNe(t, a.data, c.data)

	AssertEq(t, a.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
	AssertEq(t, a.Get(ctx.NewString("foo")).Equal(ctx.NewString("foo")), true)
	AssertEq(t, a.Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)

	AssertEq(t, c.Get(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)), true)
	AssertEq(t, c.Get(ctx.NewString("foo")), nil)
	AssertEq(t, c.Get(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)), true)
}

func TestSetCombEncode(t *testing.T) {
	ctx := NewContext()

	empty := ctx.NewSet(nil)
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "Set{}", sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := empty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, "Set{}", sb.String())
	}

	nonEmpty := ctx.NewSet([]Value{
		ctx.NewNull(),
		ctx.NewBoolean(false),
		ctx.NewNumber(123.456),
		ctx.NewString("foo"),
		ctx.NewSet(nil),
		ctx.NewSet([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		}),
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{null,false,123.456,"foo",Set{},{"foo","bar","baz"}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Separator = " "
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{null, false, 123.456, "foo", Set{}, {"foo", "bar", "baz"}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		err := nonEmpty.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{
	null,
	false,
	123.456,
	"foo",
	Set{},
	{
		"foo",
		"bar",
		"baz"
	}
}`, sb.String())
	}

	deeplyNested := ctx.NewSet([]Value{
		ctx.NewSet([]Value{
			ctx.NewSet([]Value{
				ctx.NewSet([]Value{
					ctx.NewString("foo"),
				}),
			}),
		}),
	})
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{{{{"foo"}}}}`, sb.String())
	}
	{
		var sb strings.Builder
		e := NewCombEncoder(&sb)
		e.Indent = Ptr("\t")
		e.Separator = " "
		err := deeplyNested.CombEncode(e)
		AssertEq(t, err, nil)
		AssertEq(t, `{
	{
		{
			{
				"foo"
			}
		}
	}
}`, sb.String())
	}
}

func TestReferenceTypename(t *testing.T) {
	ctx := NewContext()
	reference := ctx.NewReference(ctx.NewNumber(123.456))
	AssertEq(t, "reference", reference.Typename())
}

func TestReferenceString(t *testing.T) {
	ctx := NewContext()
	reference := ctx.NewReference(ctx.NewNumber(123.456))
	AssertEq(t, strings.HasPrefix(reference.String(), "reference@"), true)
}

func TestReferenceCopy(t *testing.T) {
	ctx := NewContext()
	reference := ctx.NewReference(ctx.NewNumber(123.456))
	AssertEq(t, reference, reference.Copy().(*Reference))
}

func TestReferenceCombEncode(t *testing.T) {
	ctx := NewContext()
	reference := ctx.NewReference(ctx.NewNumber(123.456))
	var sb strings.Builder
	e := NewCombEncoder(&sb)
	err := reference.CombEncode(e)
	AssertNe(t, err, nil)
	AssertEq(t, strings.HasPrefix(err.Error(), "invalid comb value reference@"), true)
}

func TestExternalData(t *testing.T) {
	ctx := NewContext()
	var x int32 = 42
	external := ctx.NewExternal(x)
	AssertEq(t, external.Data().(int32), 42)
}

func TestExternalTypename(t *testing.T) {
	ctx := NewContext()
	var x int32 = 42
	external := ctx.NewExternal(x)
	AssertEq(t, "external", external.Typename())
}

func TestExternalString(t *testing.T) {
	ctx := NewContext()
	var x int32 = 42
	external := ctx.NewExternal(x)
	AssertEq(t, "external(42)", external.String())
}

func TestExternalCopy(t *testing.T) {
	ctx := NewContext()
	var x int32 = 42
	external := ctx.NewExternal(x)
	AssertEq(t, external, external.Copy().(*External))
}

func TestExternalCombEncode(t *testing.T) {
	ctx := NewContext()
	var x int32 = 42
	external := ctx.NewExternal(x)
	var sb strings.Builder
	e := NewCombEncoder(&sb)
	err := external.CombEncode(e)
	AssertNe(t, err, nil)
	AssertEq(t, "invalid comb value external(42)", err.Error())
}
