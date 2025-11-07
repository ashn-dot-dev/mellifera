package mellifera

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
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
	CopyOnWrite()
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

func (ctx *Context) NewRegexp(text string) (*Regexp, error) {
	data, err := regexp.Compile(text)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression \"%s\"", escape(text))
	}
	return &Regexp{data}, nil
}

func (ctx *Context) NewVector(elements []Value) *Vector {
	if elements == nil {
		return &Vector{}
	}

	return &Vector{
		data: &VectorData{
			elements: elements,
			uses:     1,
		},
	}
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

func (self *Null) CopyOnWrite() {
	// immutable value
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

func (self *Boolean) CopyOnWrite() {
	// immutable value
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

func (self *Number) CopyOnWrite() {
	// immutable value
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

func (self *String) CopyOnWrite() {
	// immutable value
}

type Regexp struct {
	data *regexp.Regexp
}

func (self *Regexp) Typename() string {
	return "regexp"
}

func (self *Regexp) String() string {
	return fmt.Sprintf("r\"%s\"", escape(self.data.String()))
}

func (self *Regexp) Copy() Value {
	return self // immutable value
}

func (self *Regexp) CopyOnWrite() {
	// immutable value
}

type VectorData struct {
	elements []Value
	uses     int
}

type Vector struct {
	data *VectorData
}

func (self *Vector) Typename() string {
	return "vector"
}

func (self *Vector) String() string {
	if self.data == nil {
		return "[]"
	}

	s := make([]string, len(self.data.elements))
	for i, element := range self.data.elements {
		s[i] = element.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(s, ", "))
}

func (self *Vector) Copy() Value {
	if self.data == nil {
		return &Vector{}
	}

	self.data.uses += 1
	return &Vector{
		data: self.data,
	}
}

func (self *Vector) CopyOnWrite() {
	if self.data != nil && self.data.uses > 1 {
		self.data.uses -= 1
		elements := make([]Value, len(self.data.elements))
		for i, element := range self.data.elements {
			elements[i] = element.Copy()
		}
		self.data = &VectorData{
			elements: elements,
			uses:     1,
		}
	}
}

func (self *Vector) Count() int {
	if self.data == nil {
		return 0
	}

	return len(self.data.elements)
}

func (self *Vector) Get(index int) Value {
	return self.data.elements[index]
}

func (self *Vector) Set(index int, value Value) {
	self.CopyOnWrite()
	self.data.elements[index] = value
}
