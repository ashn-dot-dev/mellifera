package mellifera

import (
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
