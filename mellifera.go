package mellifera

import (
	"fmt"
	"math"
	"strconv"
)

func escape(s string) string {
	result := ""
	for _, r := range s {
		if r == '\t' {
			result += "\\t"
			continue
		}
		if r == '\n' {
			result += "\\n"
			continue
		}
		if r == '"' {
			result += "\\\""
			continue
		}
		if r == '\\' {
			result += "\\\\"
			continue
		}
		result += string(r)
	}
	return result
}

type Value interface {
	Typename() string
	String() string
	Copy() Value
}

type Context struct {
	null *Null
}

func NewContext() Context {
	ctx := Context{}
	ctx.null = ctx.NewNull()
	return ctx
}

func (ctx *Context) NewNull() *Null {
	return &Null{}
}

func (ctx *Context) NewBoolean(data bool) *Boolean {
	return &Boolean{data}
}

func (ctx *Context) NewNumber(data float64) *Number {
	return &Number{data}
}

func (ctx *Context) NewString(data string) *String {
	return &String{data}
}

type Null struct{}

func (self *Null) Typename() string {
	return "null"
}

func (self *Null) String() string {
	return "null"
}

func (self *Null) Copy() Value {
	return self // immutable value
}

type Boolean struct {
	data bool
}

func (self *Boolean) Typename() string {
	return "boolean"
}

func (self *Boolean) String() string {
	if self.data {
		return "true"
	}
	return "false"
}

func (self *Boolean) Copy() Value {
	return self // immutable value
}

type Number struct {
	data float64
}

func (self *Number) Typename() string {
	return "number"
}

func (self *Number) String() string {
	if math.IsNaN(self.data) {
		return "NaN"
	}
	if self.data == math.Inf(+1) {
		return "Inf"
	}
	if self.data == math.Inf(-1) {
		return "-Inf"
	}
	return strconv.FormatFloat(self.data, 'g', -1, 64)
}

func (self *Number) Copy() Value {
	return self // immutable value
}

type String struct {
	data string
}

func (self *String) Typename() string {
	return "string"
}

func (self *String) String() string {
	return fmt.Sprintf("\"%s\"", escape(self.data))
}

func (self *String) Copy() Value {
	return self // immutable value
}
