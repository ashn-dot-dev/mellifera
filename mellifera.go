package mellifera

import (
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Utility function used to get the address of literals.
func Ptr[T any](v T) *T {
	return &v
}

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

func quote(s string) string {
	if strings.Contains(s, "`") {
		return fmt.Sprintf(`"%s"`, s)
	}
	return fmt.Sprintf("`%s`", s)
}

// FNV-1a
func fnv1a(s string) uint64 {
	var hash uint64 = 14695981039346656037 // FNV_offset_basis
	for i := 0; i < len(s); i += 1 {
		hash ^= uint64(s[i])
		hash *= 1099511628211 // FNV_prime
	}
	return hash
}

type Value interface {
	Typename() string
	String() string
	Meta() *Map
	Copy() Value
	CopyOnWrite()
	Hash() uint64
	Equal(Value) bool
	CombEncode(e *CombEncoder) error
}

func ValueAsInt(value Value) (int, error) {
	integer, err := ValueAsInt64(value)
	if err != nil {
		return 0, err
	}
	if int64(int(integer)) != integer {
		return 0, fmt.Errorf("cannot convert %v into an int without truncation", value)
	}
	return int(integer), nil
}

func ValueAsInt64(value Value) (int64, error) {
	number, ok := value.(*Number)
	if !ok {
		return 0, fmt.Errorf("cannot convert %s-like value into an integer", value.Typename())
	}
	truncated := math.Trunc(number.data)
	if truncated != number.data {
		return 0, fmt.Errorf("cannot convert %v into an int64 without truncation", value)
	}
	return int64(truncated), nil
}

type CombEncoder struct {
	w           io.Writer
	indentText  *string // Optional: nil implies single-line default formatting.
	indentLevel int     // Number of times the indent text is written before main text per line.

	err error // Internal sticky error.
}

func NewCombEncoder(w io.Writer, indent *string) *CombEncoder {
	return &CombEncoder{
		w:           w,
		indentText:  indent,
		indentLevel: 0,
		err:         nil,
	}
}

func (e *CombEncoder) writeString(s string) error {
	if e.err != nil {
		return e.err
	}

	_, e.err = e.w.Write([]byte(s))

	return e.err
}

func (e *CombEncoder) writeIndent(s string) error {
	if e.indentText != nil {
		for range e.indentLevel {
			e.writeString(*e.indentText)
		}
	}

	e.writeString(s)

	return e.err
}

func (e *CombEncoder) writeEndOfLine() error {
	if e.indentText != nil {
		e.writeString("\n")
	} else {
		e.writeString(" ")
	}

	return e.err
}

type Context struct {
	functionMeta  *Map
	booleanMeta   *Map
	numberMeta    *Map
	stringMeta    *Map
	regexpMeta    *Map
	vectorMeta    *Map
	mapMeta       *Map
	setMeta       *Map
	referenceMeta *Map

	// Null Singleton.
	Null *Null
	// Boolean Singletons
	True  *Boolean
	False *Boolean
	// Base Environment
	BaseEnvironment Environment
	// Miscellaneous State and Definitions
	reNumberDec     *regexp.Regexp
	reNumberHex     *regexp.Regexp
	identifierCache map[string]*String
}

func NewContext() Context {
	ctx := Context{}

	ctx.functionMeta = ctx.NewMetaMap("function", nil)
	ctx.booleanMeta = ctx.NewMetaMap("boolean", nil)
	ctx.numberMeta = ctx.NewMetaMap("number", nil)
	ctx.stringMeta = ctx.NewMetaMap("string", nil)
	ctx.regexpMeta = ctx.NewMetaMap("regexp", nil)
	ctx.vectorMeta = ctx.NewMetaMap("vector", nil)
	ctx.mapMeta = ctx.NewMetaMap("map", nil)
	ctx.setMeta = ctx.NewMetaMap("set", nil)
	ctx.referenceMeta = ctx.NewMetaMap("reference", nil)

	ctx.Null = &Null{nil}
	ctx.True = &Boolean{true, ctx.booleanMeta}
	ctx.False = &Boolean{false, ctx.booleanMeta}
	ctx.BaseEnvironment = NewEnvironment(nil)
	ctx.reNumberDec = regexp.MustCompile(`^\d+(\.\d+)?`)
	ctx.reNumberHex = regexp.MustCompile(`^0x[0-9a-fA-F]+`)
	ctx.identifierCache = map[string]*String{}

	ctx.BaseEnvironment.Let("dump", BuiltinDump(&ctx))
	ctx.BaseEnvironment.Let("dumpln", BuiltinDumpln(&ctx))

	return ctx
}

func (ctx *Context) NewNull() *Null {
	return ctx.Null
}

func (ctx *Context) NewBoolean(data bool) *Boolean {
	if data {
		return ctx.True
	}
	return ctx.False
}

func (ctx *Context) NewNumber(data float64) *Number {
	return &Number{data, ctx.numberMeta}
}

func (ctx *Context) NewString(data string) *String {
	return &String{data, ctx.stringMeta}
}

func (ctx *Context) NewRegexp(text string) (*Regexp, error) {
	data, err := regexp.Compile(text)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression \"%s\"", escape(text))
	}
	return &Regexp{data, ctx.regexpMeta}, nil
}

func (ctx *Context) NewVector(elements []Value) *Vector {
	if elements == nil || len(elements) == 0 {
		return &Vector{
			data: nil,
			meta: ctx.vectorMeta,
		}
	}

	return &Vector{
		data: &VectorData{
			elements: elements,
			uses:     1,
		},
		meta: ctx.vectorMeta,
	}
}

func (ctx *Context) NewMap(elements []MapPair) *Map {
	if elements == nil || len(elements) == 0 {
		return &Map{
			data: nil,
			meta: ctx.mapMeta,
		}
	}

	result := &Map{
		data: nil,
		meta: ctx.mapMeta,
	}
	for _, element := range elements {
		result.Insert(element.key, element.value)
	}
	return result
}

func (ctx *Context) NewMetaMap(name string, elements []MapPair) *Map {
	if elements == nil || len(elements) == 0 {
		return &Map{
			data: nil,
			name: &name,
		}
	}

	result := &Map{}
	for _, element := range elements {
		result.Insert(element.key, element.value)
	}
	result.name = &name // freeze
	return result
}

func (ctx *Context) NewSet(elements []Value) *Set {
	if elements == nil || len(elements) == 0 {
		return &Set{
			data: nil,
			meta: ctx.setMeta,
		}
	}

	result := &Set{
		data: nil,
		meta: ctx.setMeta,
	}
	for _, element := range elements {
		result.Insert(element)
	}
	return result
}

func (ctx *Context) NewReference(value Value) *Reference {
	return &Reference{value, ctx.referenceMeta}
}

func (ctx *Context) NewFunction(ast *AstExpressionFunction, env *Environment) *Function {
	return &Function{ast, env, ctx.functionMeta}
}

func (ctx *Context) NewBuiltin(name string, types []Type, impl func(*Context, []Value) (Value, error)) *Builtin {
	return &Builtin{name, types, impl, ctx.functionMeta}
}

type Null struct {
	meta *Map // Optional
}

func (self *Null) Typename() string {
	return "null"
}

func (self *Null) String() string {
	return "null"
}

func (self *Null) Meta() *Map {
	return self.meta
}

func (self *Null) Copy() Value {
	return self // immutable value
}

func (self *Null) CopyOnWrite() {
	// immutable value
}

func (self *Null) Hash() uint64 {
	return 0
}

func (self *Null) Equal(other Value) bool {
	_, ok := other.(*Null)
	return ok
}

func (self *Null) CombEncode(e *CombEncoder) error {
	return e.writeString(self.String())
}

type Boolean struct {
	data bool
	meta *Map // Optional
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

func (self *Boolean) Meta() *Map {
	return self.meta
}

func (self *Boolean) Copy() Value {
	return self // immutable value
}

func (self *Boolean) CopyOnWrite() {
	// immutable value
}

func (self *Boolean) Hash() uint64 {
	if self.data {
		return 1
	}
	return 0
}

func (self *Boolean) Equal(other Value) bool {
	othr, ok := other.(*Boolean)
	if !ok {
		return false
	}
	return self.data == othr.data
}

func (self *Boolean) CombEncode(e *CombEncoder) error {
	return e.writeString(self.String())
}

type Number struct {
	data float64
	meta *Map // Optional
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
	return strconv.FormatFloat(self.data, 'f', -1, 64)
}

func (self *Number) Meta() *Map {
	return self.meta
}

func (self *Number) Copy() Value {
	return self // immutable value
}

func (self *Number) CopyOnWrite() {
	// immutable value
}

func (self *Number) Hash() uint64 {
	return math.Float64bits(self.data)
}

func (self *Number) Equal(other Value) bool {
	othr, ok := other.(*Number)
	if !ok {
		return false
	}
	return self.data == othr.data
}

func (self *Number) CombEncode(e *CombEncoder) error {
	if e.err == nil && (math.IsInf(self.data, 0) || math.IsNaN(self.data)) {
		e.err = fmt.Errorf("invalid comb value %s", self.String())
		return e.err
	}
	return e.writeString(self.String())
}

type String struct {
	data string
	meta *Map // Optional
}

func (self *String) Typename() string {
	return "string"
}

func (self *String) String() string {
	return fmt.Sprintf("\"%s\"", escape(self.data))
}

func (self *String) Meta() *Map {
	return self.meta
}

func (self *String) Copy() Value {
	return self // immutable value
}

func (self *String) CopyOnWrite() {
	// immutable value
}

func (self *String) Hash() uint64 {
	return fnv1a(self.data)
}

func (self *String) Equal(other Value) bool {
	othr, ok := other.(*String)
	if !ok {
		return false
	}
	return self.data == othr.data
}

func (self *String) CombEncode(e *CombEncoder) error {
	return e.writeString(self.String())
}

type Regexp struct {
	data *regexp.Regexp
	meta *Map // Optional
}

func (self *Regexp) Typename() string {
	return "regexp"
}

func (self *Regexp) String() string {
	return fmt.Sprintf("r\"%s\"", escape(self.data.String()))
}

func (self *Regexp) Meta() *Map {
	return self.meta
}

func (self *Regexp) Copy() Value {
	return self // immutable value
}

func (self *Regexp) CopyOnWrite() {
	// immutable value
}

func (self *Regexp) Hash() uint64 {
	return fnv1a(self.data.String())
}

func (self *Regexp) Equal(other Value) bool {
	othr, ok := other.(*Regexp)
	if !ok {
		return false
	}
	return self.data.String() == othr.data.String()
}

func (self *Regexp) CombEncode(e *CombEncoder) error {
	if e.err == nil {
		e.err = fmt.Errorf("invalid comb value %s", self.String())
	}
	return e.err
}

type VectorData struct {
	elements []Value
	uses     int
}

type Vector struct {
	data *VectorData
	meta *Map // Optional
}

func (self *Vector) Typename() string {
	return "vector"
}

func (self *Vector) String() string {
	if self.data == nil || len(self.data.elements) == 0 {
		return "[]"
	}

	s := make([]string, len(self.data.elements))
	for i, element := range self.data.elements {
		s[i] = element.String()
	}
	return fmt.Sprintf("[%s]", strings.Join(s, ", "))
}

func (self *Vector) Meta() *Map {
	return self.meta
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

func (self *Vector) Hash() uint64 {
	return fnv1a(self.String())
}

func (self *Vector) Equal(other Value) bool {
	othr, ok := other.(*Vector)
	if !ok {
		return false
	}
	if self.Count() != othr.Count() {
		return false
	}
	if self.data != nil && othr.data != nil {
		for i := range self.data.elements {
			if !self.data.elements[i].Equal(othr.data.elements[i]) {
				return false
			}
		}
	}
	return true
}

func (self *Vector) CombEncode(e *CombEncoder) error {
	if self.data == nil || len(self.data.elements) == 0 {
		e.writeString("[]")
		return e.err
	}

	e.writeString("[")
	if e.indentText != nil {
		e.writeEndOfLine()
	}
	e.indentLevel += 1

	for i, element := range self.data.elements {
		e.writeIndent("")
		element.CombEncode(e)

		if i != len(self.data.elements)-1 {
			e.writeString(",")
			e.writeEndOfLine()
		} else if e.indentText != nil {
			e.writeEndOfLine()
		}
	}

	e.indentLevel -= 1
	e.writeIndent("]")

	return e.err
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

func (self *Vector) Push(value Value) {
	self.CopyOnWrite()
	if self.data == nil {
		self.data = &VectorData{
			elements: nil,
			uses:     1,
		}
	}
	self.data.elements = append(self.data.elements, value)
}

type MapPair struct {
	key   Value
	value Value
}

type MapElement struct {
	prev  *MapElement
	next  *MapElement
	key   Value
	value Value
}

type MapData struct {
	buckets map[uint64][]*MapElement
	head    *MapElement
	tail    *MapElement
	count   int
	uses    int
}

// Returns nil on lookup failure.
func (self *MapData) LookupWithHash(key Value, hash uint64) *MapElement {
	bucket, ok := self.buckets[hash]
	if !ok || len(bucket) == 0 {
		return nil
	}

	for _, element := range bucket {
		if element.key.Equal(key) {
			return element
		}
	}

	return nil
}

// Returns nil on lookup failure.
func (self *MapData) Lookup(key Value) *MapElement {
	return self.LookupWithHash(key, key.Hash())
}

func (self *MapData) Insert(key, value Value) {
	if self.buckets == nil {
		self.buckets = make(map[uint64][]*MapElement)
	}

	hash := key.Hash()
	if self.head == nil {
		element := &MapElement{
			key:   key,
			value: value,
		}

		self.buckets[hash] = append(self.buckets[hash], element)
		self.head = element
		self.tail = element
		self.count = 1
		return
	}

	lookup := self.LookupWithHash(key, hash)
	if lookup == nil {
		element := &MapElement{
			prev:  self.tail,
			key:   key,
			value: value,
		}

		self.buckets[hash] = append(self.buckets[hash], element)
		self.tail.next = element
		self.tail = element
		self.count += 1
		return
	}

	lookup.key = key
	lookup.value = value
}

func (self *MapData) Remove(key Value) {
	if self.head == nil {
		return
	}

	hash := key.Hash()
	bucket, ok := self.buckets[hash]
	if !ok || len(bucket) == 0 {
		return
	}
	var lookup *MapElement
	for i := 0; i < len(bucket); i += 1 {
		if bucket[i].key.Equal(key) {
			lookup = bucket[i]
			self.buckets[hash] = append(bucket[:i], bucket[i+1:]...)
			if len(self.buckets[hash]) == 0 {
				delete(self.buckets, hash)
			}
			break
		}
	}

	if lookup != nil {
		if self.head == lookup {
			self.head = lookup.next
		}
		if self.tail == lookup {
			self.tail = lookup.prev
		}
		if lookup.prev != nil {
			lookup.prev.next = lookup.next
		}
		if lookup.next != nil {
			lookup.next.prev = lookup.prev
		}
		self.count -= 1
	}
}

type Map struct {
	data *MapData
	meta *Map    // Optional
	name *string // Optional (non-nil implies that this is a frozen metamap)
}

func (self *Map) Typename() string {
	return "map"
}

func (self *Map) String() string {
	if self.data == nil || self.data.count == 0 {
		return "Map{}"
	}

	s := make([]string, 0)
	cur := self.data.head
	for cur != nil {
		s = append(s, fmt.Sprintf("%s: %s", cur.key.String(), cur.value.String()))
		cur = cur.next
	}
	return fmt.Sprintf("{%s}", strings.Join(s, ", "))
}

func (self *Map) Meta() *Map {
	return self.meta
}

func (self *Map) Copy() Value {
	if self.data == nil {
		return &Map{}
	}

	self.data.uses += 1
	return &Map{
		data: self.data,
	}
}

func (self *Map) CopyOnWrite() {
	if self.data != nil && self.data.uses > 1 {
		self.data.uses -= 1
		data := &MapData{
			uses: 1,
		}

		cur := self.data.head
		for cur != nil {
			data.Insert(cur.key.Copy(), cur.value.Copy())
			cur = cur.next
		}
		self.data = data
	}
}

func (self *Map) Hash() uint64 {
	return fnv1a(self.String())
}

func (self *Map) Equal(other Value) bool {
	othr, ok := other.(*Map)
	if !ok {
		return false
	}
	if self.Count() != othr.Count() {
		return false
	}

	if self.Count() == 0 {
		// Empty maps.
		return true
	}

	// Non-empty maps - self and other both have non-nil data.
	selfCur := self.data.head
	othrCur := othr.data.head
	for selfCur != nil {
		if !selfCur.key.Equal(othrCur.key) {
			return false
		}
		if !selfCur.value.Equal(othrCur.value) {
			return false
		}
		selfCur = selfCur.next
		othrCur = othrCur.next
	}

	return true
}

func (self *Map) CombEncode(e *CombEncoder) error {
	if self.data == nil || self.data.count == 0 {
		e.writeString("Map{}")
		return e.err
	}

	e.writeString("{")
	if e.indentText != nil {
		e.writeEndOfLine()
	}
	e.indentLevel += 1

	cur := self.data.head
	for cur != nil {
		e.writeIndent("")
		cur.key.CombEncode(e)
		e.writeString(": ")
		cur.value.CombEncode(e)

		if cur != self.data.tail {
			e.writeString(",")
			e.writeEndOfLine()
		} else if e.indentText != nil {
			e.writeEndOfLine()
		}

		cur = cur.next
	}

	e.indentLevel -= 1
	e.writeIndent("}")

	return e.err
}

func (self *Map) Count() int {
	if self.data == nil {
		return 0
	}

	return self.data.count
}

func (self *Map) IsFrozen() bool {
	return self.name != nil
}

// Returns nil on lookup failure.
func (self *Map) Lookup(key Value) Value {
	if self.data == nil {
		return nil
	}

	element := self.data.Lookup(key)
	if element == nil {
		return nil
	}

	return element.value
}

// Returns an error when attempting to insert into a frozen map.
func (self *Map) Insert(key, value Value) error {
	if self.IsFrozen() {
		return fmt.Errorf("attempted to modify immutable map %v", self)
	}

	self.CopyOnWrite()
	if self.data == nil {
		self.data = &MapData{
			uses: 1,
		}
	}

	self.data.Insert(key, value)
	return nil
}

// Returns an error when attempting to remove from a frozen map.
func (self *Map) Remove(key Value) error {
	if self.IsFrozen() {
		return fmt.Errorf("attempted to modify immutable map %v", self)
	}

	if self.data == nil {
		return nil
	}

	self.CopyOnWrite()
	self.data.Remove(key)
	return nil
}

type SetElement struct {
	prev *SetElement
	next *SetElement
	key  Value
}

type SetData struct {
	buckets map[uint64][]*SetElement
	head    *SetElement
	tail    *SetElement
	count   int
	uses    int
}

// Returns nil on lookup failure.
func (self *SetData) LookupWithHash(key Value, hash uint64) *SetElement {
	bucket, ok := self.buckets[hash]
	if !ok || len(bucket) == 0 {
		return nil
	}

	for _, element := range bucket {
		if element.key.Equal(key) {
			return element
		}
	}

	return nil
}

// Returns nil on lookup failure.
func (self *SetData) Lookup(key Value) *SetElement {
	return self.LookupWithHash(key, key.Hash())
}

func (self *SetData) Insert(key Value) {
	if self.buckets == nil {
		self.buckets = make(map[uint64][]*SetElement)
	}

	hash := key.Hash()
	if self.head == nil {
		element := &SetElement{
			key: key,
		}

		self.buckets[hash] = append(self.buckets[hash], element)
		self.head = element
		self.tail = element
		self.count = 1
		return
	}

	lookup := self.LookupWithHash(key, hash)
	if lookup == nil {
		element := &SetElement{
			prev: self.tail,
			key:  key,
		}

		self.buckets[hash] = append(self.buckets[hash], element)
		self.tail.next = element
		self.tail = element
		self.count += 1
		return
	}

	lookup.key = key
}

func (self *SetData) Remove(key Value) {
	if self.head == nil {
		return
	}

	hash := key.Hash()
	bucket, ok := self.buckets[hash]
	if !ok || len(bucket) == 0 {
		return
	}
	var lookup *SetElement
	for i := 0; i < len(bucket); i += 1 {
		if bucket[i].key.Equal(key) {
			lookup = bucket[i]
			self.buckets[hash] = append(bucket[:i], bucket[i+1:]...)
			if len(self.buckets[hash]) == 0 {
				delete(self.buckets, hash)
			}
			break
		}
	}

	if lookup != nil {
		if self.head == lookup {
			self.head = lookup.next
		}
		if self.tail == lookup {
			self.tail = lookup.prev
		}
		if lookup.prev != nil {
			lookup.prev.next = lookup.next
		}
		if lookup.next != nil {
			lookup.next.prev = lookup.prev
		}
		self.count -= 1
	}
}

type Set struct {
	data *SetData
	meta *Map // Optional
}

func (self *Set) Typename() string {
	return "set"
}

func (self *Set) String() string {
	if self.data == nil || self.data.count == 0 {
		return "Set{}"
	}

	s := make([]string, 0)
	cur := self.data.head
	for cur != nil {
		s = append(s, cur.key.String())
		cur = cur.next
	}
	return fmt.Sprintf("{%s}", strings.Join(s, ", "))
}

func (self *Set) Meta() *Map {
	return self.meta
}

func (self *Set) Copy() Value {
	if self.data == nil {
		return &Set{}
	}

	self.data.uses += 1
	return &Set{
		data: self.data,
	}
}

func (self *Set) CopyOnWrite() {
	if self.data != nil && self.data.uses > 1 {
		self.data.uses -= 1
		data := &SetData{
			uses: 1,
		}

		cur := self.data.head
		for cur != nil {
			data.Insert(cur.key.Copy())
			cur = cur.next
		}
		self.data = data
	}
}

func (self *Set) Hash() uint64 {
	return fnv1a(self.String())
}

func (self *Set) Equal(other Value) bool {
	othr, ok := other.(*Set)
	if !ok {
		return false
	}
	if self.Count() != othr.Count() {
		return false
	}

	if self.Count() == 0 {
		// Empty sets.
		return true
	}

	// Non-empty sets - self and other both have non-nil data.
	selfCur := self.data.head
	othrCur := othr.data.head
	for selfCur != nil {
		if !selfCur.key.Equal(othrCur.key) {
			return false
		}
		selfCur = selfCur.next
		othrCur = othrCur.next
	}

	return true
}

func (self *Set) CombEncode(e *CombEncoder) error {
	if self.data == nil || self.data.count == 0 {
		e.writeString("Set{}")
		return e.err
	}

	e.writeString("{")
	if e.indentText != nil {
		e.writeEndOfLine()
	}
	e.indentLevel += 1

	cur := self.data.head
	for cur != nil {
		e.writeIndent("")
		cur.key.CombEncode(e)

		if cur != self.data.tail {
			e.writeString(",")
			e.writeEndOfLine()
		} else if e.indentText != nil {
			e.writeEndOfLine()
		}

		cur = cur.next
	}

	e.indentLevel -= 1
	e.writeIndent("}")

	return e.err
}

func (self *Set) Count() int {
	if self.data == nil {
		return 0
	}

	return self.data.count
}

// Returns nil on lookup failure.
func (self *Set) Lookup(value Value) Value {
	if self.data == nil {
		return nil
	}

	element := self.data.Lookup(value)
	if element == nil {
		return nil
	}

	return element.key
}

func (self *Set) Insert(value Value) {
	self.CopyOnWrite()
	if self.data == nil {
		self.data = &SetData{
			uses: 1,
		}
	}

	self.data.Insert(value)
}

func (self *Set) Remove(value Value) {
	if self.data == nil {
		return
	}

	self.CopyOnWrite()
	self.data.Remove(value)
}

type Reference struct {
	data Value
	meta *Map // Optional
}

func (self *Reference) Typename() string {
	return "reference"
}

func (self *Reference) String() string {
	return fmt.Sprintf("reference@%p", self.data)
}

func (self *Reference) Meta() *Map {
	return self.meta
}

func (self *Reference) Copy() Value {
	return self // immutable value
}

func (self *Reference) CopyOnWrite() {
	// immutable value
}

func (self *Reference) Hash() uint64 {
	return uint64(reflect.ValueOf(self.data).Pointer())
}

func (self *Reference) Equal(other Value) bool {
	othr, ok := other.(*Reference)
	if !ok {
		return false
	}
	return reflect.ValueOf(self.data).Pointer() == reflect.ValueOf(othr.data).Pointer()
}

func (self *Reference) CombEncode(e *CombEncoder) error {
	if e.err == nil {
		e.err = fmt.Errorf("invalid comb value %s", self.String())
	}
	return e.err
}

type Function struct {
	Ast  *AstExpressionFunction
	Env  *Environment
	meta *Map // Optional
}

func (self *Function) Typename() string {
	return "function"
}

func (self *Function) String() string {
	name := "function"
	if self.Ast.Name != nil {
		name = self.Ast.Name.data
	}
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
			name = escape(name)
			break
		}
	}
	if self.Ast.Location != nil {
		return fmt.Sprintf("%s@[%v, line %v]", name, self.Ast.Location.File, self.Ast.Location.Line)
	}
	return name
}

func (self *Function) Meta() *Map {
	return self.meta
}

func (self *Function) Copy() Value {
	return self // immutable value
}

func (self *Function) CopyOnWrite() {
	// immutable value
}

func (self *Function) Hash() uint64 {
	return uint64(reflect.ValueOf(self.Ast).Pointer() + reflect.ValueOf(self.Env).Pointer())
}

func (self *Function) Equal(other Value) bool {
	othr, ok := other.(*Function)
	if !ok {
		return false
	}
	return reflect.ValueOf(self.Ast).Pointer() == reflect.ValueOf(othr.Ast).Pointer() && reflect.ValueOf(self.Env).Pointer() == reflect.ValueOf(othr.Env).Pointer()
}

func (self *Function) CombEncode(e *CombEncoder) error {
	if e.err == nil {
		e.err = fmt.Errorf("invalid comb value %s", self.String())
	}
	return e.err
}

type Builtin struct {
	name  string
	types []Type
	impl  func(*Context, []Value) (Value, error)
	meta  *Map // Optional
}

func (self *Builtin) Typename() string {
	return "function"
}

func (self *Builtin) String() string {
	return fmt.Sprintf("%s@builtin", self.name)
}

func (self *Builtin) Meta() *Map {
	return self.meta
}

func (self *Builtin) Copy() Value {
	return self // immutable value
}

func (self *Builtin) CopyOnWrite() {
	// immutable value
}

func (self *Builtin) Hash() uint64 {
	return fnv1a(self.name)
}

func (self *Builtin) Equal(other Value) bool {
	othr, ok := other.(*Builtin)
	if !ok {
		return false
	}
	return self.name == othr.name && reflect.ValueOf(self.impl).Pointer() == reflect.ValueOf(othr.impl).Pointer()
}

func (self *Builtin) CombEncode(e *CombEncoder) error {
	if e.err == nil {
		e.err = fmt.Errorf("invalid comb value %s", self.String())
	}
	return e.err
}

type SourceLocation struct {
	File string
	Line int
}

func optionalSourceLocationIntoValue(ctx *Context, location *SourceLocation) Value {
	if location == nil {
		return ctx.Null
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("file"), ctx.NewString(location.File)},
		{ctx.NewString("line"), ctx.NewNumber(float64(location.Line))},
	})
}

type ParseError struct {
	Location *SourceLocation // Optional
	why      string
}

func (self ParseError) Error() string {
	return self.why
}

// Token Kinds
const (
	// Meta
	TOKEN_EOF = "end-of-file"
	// Identifiers and Literals
	TOKEN_IDENTIFIER = "identifier"
	TOKEN_TEMPLATE   = "template"
	TOKEN_NUMBER     = "number"
	TOKEN_STRING     = "string"
	TOKEN_REGEXP     = "regexp"
	// Operators
	TOKEN_ADD    = "+"
	TOKEN_SUB    = "-"
	TOKEN_MUL    = "*"
	TOKEN_DIV    = "/"
	TOKEN_REM    = "%"
	TOKEN_EQ     = "=="
	TOKEN_NE     = "!="
	TOKEN_LE     = "<="
	TOKEN_GE     = ">="
	TOKEN_LT     = "<"
	TOKEN_GT     = ">"
	TOKEN_EQ_RE  = "=~"
	TOKEN_NE_RE  = "!~"
	TOKEN_MKREF  = ".&"
	TOKEN_DEREF  = ".*"
	TOKEN_DOT    = "."
	TOKEN_SCOPE  = "::"
	TOKEN_ASSIGN = "="
	// Delimiters
	TOKEN_COMMA     = ","
	TOKEN_COLON     = ":"
	TOKEN_SEMICOLON = ";"
	TOKEN_LPAREN    = "("
	TOKEN_RPAREN    = ")"
	TOKEN_LBRACE    = "{"
	TOKEN_RBRACE    = "}"
	TOKEN_LBRACKET  = "["
	TOKEN_RBRACKET  = "]"
	// Keywords
	TOKEN_NULL     = "null"
	TOKEN_TRUE     = "true"
	TOKEN_FALSE    = "false"
	TOKEN_MAP      = "Map"
	TOKEN_SET      = "Set"
	TOKEN_NOT      = "not"
	TOKEN_AND      = "and"
	TOKEN_OR       = "or"
	TOKEN_FUNCTION = "function"
)

type Token struct {
	Kind     string
	Literal  string
	Location *SourceLocation // Optional
	Value    Value           // Optional
	Template []AstExpression // Optional
}

func (self Token) String() string {
	return self.Kind
}

func (self Token) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(self.Kind)},
		{ctx.NewString("literal"), ctx.NewString(self.Literal)},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
	})
}

type Lexer struct {
	ctx      *Context
	source   string
	location *SourceLocation // Optional
	position int

	keywords map[string]string
}

func NewLexer(ctx *Context, source string, location *SourceLocation) Lexer {
	keywords := map[string]string{
		TOKEN_NULL:     TOKEN_NULL,
		TOKEN_TRUE:     TOKEN_TRUE,
		TOKEN_FALSE:    TOKEN_FALSE,
		TOKEN_MAP:      TOKEN_MAP,
		TOKEN_SET:      TOKEN_SET,
		TOKEN_NOT:      TOKEN_NOT,
		TOKEN_AND:      TOKEN_AND,
		TOKEN_OR:       TOKEN_OR,
		TOKEN_FUNCTION: TOKEN_FUNCTION,
	}

	return Lexer{
		ctx:      ctx,
		source:   source,
		location: location,
		position: 0,

		keywords: keywords,
	}
}

func (self *Lexer) currentLocation() *SourceLocation {
	if self.location == nil {
		return nil
	}
	return &SourceLocation{self.location.File, self.location.Line}
}

func (self *Lexer) currentRune() rune {
	if self.position >= len(self.source) {
		return rune(0)
	}
	current, _ := utf8.DecodeRuneInString(self.source[self.position:])
	return current
}

func (self *Lexer) peekRune() rune {
	if self.position+1 >= len(self.source) {
		return rune(0)
	}
	_, currentSize := utf8.DecodeRuneInString(self.source[self.position:])
	peek, _ := utf8.DecodeRuneInString(self.source[self.position+currentSize:])
	return peek
}

func (self *Lexer) isEof() bool {
	return self.position >= len(self.source)
}

func (self *Lexer) remaining() string {
	return self.source[self.position:]
}

func (self *Lexer) advanceRune() {
	if self.isEof() {
		return
	}
	if self.location != nil && self.currentRune() == '\n' {
		self.location.Line += 1
	}
	_, size := utf8.DecodeRuneInString(self.source[self.position:])
	self.position += size
}

func (self *Lexer) expectRune(r rune) error {
	if self.isEof() {
		return ParseError{
			Location: self.currentLocation(),
			why:      fmt.Sprintf("expected %s, found end-of-file", quote(string([]rune{r}))),
		}
	}
	current := self.currentRune()
	if current != r {
		return ParseError{
			Location: self.currentLocation(),
			why:      fmt.Sprintf("expected %s, found %s", quote(string([]rune{r})), quote(string([]rune{current}))),
		}
	}
	self.advanceRune()
	return nil
}

func (self *Lexer) skipWhitespace() {
	for !self.isEof() && unicode.IsSpace(self.currentRune()) {
		self.advanceRune()
	}
}

func (self *Lexer) skipComment() {
	if self.currentRune() != '#' {
		return
	}
	for !self.isEof() && self.currentRune() != '\n' {
		self.advanceRune()
	}
	self.advanceRune()
}

func (self *Lexer) skipWhiteSpaceAndComments() {
	for !self.isEof() && (unicode.IsSpace(self.currentRune()) || self.currentRune() == '#') {
		self.skipWhitespace()
		self.skipComment()
	}
}

func (self *Lexer) lexKeywordOrIdentifier() (Token, error) {
	literal := ""
	for unicode.IsLetter(self.currentRune()) || self.currentRune() == '_' {
		literal += string(self.currentRune())
		self.advanceRune()
	}
	if len(literal) == 0 {
		return Token{}, errors.New("empty keyword or identifier")
	}

	keyword, ok := self.keywords[literal]
	if ok {
		return Token{
			Kind:     keyword,
			Literal:  literal,
			Location: self.currentLocation(),
		}, nil
	}
	return Token{
		Kind:     TOKEN_IDENTIFIER,
		Literal:  literal,
		Location: self.currentLocation(),
	}, nil
}

func (self *Lexer) lexNumber() (Token, error) {
	hex := self.ctx.reNumberHex.FindString(self.source[self.position:])
	if hex != "" {
		self.position += len(hex)
		parsed, err := strconv.ParseInt(hex, 0, 64)
		if err != nil {
			return Token{}, err
		}
		return Token{
			Kind:     TOKEN_NUMBER,
			Literal:  hex,
			Location: self.currentLocation(),
			Value:    self.ctx.NewNumber(float64(parsed)),
		}, nil
	}

	dec := self.ctx.reNumberDec.FindString(self.source[self.position:])
	if dec != "" {
		self.position += len(dec)
		parsed, err := strconv.ParseFloat(dec, 64)
		if err != nil {
			return Token{}, err
		}
		return Token{
			Kind:     TOKEN_NUMBER,
			Literal:  dec,
			Location: self.currentLocation(),
			Value:    self.ctx.NewNumber(parsed),
		}, nil
	}

	return Token{}, errors.New("invalid number")
}

func (self *Lexer) lexEscStringPart() ([]byte, error) {
	if self.isEof() {
		return nil, ParseError{
			Location: self.currentLocation(),
			why:      "expected character, found end-of-file",
		}
	}

	if self.currentRune() == '\n' {
		return nil, ParseError{
			Location: self.currentLocation(),
			why:      "expected character, found newline",
		}
	}

	if !unicode.IsPrint(self.currentRune()) {
		return nil, ParseError{
			Location: self.currentLocation(),
			why:      fmt.Sprintf("expected prinable character, found %#x", self.currentRune()),
		}
	}

	if self.currentRune() == '\\' && self.peekRune() == 't' {
		self.advanceRune()
		self.advanceRune()
		return []byte("\t"), nil
	}

	if self.currentRune() == '\\' && self.peekRune() == 'n' {
		self.advanceRune()
		self.advanceRune()
		return []byte("\n"), nil
	}

	if self.currentRune() == '\\' && self.peekRune() == '"' {
		self.advanceRune()
		self.advanceRune()
		return []byte("\""), nil
	}

	if self.currentRune() == '\\' && self.peekRune() == '\\' {
		self.advanceRune()
		self.advanceRune()
		return []byte("\\"), nil
	}

	if self.currentRune() == '\\' && self.peekRune() == 'x' {
		self.advanceRune()
		self.advanceRune()
		nybbles := []rune{self.currentRune(), self.peekRune()}
		self.advanceRune()
		self.advanceRune()
		mapping := map[rune]int{
			'0': 0x0,
			'1': 0x1,
			'2': 0x2,
			'3': 0x3,
			'4': 0x4,
			'5': 0x5,
			'6': 0x6,
			'7': 0x7,
			'8': 0x8,
			'9': 0x9,
			'A': 0xA,
			'B': 0xB,
			'C': 0xC,
			'D': 0xD,
			'E': 0xE,
			'F': 0xF,
			'a': 0xA,
			'b': 0xB,
			'c': 0xC,
			'd': 0xD,
			'e': 0xE,
			'f': 0xF,
		}
		nybble0, foundNybble0 := mapping[nybbles[0]]
		nybble1, foundNybble1 := mapping[nybbles[1]]
		if !(foundNybble0 && foundNybble1) {
			sequence := "\\x" + string(nybbles)
			return nil, ParseError{
				Location: self.currentLocation(),
				why:      fmt.Sprintf("expected hexadecimal escape sequence, found %s", quote(sequence)),
			}
		}
		return []byte{byte(nybble0<<4 | nybble1)}, nil
	}

	if self.currentRune() == '\\' {
		sequence := string([]rune{self.currentRune(), self.peekRune()})
		return nil, ParseError{
			Location: self.currentLocation(),
			why:      fmt.Sprintf("expected escape sequence, found %s", sequence),
		}
	}

	character := self.currentRune()
	self.advanceRune()
	return []byte(string(character)), nil
}

func (self *Lexer) lexEscString() (Token, error) {
	start := self.position
	if err := self.expectRune('"'); err != nil {
		return Token{}, err
	}
	bytes := []byte{}
	for !self.isEof() && self.currentRune() != '"' {
		part, err := self.lexEscStringPart()
		if err != nil {
			return Token{}, err
		}
		bytes = append(bytes, part...)
	}
	if err := self.expectRune('"'); err != nil {
		return Token{}, err
	}
	literal := self.source[start:self.position]
	return Token{
		Kind:     TOKEN_STRING,
		Literal:  literal,
		Location: self.currentLocation(),
		Value:    self.ctx.NewString(string(bytes)),
	}, nil
}

func (self *Lexer) lexRawStringPart() ([]byte, error) {
	if self.isEof() {
		return []byte{}, ParseError{
			Location: self.currentLocation(),
			why:      "expected character, found end-of-file",
		}
	}

	character := self.currentRune()
	self.advanceRune()
	return []byte(string(character)), nil
}

func (self *Lexer) lexRawString() (Token, error) {
	location := self.currentLocation()
	start := self.position
	bytes := []byte{}
	var literal string
	if strings.HasPrefix(self.remaining(), "```") {
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		for !self.isEof() && !strings.HasPrefix(self.remaining(), "```") {
			part, err := self.lexRawStringPart()
			if err != nil {
				return Token{}, err
			}
			bytes = append(bytes, part...)
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		literal = self.source[start+3 : self.position-3]
		// Future-proof in case I want to add variable-number-of-tick raw
		// string literals in the future.
		if len(bytes) == 0 {
			if err := self.expectRune('`'); err != nil {
				return Token{}, ParseError{
					Location: location,
					why:      "invalid empty multi-tick raw string",
				}
			}
		}
	} else {
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		for !self.isEof() && self.currentRune() != '`' {
			part, err := self.lexRawStringPart()
			if err != nil {
				return Token{}, err
			}
			bytes = append(bytes, part...)
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		literal = self.source[start:self.position]
	}
	return Token{
		Kind:     TOKEN_STRING,
		Literal:  literal,
		Location: location,
		Value:    self.ctx.NewString(string(bytes)),
	}, nil
}

func (self *Lexer) lexTemplate() (Token, error) {
	location := self.currentLocation()
	start := self.position
	if err := self.expectRune('$'); err != nil {
		return Token{}, err
	}

	template := []AstExpression{}
	bytes := []byte{} // current text being parsed

	lexTemplateElement := func(defaultFunc func() ([]byte, error)) error {
		if strings.HasPrefix(self.remaining(), "{{") {
			bytes = append(bytes, []byte("{")...)
			self.position += len("{{")
			return nil
		}

		if strings.HasPrefix(self.remaining(), "}}") {
			bytes = append(bytes, []byte("}")...)
			self.position += len("}}")
			return nil
		}

		if strings.HasPrefix(self.remaining(), "{") {
			if len(bytes) != 0 {
				template = append(template, AstExpressionString{location, self.ctx.NewString(string(bytes))})
			}
			bytes = []byte{}
			self.position += len("{")

			lexer := NewLexer(self.ctx, self.remaining(), nil)
			parser := NewParser(&lexer)
			expression, err := parser.ParseExpression()
			if err != nil {
				return ParseError{
					Location: location,
					why:      err.Error(),
				}
			}
			template = append(template, expression)

			if parser.currentToken.Kind != TOKEN_RBRACE {
				return ParseError{
					Location: location,
					why:      fmt.Sprintf("expected `}}` to close template expression, found %s", quote(parser.currentToken.Kind)),
				}
			}
			self.position += strings.LastIndex(self.remaining(), lexer.remaining())

			return nil
		}

		text, err := defaultFunc()
		if err != nil {
			return err
		}
		bytes = append(bytes, text...)
		return nil
	}

	if strings.HasPrefix(self.remaining(), "```") {
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		for !self.isEof() && !strings.HasPrefix(self.remaining(), "```") {
			err := lexTemplateElement(func() ([]byte, error) {
				return self.lexRawStringPart()
			})
			if err != nil {
				return Token{}, err
			}
		}
		if len(bytes) != 0 {
			template = append(template, AstExpressionString{location, self.ctx.NewString(string(bytes))})
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
	} else if strings.HasPrefix(self.remaining(), "`") {
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		for !self.isEof() && self.currentRune() != '`' {
			err := lexTemplateElement(func() ([]byte, error) {
				return self.lexRawStringPart()
			})
			if err != nil {
				return Token{}, err
			}
		}
		if len(bytes) != 0 {
			template = append(template, AstExpressionString{location, self.ctx.NewString(string(bytes))})
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
	} else if self.currentRune() == '"' {
		if err := self.expectRune('"'); err != nil {
			return Token{}, err
		}
		for self.currentRune() != '"' {
			err := lexTemplateElement(func() ([]byte, error) {
				return self.lexEscStringPart()
			})
			if err != nil {
				return Token{}, err
			}
		}
		if len(bytes) != 0 {
			template = append(template, AstExpressionString{location, self.ctx.NewString(string(bytes))})
		}
		if err := self.expectRune('"'); err != nil {
			return Token{}, err
		}
	} else {
		return Token{}, ParseError{
			Location: location,
			why:      fmt.Sprintf("expected template of the form $\"...\", $`...` or $```...```, found `$` followed by %s", quote(string(self.currentRune()))),
		}
	}

	literal := self.source[start:self.position]
	return Token{
		Kind:     TOKEN_TEMPLATE,
		Literal:  literal,
		Location: location,
		Template: template,
	}, nil
}

func (self *Lexer) lexRegexp() (Token, error) {
	location := self.currentLocation()
	start := self.position
	if err := self.expectRune('r'); err != nil {
		return Token{}, err
	}

	bytes := []byte{}
	if self.currentRune() == '"' {
		if err := self.expectRune('"'); err != nil {
			return Token{}, err
		}
		for self.currentRune() != '"' {
			part, err := self.lexEscStringPart()
			if err != nil {
				return Token{}, err
			}
			bytes = append(bytes, part...)
		}
		if err := self.expectRune('"'); err != nil {
			return Token{}, err
		}
	} else if self.currentRune() == '`' {
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
		for self.currentRune() != '`' {
			part, err := self.lexRawStringPart()
			if err != nil {
				return Token{}, err
			}
			bytes = append(bytes, part...)
		}
		if err := self.expectRune('`'); err != nil {
			return Token{}, err
		}
	} else {
		return Token{}, ParseError{
			Location: location,
			why:      fmt.Sprintf("expected %s or %s, found %s", quote("\""), quote("`"), quote(string(self.currentRune()))),
		}
	}
	literal := self.source[start:self.position]
	regexp, err := self.ctx.NewRegexp(string(bytes))
	if err != nil {
		return Token{}, ParseError{
			Location: location,
			why:      err.Error(),
		}
	}
	return Token{
		Kind:     TOKEN_REGEXP,
		Literal:  literal,
		Location: location,
		Value:    regexp,
	}, nil
}

func (self *Lexer) NextToken() (Token, error) {
	self.skipWhiteSpaceAndComments()
	if self.isEof() {
		return Token{
			Kind:     TOKEN_EOF,
			Literal:  "",
			Location: self.currentLocation(),
		}, nil
	}

	location := self.currentLocation()

	// Literals, Identifiers, and Keywords
	if '0' <= self.currentRune() && self.currentRune() <= '9' {
		return self.lexNumber()
	}
	if self.currentRune() == '"' {
		return self.lexEscString()
	}
	if self.currentRune() == '`' {
		return self.lexRawString()
	}
	if self.currentRune() == '$' {
		return self.lexTemplate()
	}
	if strings.HasPrefix(self.remaining(), "r\"") || strings.HasPrefix(self.remaining(), "r`") {
		return self.lexRegexp()
	}
	if unicode.IsLetter(self.currentRune()) || self.currentRune() == '_' {
		return self.lexKeywordOrIdentifier()
	}

	// Operators and Delimiters
	matchRemaining := func(kind string) (Token, bool) {
		if strings.HasPrefix(self.remaining(), kind) {
			literal := self.source[self.position : self.position+len(kind)]
			self.position += len(kind)
			return Token{
				Kind:     kind,
				Literal:  literal,
				Location: location,
			}, true
		}
		return Token{}, false
	}
	operatorsAndDelimiters := []string{
		// Operators
		TOKEN_ADD,
		TOKEN_SUB,
		TOKEN_MUL,
		TOKEN_DIV,
		TOKEN_REM,
		TOKEN_EQ,
		TOKEN_NE,
		TOKEN_LE,
		TOKEN_GE,
		TOKEN_LT,
		TOKEN_GT,
		TOKEN_EQ_RE,
		TOKEN_NE_RE,
		TOKEN_MKREF,
		TOKEN_DEREF,
		TOKEN_DOT,
		TOKEN_SCOPE,
		TOKEN_ASSIGN,
		// Delmimiters
		TOKEN_COMMA,
		TOKEN_COLON,
		TOKEN_SEMICOLON,
		TOKEN_LPAREN,
		TOKEN_RPAREN,
		TOKEN_LBRACE,
		TOKEN_RBRACE,
		TOKEN_LBRACKET,
		TOKEN_RBRACKET,
	}
	for _, d := range operatorsAndDelimiters {
		if token, match := matchRemaining(d); match {
			return token, nil
		}
	}

	return Token{}, ParseError{
		Location: self.currentLocation(),
		why:      fmt.Sprintf("unknown token %s", quote(string([]rune{self.currentRune()}))),
	}
}

type TraceElement struct {
	Location *SourceLocation // Optional
	FuncName string
}

type Error struct {
	Location *SourceLocation // Optional
	Value    Value
	Trace    []TraceElement
}

func (self Error) Error() string {
	return self.Value.String()
}

func NewError(location *SourceLocation, value Value) Error {
	return Error{
		Location: location,
		Value:    value,
		Trace:    []TraceElement{},
	}
}

type Environment struct {
	outer *Environment // Optional
	store map[string]Value
}

func NewEnvironment(outer *Environment) Environment {
	return Environment{
		outer: outer,
		store: map[string]Value{},
	}
}

func (self *Environment) Let(name string, value Value) {
	self.store[name] = value
}

func (self *Environment) Set(name string, value Value) error {
	env := self
	for env != nil {
		_, ok := self.store[name]
		if ok {
			self.store[name] = value
			return nil
		}
		env = env.outer
	}
	return fmt.Errorf("identifier %s is not defined", name)
}

func (self *Environment) Get(name string) (Value, error) {
	env := self
	for env != nil {
		value, ok := self.store[name]
		if ok {
			return value, nil
		}
		env = env.outer
	}
	return nil, fmt.Errorf("identifier %s is not defined", name)
}

type ControlFlow interface {
	ControlFlowLocation() *SourceLocation
}

type Return struct {
	Location *SourceLocation // Optional
	Value    Value
}

func (self Return) ControlFlowLocation() *SourceLocation {
	return self.Location
}

type Break struct {
	Location *SourceLocation // Optional
}

func (self Break) ControlFlowLocation() *SourceLocation {
	return self.Location
}

type Continue struct {
	Location *SourceLocation // Optional
}

func (self Continue) ControlFlowLocation() *SourceLocation {
	return self.Location
}

func Typename(value Value) string {
	if value.Meta() != nil && value.Meta().name != nil {
		return *value.Meta().name
	}
	return value.Typename()
}

type AstNode interface {
	IntoValue(*Context) Value
}

type AstExpression interface {
	ExpressionLocation() *SourceLocation
	IntoValue(*Context) Value
	Eval(*Context, *Environment) (Value, error)
}

type AstStatement interface {
	StatementLocation() *SourceLocation
	IntoValue(*Context) Value
	Eval(*Context, *Environment) (ControlFlow, error)
}

type AstProgram struct {
	Location   *SourceLocation // Optional
	Statements []AstStatement
}

func (self AstProgram) IntoValue(ctx *Context) Value {
	statements := ctx.NewVector(nil)
	for _, statement := range self.Statements {
		statements.Push(statement.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("statements"), statements},
	})
}

func (self AstProgram) Eval(ctx *Context, env *Environment) (Value, error) {
	var result Value = ctx.Null
	for _, statement := range self.Statements {
		if statementExpression, ok := statement.(AstStatementExpression); ok {
			// If a top-level statement is an expression, directly evaluate
			// that expression and save the result. This allows the result of
			// the last top-level expression statement to be used as the
			// result of program execution.
			value, error := statementExpression.Expression.Eval(ctx, env)
			if error != nil {
				return nil, error
			}
			result = value
			continue
		}

		cflow, error := statement.Eval(ctx, env)
		if error != nil {
			return nil, error
		}
		if result, ok := cflow.(Return); ok {
			return result.Value, nil
		}
		if result, ok := cflow.(Break); ok {
			return nil, NewError(result.Location, ctx.NewString("attempted to break outside of a loop"))
		}
		if result, ok := cflow.(Continue); ok {
			return nil, NewError(result.Location, ctx.NewString("attempted to continue outside of a loop"))
		}
	}
	return result, nil
}

type AstIdentifier struct {
	Location *SourceLocation // Optional
	Name     *String         // Cached
}

func (self AstIdentifier) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("name"), self.Name.Copy()},
	})
}

type AstExpressionIdentifier struct {
	Location *SourceLocation // Optional
	Name     *String         // Cached
}

func (self AstExpressionIdentifier) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionIdentifier) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("name"), self.Name.Copy()},
	})
}

func (self AstExpressionIdentifier) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := env.Get(self.Name.data)
	if err != nil {
		return nil, Error{self.Location, ctx.NewString(err.Error()), nil}
	}
	return value, err
}

type AstExpressionTemplate struct {
	Location *SourceLocation // Optional
	Template []AstExpression
}

func (self AstExpressionTemplate) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionTemplate) IntoValue(ctx *Context) Value {
	elements := []Value{}
	for _, element := range self.Template {
		elements = append(elements, element.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("template"), ctx.NewVector(elements)},
	})
}

func (self AstExpressionTemplate) Eval(ctx *Context, env *Environment) (Value, error) {
	output := []byte{}
	for _, element := range self.Template {
		value, err := element.Eval(ctx, env)
		if err != nil {
			return nil, err
		}

		// TODO: Handle into_string metafunction.

		if s, ok := value.(*String); ok {
			output = append(output, s.data...)
			continue
		}
		output = append(output, []byte(value.String())...)
	}
	return ctx.NewString(string(output)), nil
}

type AstExpressionNull struct {
	Location *SourceLocation // Optional
}

func (self AstExpressionNull) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionNull) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
	})
}

func (self AstExpressionNull) Eval(ctx *Context, env *Environment) (Value, error) {
	return ctx.Null.Copy(), nil
}

type AstExpressionBoolean struct {
	Location *SourceLocation // Optional
	Data     *Boolean
}

func (self AstExpressionBoolean) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionBoolean) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("data"), self.Data.Copy()},
	})
}

func (self AstExpressionBoolean) Eval(ctx *Context, env *Environment) (Value, error) {
	return self.Data.Copy(), nil
}

type AstExpressionNumber struct {
	Location *SourceLocation // Optional
	Data     *Number
}

func (self AstExpressionNumber) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionNumber) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("data"), self.Data.Copy()},
	})
}

func (self AstExpressionNumber) Eval(ctx *Context, env *Environment) (Value, error) {
	return self.Data.Copy(), nil
}

type AstExpressionString struct {
	Location *SourceLocation // Optional
	Data     *String
}

func (self AstExpressionString) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionString) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("data"), self.Data.Copy()},
	})
}

func (self AstExpressionString) Eval(ctx *Context, env *Environment) (Value, error) {
	return self.Data.Copy(), nil
}

type AstExpressionRegexp struct {
	Location *SourceLocation // Optional
	Data     *Regexp
}

func (self AstExpressionRegexp) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionRegexp) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("data"), ctx.NewString(self.Data.String())},
	})
}

func (self AstExpressionRegexp) Eval(ctx *Context, env *Environment) (Value, error) {
	return self.Data.Copy(), nil
}

type AstExpressionVector struct {
	Location *SourceLocation // Optional
	Elements []AstExpression
}

func (self AstExpressionVector) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionVector) IntoValue(ctx *Context) Value {
	elements := []Value{}
	for _, element := range self.Elements {
		elements = append(elements, element.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("elements"), ctx.NewVector(elements)},
	})
}

func (self AstExpressionVector) Eval(ctx *Context, env *Environment) (Value, error) {
	elements := []Value{}
	for _, element := range self.Elements {
		value, err := element.Eval(ctx, env)
		if err != nil {
			return nil, err
		}
		elements = append(elements, value.Copy())
	}
	return ctx.NewVector(elements), nil
}

type AstMapPair struct {
	Key   AstExpression
	Value AstExpression
}

type AstExpressionMap struct {
	Location *SourceLocation // Optional
	Elements []AstMapPair
}

func (self AstExpressionMap) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionMap) IntoValue(ctx *Context) Value {
	elements := []Value{}
	for _, element := range self.Elements {
		elements = append(elements,
			ctx.NewVector([]Value{element.Key.IntoValue(ctx), element.Value.IntoValue(ctx)}))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("elements"), ctx.NewVector(elements)},
	})
}

func (self AstExpressionMap) Eval(ctx *Context, env *Environment) (Value, error) {
	pairs := []MapPair{}
	for _, element := range self.Elements {
		k, err := element.Key.Eval(ctx, env)
		if err != nil {
			return nil, err
		}
		v, err := element.Value.Eval(ctx, env)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, MapPair{k.Copy(), v.Copy()})
	}
	return ctx.NewMap(pairs), nil
}

type AstExpressionSet struct {
	Location *SourceLocation // Optional
	Elements []AstExpression
}

func (self AstExpressionSet) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionSet) IntoValue(ctx *Context) Value {
	elements := []Value{}
	for _, element := range self.Elements {
		elements = append(elements, element.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("elements"), ctx.NewVector(elements)},
	})
}

func (self AstExpressionSet) Eval(ctx *Context, env *Environment) (Value, error) {
	elements := []Value{}
	for _, element := range self.Elements {
		value, err := element.Eval(ctx, env)
		if err != nil {
			return nil, err
		}
		elements = append(elements, value.Copy())
	}
	return ctx.NewSet(elements), nil
}

type AstExpressionFunction struct {
	Location   *SourceLocation // Optional
	Parameters []AstIdentifier
	Body       AstBlock
	Name       *String // optional
}

func (self AstExpressionFunction) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionFunction) IntoValue(ctx *Context) Value {
	parameters := ctx.NewVector(nil)
	for _, parameter := range self.Parameters {
		parameters.Push(parameter.IntoValue(ctx))
	}
	var name Value = ctx.NewNull()
	if self.Name != nil {
		name = self.Name.Copy()
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("paramters"), parameters},
		{ctx.NewString("body"), self.Body.IntoValue(ctx)},
		{ctx.NewString("name"), name},
	})
}

func (self *AstExpressionFunction) Eval(ctx *Context, env *Environment) (Value, error) {
	return ctx.NewFunction(self, env), nil
}

type AstExpressionGrouped struct {
	Location   *SourceLocation // Optional
	Expression AstExpression
}

func (self AstExpressionGrouped) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionGrouped) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("expression"), self.Expression.IntoValue(ctx)},
	})
}

func (self AstExpressionGrouped) Eval(ctx *Context, env *Environment) (Value, error) {
	return self.Expression.Eval(ctx, env)
}

type AstExpressionPositive struct {
	Location   *SourceLocation // Optional
	Expression AstExpression
}

func (self AstExpressionPositive) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionPositive) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("expression"), self.Expression.IntoValue(ctx)},
	})
}

func (self AstExpressionPositive) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := self.Expression.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	if number, ok := value.(*Number); ok {
		return ctx.NewNumber(+number.data), nil
	}

	return nil, NewError(
		self.Location,
		ctx.NewString(fmt.Sprintf("attempted unary + operation with type %s", quote(Typename(value)))),
	)
}

type AstExpressionNegative struct {
	Location   *SourceLocation // Optional
	Expression AstExpression
}

func (self AstExpressionNegative) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionNegative) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("expression"), self.Expression.IntoValue(ctx)},
	})
}

func (self AstExpressionNegative) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := self.Expression.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	if number, ok := value.(*Number); ok {
		return ctx.NewNumber(-number.data), nil
	}

	return nil, NewError(
		self.Location,
		ctx.NewString(fmt.Sprintf("attempted unary - operation with type %s", quote(Typename(value)))),
	)
}

type AstExpressionNot struct {
	Location   *SourceLocation // Optional
	Expression AstExpression
}

func (self AstExpressionNot) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionNot) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("expression"), self.Expression.IntoValue(ctx)},
	})
}

func (self AstExpressionNot) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := self.Expression.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	if boolean, ok := value.(*Boolean); ok {
		return ctx.NewBoolean(!boolean.data), nil
	}

	return nil, NewError(
		self.Location,
		ctx.NewString(fmt.Sprintf("attempted unary not operation with type %s", quote(Typename(value)))),
	)
}

type AstExpressionAccessIndex struct {
	Location *SourceLocation // Optional
	Store    AstExpression
	Field    AstExpression
}

func (self AstExpressionAccessIndex) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionAccessIndex) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("store"), self.Store.IntoValue(ctx)},
		{ctx.NewString("field"), self.Field.IntoValue(ctx)},
	})
}

func (self AstExpressionAccessIndex) Eval(ctx *Context, env *Environment) (Value, error) {
	store, err := self.Store.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	field, err := self.Field.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	if v, ok := store.(*Vector); ok {
		integer, err := ValueAsInt(field)
		if err != nil {
			return nil, fmt.Errorf("invalid vector access with field %v", field)
		}
		return v.Get(integer), nil
	}
	if m, ok := store.(*Map); ok {
		lookup := m.Lookup(field)
		if lookup == nil {
			return nil, fmt.Errorf("invalid map access with field %v", field)
		}
		return lookup, nil
	}

	return nil, NewError(
		self.Location,
		ctx.NewString(fmt.Sprintf("attempted to access field of type %s with type %s", quote(Typename(store)), quote(Typename(field)))),
	)
}

type AstExpressionAccessScope struct {
	Location *SourceLocation // Optional
	Store    AstExpression
	Field    AstIdentifier
}

func (self AstExpressionAccessScope) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionAccessScope) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("store"), self.Store.IntoValue(ctx)},
		{ctx.NewString("field"), self.Field.IntoValue(ctx)},
	})
}

func (self AstExpressionAccessScope) Eval(ctx *Context, env *Environment) (Value, error) {
	store, err := self.Store.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	field := self.Field.Name
	if err != nil {
		return nil, err
	}

	m, ok := store.(*Map)
	if !ok {
		return nil, NewError(
			self.Location,
			ctx.NewString(fmt.Sprintf("attempted to access field of type %s", quote(Typename(store)))),
		)
	}
	lookup := m.Lookup(field)
	if lookup == nil {
		return nil, fmt.Errorf("invalid map access with field %v", field)
	}

	return lookup, nil
}

type AstExpressionAccessDot struct {
	Location *SourceLocation // Optional
	Store    AstExpression
	Field    AstIdentifier
}

func (self AstExpressionAccessDot) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionAccessDot) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("store"), self.Store.IntoValue(ctx)},
		{ctx.NewString("field"), self.Field.IntoValue(ctx)},
	})
}

func (self AstExpressionAccessDot) Eval(ctx *Context, env *Environment) (Value, error) {
	store, err := self.Store.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	field := self.Field.Name
	if err != nil {
		return nil, err
	}

	// When directly reading property via dot access, prioritize the fields
	// of value itself *before* looking at the fields of the value's
	// metamap. This is done so that operations such as:
	//
	//   somemap.foo
	//
	// will find the field "foo" rather than a metafunction `foo` in the
	// map, which is almost certainly the desired behavior for nominal
	// property lookup.
	if m, ok := store.(*Map); ok {
		lookup := m.Lookup(field)
		if lookup != nil {
			return lookup, nil
		}

		if m.meta != nil {
			lookup = m.meta.Lookup(field)
			if lookup != nil {
				return lookup, nil
			}
		}
	}

	// Special case where a reference value is implicitly dereferenced when
	// accessing the target field.
	if reference, ok := store.(*Reference); ok {
		derefStore := reference.data

		// Prioritize fields of the value itself *before* looking at the
		// fields of the value's metamap.
		if m, ok := derefStore.(*Map); ok {
			lookup := m.Lookup(field)
			if lookup != nil {
				return lookup, nil
			}

			if m.meta != nil {
				lookup = m.meta.Lookup(field)
				if lookup != nil {
					return lookup, nil
				}
			}

			return nil, fmt.Errorf("invalid %s to %s access with field %v", store.Typename(), derefStore.Typename(), field)
		}
	}

	return nil, fmt.Errorf("invalid %s access with field %v", store.Typename(), field)
}

type AstExpressionMkref struct {
	Location *SourceLocation // Optional
	Lhs      AstExpression
}

func (self AstExpressionMkref) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionMkref) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("lhs"), self.Lhs.IntoValue(ctx)},
	})
}

func (self AstExpressionMkref) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := self.Lhs.Eval(ctx, env)
	if err != nil {
		return nil, err
	}
	return ctx.NewReference(value), nil
}

type AstExpressionDeref struct {
	Location *SourceLocation // Optional
	Lhs      AstExpression
}

func (self AstExpressionDeref) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionDeref) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("lhs"), self.Lhs.IntoValue(ctx)},
	})
}

func (self AstExpressionDeref) Eval(ctx *Context, env *Environment) (Value, error) {
	value, err := self.Lhs.Eval(ctx, env)
	if err != nil {
		return nil, err
	}
	return ctx.NewReference(value), nil
}

type AstExpressionFunctionCall struct {
	Location  *SourceLocation // Optional
	Function  AstExpression
	Arguments []AstExpression
}

func (self AstExpressionFunctionCall) ExpressionLocation() *SourceLocation {
	return self.Location
}

func (self AstExpressionFunctionCall) IntoValue(ctx *Context) Value {
	arguments := ctx.NewVector(nil)
	for _, argument := range self.Arguments {
		arguments.Push(argument.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("function"), self.Function.IntoValue(ctx)},
		{ctx.NewString("arguments"), arguments},
	})
}

func (self AstExpressionFunctionCall) Eval(ctx *Context, env *Environment) (Value, error) {
	// TODO: Handle case with dot access passing an implicit self parameter.

	function, err := self.Function.Eval(ctx, env)
	if err != nil {
		return nil, err
	}

	arguments := []Value{}
	// TODO: Insert self argument once implicit self argument is handled.
	for _, argument := range self.Arguments {
		result, err := argument.Eval(ctx, env)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, result)
	}

	return Call(ctx, self.Location, function, arguments)
}

type AstBlock struct {
	Location   *SourceLocation // Optional
	Statements []AstStatement
}

func (self AstBlock) IntoValue(ctx *Context) Value {
	statements := ctx.NewVector(nil)
	for _, statement := range self.Statements {
		statements.Push(statement.IntoValue(ctx))
	}
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("statements"), statements},
	})
}

func (self AstBlock) Eval(ctx *Context, env *Environment) (ControlFlow, error) {
	for _, statement := range self.Statements {
		cflow, error := statement.Eval(ctx, env)
		if error != nil {
			return nil, error
		}
		if cflow != nil {
			return cflow, nil
		}
	}
	return nil, nil
}

type AstStatementExpression struct {
	Location   *SourceLocation // Optional
	Expression AstExpression
}

func (self AstStatementExpression) StatementLocation() *SourceLocation {
	return self.Location
}

func (self AstStatementExpression) IntoValue(ctx *Context) Value {
	return ctx.NewMap([]MapPair{
		{ctx.NewString("kind"), ctx.NewString(reflect.TypeOf(self).Name())},
		{ctx.NewString("location"), optionalSourceLocationIntoValue(ctx, self.Location)},
		{ctx.NewString("expression"), self.Expression.IntoValue(ctx)},
	})
}

func (self AstStatementExpression) Eval(ctx *Context, env *Environment) (ControlFlow, error) {
	_, error := self.Expression.Eval(ctx, env)
	if error != nil {
		return nil, error
	}
	return nil, nil
}

// Precedence Levels
const (
	PRECEDENCE_LOWEST  = iota
	PRECEDENCE_OR      = iota // or
	PRECEDENCE_AND     = iota // and
	PRECEDENCE_COMPARE = iota // == != <= >= < > =~ !~
	PRECEDENCE_ADD_SUB = iota // + -
	PRECEDENCE_MUL_DIV = iota // * /
	PRECEDENCE_PREFIX  = iota // +x -x
	PRECEDENCE_POSTFIX = iota // foo(bar, 123) foo[42] .& .*
)

type Parser struct {
	lexer        *Lexer
	currentToken Token

	precedences       map[string]int
	parseNudFunctions map[string]func(*Parser) (AstExpression, error)
	parseLedFunctions map[string]func(*Parser, AstExpression) (AstExpression, error)
}

func NewParser(lexer *Lexer) Parser {
	self := Parser{
		lexer:        lexer,
		currentToken: Token{"invalid program", "", lexer.location, nil, nil},

		precedences: map[string]int{
			TOKEN_OR:       PRECEDENCE_OR,
			TOKEN_AND:      PRECEDENCE_AND,
			TOKEN_EQ:       PRECEDENCE_COMPARE,
			TOKEN_NE:       PRECEDENCE_COMPARE,
			TOKEN_LE:       PRECEDENCE_COMPARE,
			TOKEN_GE:       PRECEDENCE_COMPARE,
			TOKEN_LT:       PRECEDENCE_COMPARE,
			TOKEN_GT:       PRECEDENCE_COMPARE,
			TOKEN_EQ_RE:    PRECEDENCE_COMPARE,
			TOKEN_NE_RE:    PRECEDENCE_COMPARE,
			TOKEN_ADD:      PRECEDENCE_ADD_SUB,
			TOKEN_SUB:      PRECEDENCE_ADD_SUB,
			TOKEN_MUL:      PRECEDENCE_MUL_DIV,
			TOKEN_DIV:      PRECEDENCE_MUL_DIV,
			TOKEN_REM:      PRECEDENCE_MUL_DIV,
			TOKEN_LPAREN:   PRECEDENCE_POSTFIX,
			TOKEN_LBRACKET: PRECEDENCE_POSTFIX,
			TOKEN_SCOPE:    PRECEDENCE_POSTFIX,
			TOKEN_DOT:      PRECEDENCE_POSTFIX,
			TOKEN_MKREF:    PRECEDENCE_POSTFIX,
			TOKEN_DEREF:    PRECEDENCE_POSTFIX,
		},
		parseNudFunctions: map[string]func(*Parser) (AstExpression, error){
			TOKEN_IDENTIFIER: (*Parser).ParseExpressionIdentifier,
			TOKEN_TEMPLATE:   (*Parser).ParseExpressionTemplate,
			TOKEN_NULL:       (*Parser).ParseExpressionNull,
			TOKEN_TRUE:       (*Parser).ParseExpressionBoolean,
			TOKEN_FALSE:      (*Parser).ParseExpressionBoolean,
			TOKEN_NUMBER:     (*Parser).ParseExpressionNumber,
			TOKEN_STRING:     (*Parser).ParseExpressionString,
			TOKEN_REGEXP:     (*Parser).ParseExpressionRegexp,
			TOKEN_LBRACKET:   (*Parser).ParseExpressionVector,
			TOKEN_MAP:        (*Parser).ParseExpressionMapOrSet,
			TOKEN_SET:        (*Parser).ParseExpressionMapOrSet,
			TOKEN_LBRACE:     (*Parser).ParseExpressionMapOrSet,
			TOKEN_FUNCTION:   (*Parser).ParseExpressionFunction,
			TOKEN_LPAREN:     (*Parser).ParseExpressionGrouped,
			TOKEN_ADD:        (*Parser).ParseExpressionPositive,
			TOKEN_SUB:        (*Parser).ParseExpressionNegative,
			TOKEN_NOT:        (*Parser).ParseExpressionNot,
		},
		parseLedFunctions: map[string]func(*Parser, AstExpression) (AstExpression, error){
			TOKEN_LPAREN:   (*Parser).ParseExpressionFunctionCall,
			TOKEN_LBRACKET: (*Parser).ParseExpressionAccessIndex,
			TOKEN_SCOPE:    (*Parser).ParseExpressionAccessScope,
			TOKEN_DOT:      (*Parser).ParseExpressionAccessDot,
			TOKEN_MKREF:    (*Parser).ParseExpressionMkref,
			TOKEN_DEREF:    (*Parser).ParseExpressionDeref,
		},
	}
	self.advanceToken()
	return self
}

func (self *Parser) context() *Context {
	return self.lexer.ctx
}

func (self *Parser) advanceToken() (Token, error) {
	current := self.currentToken
	token, err := self.lexer.NextToken()
	if err != nil {
		return token, err
	}
	self.currentToken = token
	return current, nil
}

func (self *Parser) checkCurrent(kind string) bool {
	return self.currentToken.Kind == kind
}

func (self *Parser) expectCurrent(kind string) (Token, error) {
	current := self.currentToken
	if current.Kind != kind {
		return Token{}, ParseError{
			current.Location,
			fmt.Sprintf("expected %s, found %s", quote(kind), quote(current.String())),
		}
	}
	if _, err := self.advanceToken(); err != nil {
		return Token{}, err
	}
	return current, nil
}

func (self *Parser) identifier(name string) *String {
	cached, ok := self.lexer.ctx.identifierCache[name]
	if ok {
		return cached
	}
	self.lexer.ctx.identifierCache[name] = self.lexer.ctx.NewString(name)
	return self.lexer.ctx.identifierCache[name]
}

func (self *Parser) ParseProgram() (AstProgram, error) {
	location := self.currentToken.Location
	statements := []AstStatement{}
	for !self.checkCurrent(TOKEN_EOF) {
		statement, err := self.ParseStatement()
		if err != nil {
			return AstProgram{}, err
		}
		statements = append(statements, statement)
	}
	return AstProgram{location, statements}, nil
}

func (self *Parser) ParseIdentifier() (AstIdentifier, error) {
	token, err := self.expectCurrent(TOKEN_IDENTIFIER)
	if err != nil {
		return AstIdentifier{}, err
	}
	return AstIdentifier{token.Location, self.identifier(token.Literal)}, nil
}

func (self *Parser) ParseExpression() (AstExpression, error) {
	return self.parseExpression(PRECEDENCE_LOWEST)
}

func (self *Parser) parseExpression(precedence int) (AstExpression, error) {
	parseNud, ok := self.parseNudFunctions[self.currentToken.Kind]
	if !ok {
		return nil, ParseError{
			self.currentToken.Location,
			fmt.Sprintf("expected expression, found %v", self.currentToken),
		}
	}
	expression, err := parseNud(self)
	if err != nil {
		return nil, err
	}

	getPrecedence := func(kind string) int {
		precedence, ok := self.precedences[kind]
		if !ok {
			return PRECEDENCE_LOWEST
		}
		return precedence
	}

	for precedence < getPrecedence(self.currentToken.Kind) {
		parseLed, ok := self.parseLedFunctions[self.currentToken.Kind]
		if !ok {
			return expression, nil
		}
		expression, err = parseLed(self, expression)
		if err != nil {
			return nil, err
		}
	}

	return expression, nil
}

func (self *Parser) ParseExpressionIdentifier() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_IDENTIFIER)
	if err != nil {
		return nil, err
	}
	return AstExpressionIdentifier{token.Location, self.identifier(token.Literal)}, nil
}

func (self *Parser) ParseExpressionTemplate() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_TEMPLATE)
	if err != nil {
		return nil, err
	}
	return AstExpressionTemplate{token.Location, token.Template}, nil
}

func (self *Parser) ParseExpressionNull() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_NULL)
	if err != nil {
		return nil, err
	}
	return AstExpressionNull{token.Location}, nil
}

func (self *Parser) ParseExpressionBoolean() (AstExpression, error) {
	if self.checkCurrent(TOKEN_TRUE) {
		token, err := self.expectCurrent(TOKEN_TRUE)
		if err != nil {
			return nil, err
		}
		return AstExpressionBoolean{token.Location, self.context().NewBoolean(true)}, nil
	}
	if self.checkCurrent(TOKEN_FALSE) {
		token, err := self.expectCurrent(TOKEN_FALSE)
		if err != nil {
			return nil, err
		}
		return AstExpressionBoolean{token.Location, self.context().NewBoolean(false)}, nil
	}
	return nil, ParseError{
		self.currentToken.Location,
		fmt.Sprintf("expected boolean, found %v", self.currentToken),
	}
}

func (self *Parser) ParseExpressionNumber() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_NUMBER)
	if err != nil {
		return nil, ParseError{
			self.currentToken.Location,
			fmt.Sprintf("expected number, found %v", self.currentToken),
		}
	}
	value, ok := token.Value.(*Number)
	if !ok {
		return nil, errors.New("missing number token value")
	}
	return AstExpressionNumber{token.Location, value}, nil
}

func (self *Parser) ParseExpressionString() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_STRING)
	if err != nil {
		return nil, ParseError{
			self.currentToken.Location,
			fmt.Sprintf("expected string, found %v", self.currentToken),
		}
	}
	value, ok := token.Value.(*String)
	if !ok {
		return nil, errors.New("missing string token value")
	}
	return AstExpressionString{token.Location, value}, nil
}

func (self *Parser) ParseExpressionRegexp() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_REGEXP)
	if err != nil {
		return nil, ParseError{
			self.currentToken.Location,
			fmt.Sprintf("expected regexp, found %v", self.currentToken),
		}
	}
	value, ok := token.Value.(*Regexp)
	if !ok {
		return nil, errors.New("missing regexp token value")
	}
	return AstExpressionRegexp{token.Location, value}, nil
}

func (self *Parser) ParseExpressionVector() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_LBRACKET)
	if err != nil {
		return nil, err
	}
	location := token.Location

	elements := []AstExpression{}
	for !self.checkCurrent(TOKEN_RBRACKET) {
		if len(elements) != 0 {
			if _, err := self.expectCurrent(TOKEN_COMMA); err != nil {
				return nil, err
			}
		}
		if self.checkCurrent(TOKEN_RBRACKET) {
			break
		}

		expression, err := self.ParseExpression()
		if err != nil {
			return nil, err
		}
		elements = append(elements, expression)
	}

	if _, err := self.expectCurrent(TOKEN_RBRACKET); err != nil {
		return nil, err
	}

	return AstExpressionVector{location, elements}, nil
}

func (self *Parser) ParseExpressionMapOrSet() (AstExpression, error) {
	var mapOrSet string = "UNKNOWN"
	var location *SourceLocation
	if self.checkCurrent(TOKEN_MAP) {
		mapOrSet = TOKEN_MAP
		location = self.currentToken.Location
		self.advanceToken()
		_, err := self.expectCurrent(TOKEN_LBRACE)
		if err != nil {
			return nil, err
		}
	} else if self.checkCurrent(TOKEN_SET) {
		mapOrSet = TOKEN_SET
		location = self.currentToken.Location
		self.advanceToken()
		_, err := self.expectCurrent(TOKEN_LBRACE)
		if err != nil {
			return nil, err
		}
	} else {
		token, err := self.expectCurrent(TOKEN_LBRACE)
		if err != nil {
			return nil, err
		}
		location = token.Location
	}

	var mapElements []AstMapPair
	var setElements []AstExpression
	for !self.checkCurrent(TOKEN_RBRACE) {
		if len(mapElements) != 0 || len(setElements) != 0 {
			if _, err := self.expectCurrent(TOKEN_COMMA); err != nil {
				return nil, err
			}
		}
		if self.checkCurrent(TOKEN_RBRACE) {
			break
		}

		var expression AstExpression
		if self.checkCurrent(TOKEN_DOT) {
			if mapOrSet == "UNKNOWN" {
				mapOrSet = TOKEN_MAP
			} else if mapOrSet == TOKEN_SET {
				return nil, ParseError{
					Location: location,
					why:      fmt.Sprintf("expected expression, found %s", self.currentToken.Kind),
				}
			}
			_, err := self.expectCurrent(TOKEN_DOT)
			if err != nil {
				return nil, err
			}
			identifier, err := self.ParseExpressionIdentifier()
			if err != nil {
				return nil, err
			}
			expression = AstExpressionString{identifier.(AstExpressionIdentifier).Location, identifier.(AstExpressionIdentifier).Name}
		} else {
			element, err := self.ParseExpression()
			if err != nil {
				return nil, err
			}
			expression = element
		}

		if mapOrSet == "UNKNOWN" {
			if self.checkCurrent(TOKEN_COLON) || self.checkCurrent(TOKEN_ASSIGN) {
				mapOrSet = TOKEN_MAP
			} else {
				mapOrSet = TOKEN_SET
			}
		}

		if mapOrSet == TOKEN_MAP {
			if self.checkCurrent(TOKEN_COLON) {
				_, err := self.expectCurrent(TOKEN_COLON)
				if err != nil {
					return nil, err
				}
			} else if self.checkCurrent(TOKEN_ASSIGN) {
				_, err := self.expectCurrent(TOKEN_ASSIGN)
				if err != nil {
					return nil, err
				}
			} else {
				return nil, ParseError{
					Location: location,
					why:      fmt.Sprintf("expected %s or %s, found %s", TOKEN_COLON, TOKEN_ASSIGN, self.currentToken.Kind),
				}
			}

			key := expression
			value, err := self.ParseExpression()
			if err != nil {
				return nil, err
			}
			mapElements = append(mapElements, AstMapPair{key, value})
		} else if mapOrSet == TOKEN_SET {
			setElements = append(setElements, expression)
		}
	}

	if _, err := self.expectCurrent(TOKEN_RBRACE); err != nil {
		return nil, err
	}

	if mapOrSet == TOKEN_MAP {
		return AstExpressionMap{location, mapElements}, nil
	}
	if mapOrSet == TOKEN_SET {
		return AstExpressionSet{location, setElements}, nil
	}
	return nil, ParseError{
		Location: location,
		why:      "ambiguous empty map or set",
	}
}

func (self *Parser) ParseExpressionFunction() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_FUNCTION)
	if err != nil {
		return nil, err
	}
	location := token.Location

	_, err = self.expectCurrent(TOKEN_LPAREN)
	if err != nil {
		return nil, err
	}

	parameters := []AstIdentifier{}
	for !self.checkCurrent(TOKEN_RPAREN) {
		if len(parameters) != 0 {
			if _, err := self.expectCurrent(TOKEN_COMMA); err != nil {
				return nil, err
			}
		}

		identifier, err := self.ParseIdentifier()
		if err != nil {
			return nil, err
		}
		parameters = append(parameters, identifier)
	}

	if _, err := self.expectCurrent(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	body, err := self.ParseBlock()
	if err != nil {
		return nil, err
	}

	for i := range parameters {
		for j := i + 1; j < len(parameters); j++ {
			if parameters[i].Name == parameters[j].Name {
				return nil, ParseError{
					Location: location,
					why:      fmt.Sprintf("duplicate function parameter %s", quote(parameters[i].Name.data)),
				}
			}
		}
	}

	return &AstExpressionFunction{location, parameters, body, nil}, nil
}

func (self *Parser) ParseExpressionGrouped() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_LPAREN)
	if err != nil {
		return nil, err
	}
	location := token.Location

	expression, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}

	_, err = self.expectCurrent(TOKEN_RPAREN)
	if err != nil {
		return nil, err
	}

	return AstExpressionGrouped{location, expression}, nil
}

func (self *Parser) ParseExpressionPositive() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_ADD)
	if err != nil {
		return nil, err
	}
	location := token.Location

	expression, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}

	return AstExpressionPositive{location, expression}, nil
}

func (self *Parser) ParseExpressionNegative() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_SUB)
	if err != nil {
		return nil, err
	}
	location := token.Location

	expression, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}

	return AstExpressionNegative{location, expression}, nil
}

func (self *Parser) ParseExpressionNot() (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_NOT)
	if err != nil {
		return nil, err
	}
	location := token.Location

	expression, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}

	return AstExpressionNot{location, expression}, nil
}

func (self *Parser) ParseExpressionFunctionCall(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_LPAREN)
	if err != nil {
		return nil, err
	}
	location := token.Location

	arguments := []AstExpression{}
	for !self.checkCurrent(TOKEN_RPAREN) {
		if len(arguments) != 0 {
			if _, err := self.expectCurrent(TOKEN_COMMA); err != nil {
				return nil, err
			}
		}
		if self.checkCurrent(TOKEN_RPAREN) {
			break
		}

		expression, err := self.ParseExpression()
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, expression)
	}

	if _, err := self.expectCurrent(TOKEN_RPAREN); err != nil {
		return nil, err
	}

	return AstExpressionFunctionCall{location, lhs, arguments}, nil
}

func (self *Parser) ParseExpressionAccessIndex(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_LBRACKET)
	if err != nil {
		return nil, err
	}
	field, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}
	_, err = self.expectCurrent(TOKEN_RBRACKET)
	if err != nil {
		return nil, err
	}
	return AstExpressionAccessIndex{token.Location, lhs, field}, nil
}

func (self *Parser) ParseExpressionAccessScope(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_SCOPE)
	if err != nil {
		return nil, err
	}
	field, err := self.ParseIdentifier()
	if err != nil {
		return nil, err
	}
	return AstExpressionAccessScope{token.Location, lhs, field}, nil
}

func (self *Parser) ParseExpressionAccessDot(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_DOT)
	if err != nil {
		return nil, err
	}
	field, err := self.ParseIdentifier()
	if err != nil {
		return nil, err
	}
	return AstExpressionAccessDot{token.Location, lhs, field}, nil
}

func (self *Parser) ParseExpressionMkref(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_MKREF)
	if err != nil {
		return nil, err
	}
	return AstExpressionMkref{token.Location, lhs}, nil
}

func (self *Parser) ParseExpressionDeref(lhs AstExpression) (AstExpression, error) {
	token, err := self.expectCurrent(TOKEN_DEREF)
	if err != nil {
		return nil, err
	}
	return AstExpressionDeref{token.Location, lhs}, nil
}

func (self *Parser) ParseBlock() (AstBlock, error) {
	token, err := self.expectCurrent(TOKEN_LBRACE)
	if err != nil {
		return AstBlock{}, err
	}

	statements := []AstStatement{}
	for !self.checkCurrent(TOKEN_RBRACE) {
		statement, err := self.ParseStatement()
		if err != nil {
			return AstBlock{}, err
		}
		statements = append(statements, statement)
	}

	_, err = self.expectCurrent(TOKEN_RBRACE)
	if err != nil {
		return AstBlock{}, err
	}

	return AstBlock{token.Location, statements}, nil
}

func (self *Parser) ParseStatement() (AstStatement, error) {
	return self.ParseStatementExpressionOrAssignment()
}

func (self *Parser) ParseStatementExpressionOrAssignment() (AstStatement, error) {
	expression, err := self.ParseExpression()
	if err != nil {
		return nil, err
	}

	_, err = self.expectCurrent(TOKEN_SEMICOLON)
	if err != nil {
		return nil, err
	}

	// TODO: Parse statement assignment.
	return AstStatementExpression{expression.ExpressionLocation(), expression}, nil
}

func Call(ctx *Context, location *SourceLocation, callable Value, arguments []Value) (Value, error) {
	if function, ok := callable.(*Function); ok {
		if len(arguments) != len(function.Ast.Parameters) {
			return nil, NewError(
				location,
				ctx.NewString(fmt.Sprintf("invalid function argument count (expected %v, received %v)", len(function.Ast.Parameters), len(arguments))),
			)
		}

		env := NewEnvironment(function.Env)
		for i, parameter := range function.Ast.Parameters {
			env.Let(parameter.Name.data, arguments[i])
		}

		result, err := function.Ast.Body.Eval(ctx, &env)
		if err != nil {
			if error, ok := err.(*Error); ok {
				error.Trace = append(error.Trace, TraceElement{location, function.String()})
			}
			return nil, err
		}

		if flow, ok := result.(*Return); ok {
			return flow.Value, nil
		}
		if _, ok := result.(*Break); ok {
			return nil, NewError(
				result.ControlFlowLocation(),
				ctx.NewString("attempted to break outside of a loop"),
			)
		}
		if _, ok := result.(*Continue); ok {
			return nil, NewError(
				result.ControlFlowLocation(),
				ctx.NewString("attempted to continue outside of a loop"),
			)
		}

		return ctx.NewNull(), nil
	}

	if builtin, ok := callable.(*Builtin); ok {
		if err := TypeCheckArguments(builtin.types, arguments); err != nil {
			return nil, NewError(
				location,
				ctx.NewString(err.Error()),
			)
		}
		result, err := builtin.impl(ctx, arguments)
		if err != nil {
			if error, ok := err.(*Error); ok {
				error.Trace = append(error.Trace, TraceElement{location, builtin.String()})
			}
			return nil, err
		}

		return result, nil
	}

	return nil, NewError(
		location,
		ctx.NewString(fmt.Sprintf("attempted to call non-function type %s with value %s", quote(Typename(callable)), callable.String())),
	)
}

const (
	ANY       = "any"
	NULL      = "null"
	BOOLEAN   = "boolean"
	NUMBER    = "number"
	STRING    = "string"
	REGEXP    = "regexp"
	VECTOR    = "vector"
	MAP       = "map"
	SET       = "set"
	REFERENCE = "reference"
	FUNCTION  = "function"
)

type Type struct {
	Kind string
	Base *Type
}

func (self Type) String() string {
	if self.Kind == REFERENCE && self.Base != nil {
		return fmt.Sprintf("refrence to %s", self.Base.String())
	}
	return self.Kind
}

func TVal(kind string) Type {
	return Type{kind, nil}
}

func TRef(base Type) Type {
	return Type{REFERENCE, &base}
}

func TypeCheckArgument(index int, expected Type, received Value) error {
	switch expected.Kind {
	case ANY:
		return nil
	case NULL:
		if _, ok := received.(*Null); ok {
			return nil
		}
	case BOOLEAN:
		if _, ok := received.(*Boolean); ok {
			return nil
		}
	case NUMBER:
		if _, ok := received.(*Number); ok {
			return nil
		}
	case STRING:
		if _, ok := received.(*String); ok {
			return nil
		}
	case REGEXP:
		if _, ok := received.(*Regexp); ok {
			return nil
		}
	case VECTOR:
		if _, ok := received.(*Vector); ok {
			return nil
		}
	case MAP:
		if _, ok := received.(*Map); ok {
			return nil
		}
	case SET:
		if _, ok := received.(*Set); ok {
			return nil
		}
	case REFERENCE:
		reference, ok := received.(*Reference)
		if !ok {
			break
		}
		if expected.Base == nil {
			return nil // No required base type.
		}
		if err := TypeCheckArgument(index, *expected.Base, reference.data); err != nil {
			return fmt.Errorf("expected reference to %s-like value for argument %v, received reference to %s", expected.Base, index, reference.data.Typename())
		}
		return nil
	case FUNCTION:
		if _, ok := received.(*Reference); ok {
			return nil
		}
		if _, ok := received.(*Builtin); ok {
			return nil
		}
	}
	return fmt.Errorf("expected %s-like value for argument %v, received %s", expected, index, received.Typename())
}

func TypeCheckArguments(types []Type, arguments []Value) error {
	if len(types) != len(arguments) {
		return fmt.Errorf("invalid function argument count (expected %v, received %v)", len(types), len(arguments))
	}

	for i := range types {
		if err := TypeCheckArgument(i, types[i], arguments[i]); err != nil {
			return err
		}
	}

	return nil
}

func BuiltinDump(ctx *Context) *Builtin {
	return ctx.NewBuiltin("dump", []Type{TVal(ANY)}, func(ctx *Context, arguments []Value) (Value, error) {
		fmt.Printf("%v", arguments[0])
		return ctx.NewNull(), nil
	})
}

func BuiltinDumpln(ctx *Context) *Builtin {
	return ctx.NewBuiltin("dumpln", []Type{TVal(ANY)}, func(ctx *Context, arguments []Value) (Value, error) {
		fmt.Printf("%v\n", arguments[0])
		return ctx.NewNull(), nil
	})
}
