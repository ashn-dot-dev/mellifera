package mellifera

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
