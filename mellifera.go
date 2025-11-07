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
	Equal(Value) bool
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

func (self *Null) Equal(other Value) bool {
	_, ok := other.(*Null)
	return ok
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

func (self *Boolean) Equal(other Value) bool {
	othr, ok := other.(*Boolean)
	if !ok {
		return false
	}
	return self.data == othr.data
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

func (self *Number) Equal(other Value) bool {
	othr, ok := other.(*Number)
	if !ok {
		return false
	}
	return self.data == othr.data
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

func (self *String) Equal(other Value) bool {
	othr, ok := other.(*String)
	if !ok {
		return false
	}
	return self.data == othr.data
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

func (self *Regexp) Equal(other Value) bool {
	othr, ok := other.(*Regexp)
	if !ok {
		return false
	}
	return self.data.String() == othr.data.String()
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

// Doubly-linked list of key-value pairs in insertion order. This MapData
// implementation is purposefully designed to be stupid simple with the intent
// to replace the implementation with something more performant later.
type MapData struct {
	head  *MapElement
	tail  *MapElement
	count int
	uses  int
}

func (self *MapData) Lookup(key Value) *MapElement {
	cur := self.head
	for cur != nil {
		if cur.key.Equal(key) {
			return cur
		}
		cur = cur.next
	}
	return nil
}

func (self *MapData) Insert(key, value Value) {
	if self.head == nil {
		element := &MapElement{
			key:   key,
			value: value,
		}
		self.head = element
		self.tail = element
		self.count = 1
		return
	}

	lookup := self.Lookup(key)
	if lookup == nil {
		element := &MapElement{
			prev:  self.tail,
			key:   key,
			value: value,
		}
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

	lookup := self.Lookup(key)
	if lookup == nil {
		return
	}

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

func (self *Map) Equal(other Value) bool {
	othr, ok := other.(*Map)
	if !ok {
		return false
	}
	if self.Count() != othr.Count() {
		return false
	}

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

func (self *Map) Count() int {
	if self.data == nil {
		return 0
	}

	return self.data.count
}

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
	prev  *SetElement
	next  *SetElement
	value Value
}

// Doubly-linked list of elements in insertion order. This SetData
// implementation is purposefully designed to be stupid simple with the intent
// to replace the implementation with something more performant later.
type SetData struct {
	head  *SetElement
	tail  *SetElement
	count int
	uses  int
}

func (self *SetData) Lookup(value Value) *SetElement {
	cur := self.head
	for cur != nil {
		if cur.value.Equal(value) {
			return cur
		}
		cur = cur.next
	}
	return nil
}

func (self *SetData) Insert(value Value) {
	if self.head == nil {
		element := &SetElement{
			value: value,
		}
		self.head = element
		self.tail = element
		self.count = 1
		return
	}

	lookup := self.Lookup(value)
	if lookup == nil {
		element := &SetElement{
			prev:  self.tail,
			value: value,
		}
		self.tail.next = element
		self.tail = element
		self.count += 1
		return
	}

	lookup.value = value
}

func (self *SetData) Remove(value Value) {
	if self.head == nil {
		return
	}

	lookup := self.Lookup(value)
	if lookup == nil {
		return
	}

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
		s = append(s, cur.value.String())
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
			data.Insert(cur.value.Copy())
			cur = cur.next
		}
		self.data = data
	}
}

func (self *Set) Equal(other Value) bool {
	othr, ok := other.(*Set)
	if !ok {
		return false
	}
	if self.Count() != othr.Count() {
		return false
	}

	selfCur := self.data.head
	othrCur := othr.data.head
	for selfCur != nil {
		if !selfCur.value.Equal(othrCur.value) {
			return false
		}
		selfCur = selfCur.next
		othrCur = othrCur.next
	}

	return true
}

func (self *Set) Count() int {
	if self.data == nil {
		return 0
	}

	return self.data.count
}

func (self *Set) Lookup(value Value) Value {
	if self.data == nil {
		return nil
	}

	element := self.data.Lookup(value)
	if element == nil {
		return nil
	}

	return element.value
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
