package mellifera

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
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
