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
	Copy() Value
	CopyOnWrite()
	Hash() uint64
	Equal(Value) bool
	CombEncode(e *CombEncoder) error
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
	// Null Singleton.
	Null *Null
	// Boolean Singletons
	True  *Boolean
	False *Boolean
	// Base Environment
	BaseEnvironment Environment
	// Miscellaneous State and Definitions
	reNumberDec *regexp.Regexp
	reNumberHex *regexp.Regexp
}

func NewContext() Context {
	ctx := Context{}
	ctx.Null = &Null{}
	ctx.True = &Boolean{true}
	ctx.False = &Boolean{false}
	ctx.BaseEnvironment = NewEnvironment(nil)
	ctx.reNumberDec = regexp.MustCompile(`^\d+(\.\d+)?`)
	ctx.reNumberHex = regexp.MustCompile(`^0x[0-9a-fA-F]+`)
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
	if elements == nil || len(elements) == 0 {
		return &Vector{}
	}

	return &Vector{
		data: &VectorData{
			elements: elements,
			uses:     1,
		},
	}
}

func (ctx *Context) NewMap(elements []MapPair) *Map {
	if elements == nil || len(elements) == 0 {
		return &Map{}
	}

	result := &Map{}
	for _, element := range elements {
		result.Insert(element.key, element.value)
	}
	return result
}

func (ctx *Context) NewSet(elements []Value) *Set {
	if elements == nil || len(elements) == 0 {
		return &Set{}
	}

	result := &Set{}
	for _, element := range elements {
		result.Insert(element)
	}
	return result
}

func (ctx *Context) NewReference(value Value) *Reference {
	return &Reference{value}
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

func (self *Map) Insert(key, value Value) {
	self.CopyOnWrite()
	if self.data == nil {
		self.data = &MapData{
			uses: 1,
		}
	}

	self.data.Insert(key, value)
}

func (self *Map) Remove(key Value) {
	if self.data == nil {
		return
	}

	self.CopyOnWrite()
	self.data.Remove(key)
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
}

func (self *Reference) Typename() string {
	return "reference"
}

func (self *Reference) String() string {
	return fmt.Sprintf("reference@%p", self.data)
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
	TOKEN_NUMBER     = "number"
	// Delimiters
	TOKEN_SEMICOLON = ";"
	// Keywords
	TOKEN_NULL  = "null"
	TOKEN_TRUE  = "true"
	TOKEN_FALSE = "false"
)

type Token struct {
	Kind     string
	Literal  string
	Location *SourceLocation // Optional
	Value    Value           // Optional
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
	location *SourceLocation // optional
	position int

	keywords map[string]string
}

func NewLexer(ctx *Context, source string, location *SourceLocation) Lexer {
	keywords := map[string]string{
		TOKEN_NULL:  TOKEN_NULL,
		TOKEN_TRUE:  TOKEN_TRUE,
		TOKEN_FALSE: TOKEN_FALSE,
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

func (self *Lexer) isEof() bool {
	return self.position >= len(self.source)
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

func (self *Lexer) NextToken() (Token, error) {
	self.skipWhiteSpaceAndComments()
	if self.isEof() {
		return Token{
			Kind:     TOKEN_EOF,
			Literal:  "",
			Location: self.currentLocation(),
		}, nil
	}

	// Literals, Identifiers, and Keywords
	if unicode.IsLetter(self.currentRune()) || self.currentRune() == '_' {
		return self.lexKeywordOrIdentifier()
	}
	if '0' <= self.currentRune() && self.currentRune() <= '9' {
		return self.lexNumber()
	}

	// Delimiters
	if self.currentRune() == ';' {
		self.advanceRune()
		return Token{
			Kind:     TOKEN_SEMICOLON,
			Literal:  TOKEN_SEMICOLON,
			Location: self.currentLocation(),
		}, nil
	}

	return Token{}, ParseError{
		Location: self.currentLocation(),
		why:      fmt.Sprintf("unknown token %s", quote(string([]rune{self.currentRune()}))),
	}
}

type TraceElement struct {
	Location *SourceLocation // Optional
	// TODO: Add Function/Builtin field.
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

type Parser struct {
	lexer        *Lexer
	currentToken Token

	parseNudFunctions map[string]func(*Parser) (AstExpression, error)
}

func NewParser(lexer *Lexer) Parser {
	self := Parser{
		lexer:        lexer,
		currentToken: Token{"invalid program", "", lexer.location, nil},

		parseNudFunctions: map[string]func(*Parser) (AstExpression, error){
			TOKEN_NULL:   (*Parser).ParseExpressionNull,
			TOKEN_TRUE:   (*Parser).ParseExpressionBoolean,
			TOKEN_FALSE:  (*Parser).ParseExpressionBoolean,
			TOKEN_NUMBER: (*Parser).ParseExpressionNumber,
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

func (self *Parser) ParseExpression() (AstExpression, error) {
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
	return expression, nil
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
		return AstExpressionNumber{}, errors.New("missing number token value")
	}
	return AstExpressionNumber{token.Location, value}, nil
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
