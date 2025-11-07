package mellifera

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullTypename(t *testing.T) {
	ctx := &Context{}
	null := ctx.NewNull()
	assert.Equal(t, "null", null.Typename())
}

func TestNullString(t *testing.T) {
	ctx := &Context{}
	null := ctx.NewNull()
	assert.Equal(t, "null", null.String())
}

func TestNullCopy(t *testing.T) {
	ctx := &Context{}
	null := ctx.NewNull()
	assert.Same(t, null, null.Copy())
}

func TestBooleanTypename(t *testing.T) {
	ctx := &Context{}
	{
		boolean := ctx.NewBoolean(true)
		assert.Equal(t, "boolean", boolean.Typename())
	}
	{
		boolean := ctx.NewBoolean(false)
		assert.Equal(t, "boolean", boolean.Typename())
	}
}

func TestBooleanString(t *testing.T) {
	ctx := &Context{}
	{
		boolean := ctx.NewBoolean(true)
		assert.Equal(t, "true", boolean.String())
	}
	{
		boolean := ctx.NewBoolean(false)
		assert.Equal(t, "false", boolean.String())
	}
}

func TestBooleanCopy(t *testing.T) {
	ctx := &Context{}
	{
		boolean := ctx.NewBoolean(true)
		assert.Same(t, boolean, boolean.Copy())
	}
	{
		boolean := ctx.NewBoolean(false)
		assert.Same(t, boolean, boolean.Copy())
	}
}

func TestNumberTypename(t *testing.T) {
	ctx := &Context{}
	number := ctx.NewNumber(123.456)
	assert.Equal(t, "number", number.Typename())
}

func TestNumberString(t *testing.T) {
	ctx := &Context{}
	{
		number := ctx.NewNumber(0)
		assert.Equal(t, "0", number.String())
	}
	{
		number := ctx.NewNumber(+1)
		assert.Equal(t, "1", number.String())
	}
	{
		number := ctx.NewNumber(-1)
		assert.Equal(t, "-1", number.String())
	}
	{
		number := ctx.NewNumber(+123.456)
		assert.Equal(t, "123.456", number.String())
	}
	{
		number := ctx.NewNumber(-123.456)
		assert.Equal(t, "-123.456", number.String())
	}
	{
		number := ctx.NewNumber(math.NaN())
		assert.Equal(t, "NaN", number.String())
	}
	{
		number := ctx.NewNumber(math.Inf(+1))
		assert.Equal(t, "Inf", number.String())
	}
	{
		number := ctx.NewNumber(math.Inf(-1))
		assert.Equal(t, "-Inf", number.String())
	}
}

func TestNumberCopy(t *testing.T) {
	ctx := &Context{}
	number := ctx.NewNumber(123.456)
	assert.Same(t, number, number.Copy())
}

func TestStringTypename(t *testing.T) {
	ctx := &Context{}
	string := ctx.NewString("foo")
	assert.Equal(t, "string", string.Typename())
}

func TestStringString(t *testing.T) {
	ctx := &Context{}
	string := ctx.NewString("foo\t\n\"\\bar")
	assert.Equal(t, "\"foo\\t\\n\\\"\\\\bar\"", string.String())
}

func TestStringCopy(t *testing.T) {
	ctx := &Context{}
	string := ctx.NewString("foo")
	assert.Same(t, string, string.Copy())
}

func TestRegexpTypename(t *testing.T) {
	ctx := &Context{}
	regexp, err := ctx.NewRegexp(`^\w+$`)
	require.NoError(t, err)
	assert.Equal(t, "regexp", regexp.Typename())
}

func TestRegexpString(t *testing.T) {
	ctx := &Context{}
	regexp, err := ctx.NewRegexp(`^\w+$`)
	require.NoError(t, err)
	assert.Equal(t, `r"^\\w+$"`, regexp.String())
}

func TestRegexpCopy(t *testing.T) {
	ctx := &Context{}
	regexp, err := ctx.NewRegexp(`^\w+$`)
	require.NoError(t, err)
	assert.Same(t, regexp, regexp.Copy())
}

func TestRegexpInvalidText(t *testing.T) {
	ctx := &Context{}
	_, err := ctx.NewRegexp(`\q`)
	assert.EqualError(t, err, `invalid regular expression "\\q"`)
}

func TestVectorConstructorNilElements(t *testing.T) {
	ctx := &Context{}
	vector := ctx.NewVector(nil)
	assert.Equal(t, 0, vector.Count())
}

func TestVectorConstructorNonNilElements(t *testing.T) {
	ctx := &Context{}
	{
		vector := ctx.NewVector([]Value{})
		assert.Equal(t, 0, vector.Count())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		assert.Equal(t, 3, vector.Count())
	}
}

func TestVectorTypename(t *testing.T) {
	ctx := &Context{}
	vector := ctx.NewVector(nil)
	assert.Equal(t, "vector", vector.Typename())
}

func TestVectorString(t *testing.T) {
	ctx := &Context{}
	{
		vector := ctx.NewVector(nil)
		assert.Equal(t, "[]", vector.String())
	}
	{
		vector := ctx.NewVector([]Value{})
		assert.Equal(t, "[]", vector.String())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		assert.Equal(t, `["foo", "bar", "baz"]`, vector.String())
	}
}

func TestVectorCopy(t *testing.T) {
	ctx := &Context{}
	{
		vector := ctx.NewVector(nil)
		require.Equal(t, vector.Count(), vector.Copy().(*Vector).Count())
	}
	{
		vector := ctx.NewVector([]Value{})
		require.Equal(t, vector.Count(), vector.Copy().(*Vector).Count())
	}
	{
		vector := ctx.NewVector([]Value{
			ctx.NewString("foo"),
			ctx.NewString("bar"),
			ctx.NewString("baz"),
		})
		require.Equal(t, vector.Count(), vector.Copy().(*Vector).Count())
		assert.Equal(t, "foo", vector.Copy().(*Vector).Get(0).(*String).data)
		assert.Equal(t, "bar", vector.Copy().(*Vector).Get(1).(*String).data)
		assert.Equal(t, "baz", vector.Copy().(*Vector).Get(2).(*String).data)
	}
}

func TestVectorCopyOnWrite(t *testing.T) {
	ctx := &Context{}
	a := ctx.NewVector([]Value{
		ctx.NewString("foo"),
		ctx.NewString("bar"),
		ctx.NewString("baz"),
	})
	b := a.Copy().(*Vector)
	require.Equal(t, a.Count(), b.Count())
	require.Equal(t, 2, a.data.uses)
	require.Equal(t, 2, b.data.uses)
	require.Same(t, a.data, b.data)

	b.Set(1, ctx.NewNumber(123.456))
	require.Equal(t, a.Count(), b.Count())
	require.Equal(t, 1, a.data.uses)
	require.Equal(t, 1, b.data.uses)
	require.NotSame(t, a.data, b.data)

	assert.Equal(t, "foo", a.Get(0).(*String).data)
	assert.Equal(t, "bar", a.Get(1).(*String).data)
	assert.Equal(t, "baz", a.Get(2).(*String).data)

	assert.Equal(t, "foo", b.Get(0).(*String).data)
	assert.Equal(t, 123.456, b.Get(1).(*Number).data)
	assert.Equal(t, "baz", b.Get(2).(*String).data)
}

func TestMapConstructorNilElements(t *testing.T) {
	ctx := &Context{}
	m := ctx.NewMap(nil)
	require.Equal(t, 0, m.Count())

	require.Nil(t, m.Lookup(ctx.NewNull()))
	require.Nil(t, m.Lookup(ctx.NewNumber(456.123)))
}

func TestMapConstructorNonNilElements(t *testing.T) {
	ctx := &Context{}
	{
		m := ctx.NewMap([]MapPair{})
		assert.Equal(t, 0, m.Count())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		require.Equal(t, 3, m.Count())

		require.True(t, m.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")))
		require.True(t, m.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("def")))
		require.True(t, m.Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))

		require.Nil(t, m.Lookup(ctx.NewNull()))
		require.Nil(t, m.Lookup(ctx.NewNumber(456.123)))
	}
}

func TestMapTypename(t *testing.T) {
	ctx := &Context{}
	m := ctx.NewMap(nil)
	assert.Equal(t, "map", m.Typename())
}

func TestMapString(t *testing.T) {
	ctx := &Context{}
	{
		m := ctx.NewMap(nil)
		assert.Equal(t, "Map{}", m.String())
	}
	{
		m := ctx.NewMap([]MapPair{})
		assert.Equal(t, "Map{}", m.String())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		assert.Equal(t, `{123.456: "abc", "foo": "def", []: "hij"}`, m.String())
	}
}

func TestMapCopy(t *testing.T) {
	ctx := &Context{}
	{
		m := ctx.NewMap(nil)
		require.Equal(t, m.Count(), m.Copy().(*Map).Count())
	}
	{
		m := ctx.NewMap([]MapPair{})
		require.Equal(t, m.Count(), m.Copy().(*Map).Count())
	}
	{
		m := ctx.NewMap([]MapPair{
			{ctx.NewNumber(123.456), ctx.NewString("abc")},
			{ctx.NewString("foo"), ctx.NewString("def")},
			{ctx.NewVector(nil), ctx.NewString("hij")},
		})
		require.Equal(t, m.Count(), m.Copy().(*Map).Count())

		require.True(t, m.Copy().(*Map).Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")))
		require.True(t, m.Copy().(*Map).Lookup(ctx.NewString("foo")).Equal(ctx.NewString("def")))
		require.True(t, m.Copy().(*Map).Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))

		require.Nil(t, m.Copy().(*Map).Lookup(ctx.NewNull()))
		require.Nil(t, m.Copy().(*Map).Lookup(ctx.NewNumber(456.123)))
	}
}

func TestMapCopyOnWrite(t *testing.T) {
	ctx := &Context{}
	a := ctx.NewMap([]MapPair{
		{ctx.NewNumber(123.456), ctx.NewString("abc")},
		{ctx.NewString("foo"), ctx.NewString("def")},
		{ctx.NewVector(nil), ctx.NewString("hij")},
	})
	b := a.Copy().(*Map)
	require.Equal(t, a.Count(), b.Count())
	require.Equal(t, 2, a.data.uses)
	require.Equal(t, 2, b.data.uses)
	require.Same(t, a.data, b.data)

	b.Insert(ctx.NewNumber(123.456), ctx.NewNull())
	require.Equal(t, a.Count(), b.Count())
	require.Equal(t, 1, a.data.uses)
	require.Equal(t, 1, b.data.uses)
	require.NotSame(t, a.data, b.data)

	require.True(t, a.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")))
	require.True(t, a.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("def")))
	require.True(t, a.Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))

	require.True(t, b.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNull()))
	require.True(t, b.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("def")))
	require.True(t, b.Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))

	c := a.Copy().(*Map)
	c.Remove(ctx.NewString("foo"))

	require.Equal(t, a.Count()-1, c.Count())
	require.Equal(t, 1, a.data.uses)
	require.Equal(t, 1, c.data.uses)
	require.NotSame(t, a.data, c.data)

	require.True(t, a.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")))
	require.True(t, a.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("def")))
	require.True(t, a.Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))

	require.True(t, c.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewString("abc")))
	require.Nil(t, c.Lookup(ctx.NewString("foo")))
	require.True(t, c.Lookup(ctx.NewVector(nil)).Equal(ctx.NewString("hij")))
}

func TestSetConstructorNilElements(t *testing.T) {
	ctx := &Context{}
	set := ctx.NewSet(nil)
	require.Equal(t, 0, set.Count())

	require.Nil(t, set.Lookup(ctx.NewNull()))
	require.Nil(t, set.Lookup(ctx.NewNumber(456.123)))
}

func TestSetConstructorNonNilElements(t *testing.T) {
	ctx := &Context{}
	{
		set := ctx.NewSet([]Value{})
		assert.Equal(t, 0, set.Count())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		require.Equal(t, 3, set.Count())

		require.True(t, set.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
		require.True(t, set.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("foo")))
		require.True(t, set.Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))

		require.Nil(t, set.Lookup(ctx.NewNull()))
		require.Nil(t, set.Lookup(ctx.NewNumber(456.123)))
	}
}

func TestSetTypename(t *testing.T) {
	ctx := &Context{}
	set := ctx.NewSet(nil)
	assert.Equal(t, "set", set.Typename())
}

func TestSetString(t *testing.T) {
	ctx := &Context{}
	{
		set := ctx.NewSet(nil)
		assert.Equal(t, "Set{}", set.String())
	}
	{
		set := ctx.NewSet([]Value{})
		assert.Equal(t, "Set{}", set.String())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		assert.Equal(t, `{123.456, "foo", []}`, set.String())
	}
}

func TestSetCopy(t *testing.T) {
	ctx := &Context{}
	{
		set := ctx.NewSet(nil)
		require.Equal(t, set.Count(), set.Copy().(*Set).Count())
	}
	{
		set := ctx.NewSet([]Value{})
		require.Equal(t, set.Count(), set.Copy().(*Set).Count())
	}
	{
		set := ctx.NewSet([]Value{
			ctx.NewNumber(123.456),
			ctx.NewString("foo"),
			ctx.NewVector(nil),
		})
		require.Equal(t, set.Count(), set.Copy().(*Set).Count())

		require.True(t, set.Copy().(*Set).Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
		require.True(t, set.Copy().(*Set).Lookup(ctx.NewString("foo")).Equal(ctx.NewString("foo")))
		require.True(t, set.Copy().(*Set).Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))

		require.Nil(t, set.Copy().(*Set).Lookup(ctx.NewNull()))
		require.Nil(t, set.Copy().(*Set).Lookup(ctx.NewNumber(456.123)))
	}
}

func TestSetCopyOnWrite(t *testing.T) {
	ctx := &Context{}
	a := ctx.NewSet([]Value{
		ctx.NewNumber(123.456),
		ctx.NewString("foo"),
		ctx.NewVector(nil),
	})
	b := a.Copy().(*Set)
	require.Equal(t, a.Count(), b.Count())
	require.Equal(t, 2, a.data.uses)
	require.Equal(t, 2, b.data.uses)
	require.Same(t, a.data, b.data)

	b.Insert(ctx.NewString("bar"))
	require.Equal(t, a.Count()+1, b.Count())
	require.Equal(t, 1, a.data.uses)
	require.Equal(t, 1, b.data.uses)
	require.NotSame(t, a.data, b.data)

	require.True(t, a.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
	require.True(t, a.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("foo")))
	require.True(t, a.Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))

	require.True(t, b.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
	require.True(t, b.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("foo")))
	require.True(t, b.Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))
	require.True(t, b.Lookup(ctx.NewString("bar")).Equal(ctx.NewString("bar")))

	c := a.Copy().(*Set)
	c.Remove(ctx.NewString("foo"))

	require.Equal(t, a.Count()-1, c.Count())
	require.Equal(t, 1, a.data.uses)
	require.Equal(t, 1, c.data.uses)
	require.NotSame(t, a.data, c.data)

	require.True(t, a.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
	require.True(t, a.Lookup(ctx.NewString("foo")).Equal(ctx.NewString("foo")))
	require.True(t, a.Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))

	require.True(t, c.Lookup(ctx.NewNumber(123.456)).Equal(ctx.NewNumber(123.456)))
	require.Nil(t, c.Lookup(ctx.NewString("foo")))
	require.True(t, c.Lookup(ctx.NewVector(nil)).Equal(ctx.NewVector(nil)))
}
