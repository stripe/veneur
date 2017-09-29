package explorable

import (
	"os"
	"reflect"
	"testing"

	"net/http"
	"net/http/httptest"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func getObj(obj interface{}) *Result {
	return ExploreObject(reflect.ValueOf(obj), []string{})
}

type Nameable interface {
	Name() string
}

type Person struct {
	name string
}

func (p *Person) Name() string {
	return p.name
}

type Classroom struct {
	subject  string
	teacher  Person
	students []Person
}

type ClassPtr struct {
	teacher  *Person
	students []interface{}
	subject  string
	remap    map[string]*Person
}

func TestInt(t *testing.T) {
	o := 1234
	assert.Equal(t, "1234", getObj(o).Desc)
}

func TestFloat(t *testing.T) {
	o := 1.5
	assert.Equal(t, "1.5000000000", getObj(o).Desc)
}

func TestUInt(t *testing.T) {
	o := uint(1234)
	assert.Equal(t, "1234", getObj(o).Desc)
}

func TestString(t *testing.T) {
	p := Person{
		name: "hello",
	}
	assert.Equal(t, "hello", getObj(p.name).Desc)
}

func TestStruct(t *testing.T) {
	p := Person{
		name: "hello",
	}
	assert.Equal(t, "Person", getObj(p).Desc)
	assert.Equal(t, "name", getObj(p).Children[0])

	verify(t, reflect.ValueOf(p), "hello", []string{"name"})
	verify(t, reflect.ValueOf(p), "<Invalid path NOTHERE>", []string{"NOTHERE"})
}

func TestPtr(t *testing.T) {
	p := &Person{
		name: "hello",
	}
	assert.Equal(t, "Person", getObj(p).Desc)
	assert.Equal(t, "name", getObj(p).Children[0])
	p = nil
	assert.Equal(t, "<NIL>", getObj(p).Desc)
}

func TestInterface(t *testing.T) {
	p := &ClassPtr{
		teacher: &Person{
			name: "hello",
		},
		students: make([]interface{}, 3),
	}
	p.students[0] = 123
	p.students[1] = &Person{
		name: "hello2",
	}
	var q Nameable
	q = &Person{}
	p.students[2] = q
	assert.Equal(t, "Person", getObj(p.teacher).Desc)
	assert.Equal(t, "name", getObj(p.teacher).Children[0])
	verify(t, reflect.ValueOf(p), "123", []string{"students", "0"})
	verify(t, reflect.ValueOf(p), "hello2", []string{"students", "1", "name"})

	assert.Equal(t, "slice-len(3 of 3)", getObj(p.students).Desc)
	assert.Equal(t, 3, len(getObj(p.students).Children))
	verify(t, reflect.ValueOf(p), "hello", []string{"teacher", "name"})

	var q2 *Nameable

	exploreInterface(reflect.ValueOf(q2), []string{})
}

func TestIntMapKeys(t *testing.T) {
	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[int]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[int]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[int8]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[int8]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[int16]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[int16]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[int32]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[int32]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[int64]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[int64]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[uint]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[uint]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[uint8]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[uint8]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[uint16]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[uint16]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[uint32]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[uint32]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[uint64]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[uint64]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[float32]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[float32]string{1: "a"}), []string{"Z"}).Desc)

	assert.Equal(t, "a", ExploreObject(reflect.ValueOf(map[float64]string{1: "a"}), []string{"1"}).Desc)
	assert.NotEmpty(t, "a", ExploreObject(reflect.ValueOf(map[float64]string{1: "a"}), []string{"Z"}).Desc)
}

func TestMap(t *testing.T) {
	p := &ClassPtr{}
	assert.Equal(t, "<NIL>", getObj(p.remap).Desc)
	p.remap = make(map[string]*Person)
	assert.Equal(t, "map-len(0)", getObj(p.remap).Desc)
	p.remap["hello"] = &Person{
		name: "John doe",
	}
	assert.Equal(t, "map-len(1)", getObj(p.remap).Desc)
	assert.Equal(t, "hello", getObj(p.remap).Children[0])
	verify(t, reflect.ValueOf(p), "John doe", []string{"remap", "hello", "name"})
}

func verify(t *testing.T, v reflect.Value, desc string, vals []string) {
	for i := 0; i < len(vals)-1; i++ {
		o := ExploreObject(v, vals[0:i])
		assert.Contains(t, o.Children, vals[i])
	}
	o := ExploreObject(v, vals)
	assert.Equal(t, desc, o.Desc)
}

func TestChan(t *testing.T) {
	var p chan struct{}
	assert.Equal(t, "<NIL>", getObj(p).Desc)

	p = make(chan struct{}, 1)
	assert.Equal(t, "chan-len(0 of 1)", getObj(p).Desc)
}

func TestMapInt(t *testing.T) {
	q := make(map[int64]string)
	q[1234] = "123"
	verify(t, reflect.ValueOf(q), "123", []string{"1234"})

	q2 := make(map[int64]*Person)
	q2[123] = &Person{
		name: "john",
	}
	verify(t, reflect.ValueOf(q2), "john", []string{"123", "name"})
}

func TestByte(t *testing.T) {
	q := byte(1)
	verify(t, reflect.ValueOf(q), "1", []string{""})
}

func TestBool(t *testing.T) {
	q := false
	verify(t, reflect.ValueOf(q), "false", []string{""})
}

func TestArray(t *testing.T) {
	var b [10]int
	b[0] = 65
	verify(t, reflect.ValueOf(b), "65", []string{"0"})
	verify(t, reflect.ValueOf(b), "array-len(10 of 10)", []string{})
	assert.Equal(t, 10, len(ExploreObject(reflect.ValueOf(b), []string{}).Children))
	assert.Equal(t, 0, len(ExploreObject(reflect.ValueOf(b), []string{"ABC"}).Children))
}

func TestInvalid(t *testing.T) {
	assert.Equal(t, "<INVALID>", checkConsts(reflect.Value{}).Desc)
}

func TestUnsupported(t *testing.T) {
	m := make([]int, 3)
	ptr := unsafe.Pointer(&m)
	assert.Equal(t, "<Unsupported>", ExploreObject(reflect.ValueOf(ptr), []string{}).Desc)
}

func TestFunc(t *testing.T) {
	f := os.Rename
	verify(t, reflect.ValueOf(f), "os.Rename", []string{})
	f = nil
	assert.Equal(t, "<NIL function>", exploreFunc(reflect.ValueOf(f), []string{}).Desc)

	assert.Equal(t, "<UNKNOWN FUNCTION>", exploreFunc(reflect.ValueOf([]string{}), []string{}).Desc)
}

func TestHandler(t *testing.T) {
	p := []Person{{
		name: "john",
	}}
	h := Handler{
		Val:      p,
		BasePath: "/test/",
	}
	rw := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test/0/", nil)
	h.ServeHTTP(rw, req)
	assert.Equal(t, http.StatusOK, rw.Code)
}

func TestKeyMapType(t *testing.T) {
	m := map[uint]string{0: "hi"}
	assert.Equal(t, 0, int(keyMapType(reflect.ValueOf(m).Type().Key().Kind(), "0").Uint()))
	assert.Equal(t, "<INVALID MAP KEY b>", exploreMap(reflect.ValueOf(m), []string{"b"}).Desc)
	assert.Equal(t, "<NOT FOUND MAP KEY 1>", exploreMap(reflect.ValueOf(m), []string{"1"}).Desc)
	assert.False(t, keyMapType(reflect.ValueOf(m).Type().Key().Kind(), "abc").IsValid())

	m2 := map[int]string{0: "hi"}
	assert.False(t, keyMapType(reflect.ValueOf(m2).Type().Key().Kind(), "abc").IsValid())

	m3 := map[float64]string{1.2: "hi"}
	assert.False(t, keyMapType(reflect.ValueOf(m3).Type().Key().Kind(), "abc").IsValid())
	assert.Equal(t, 0, int(keyMapType(reflect.ValueOf(m3).Type().Key().Kind(), "0").Float()))

	m4 := map[*int]struct{}{}
	assert.False(t, keyMapType(reflect.ValueOf(m4).Type().Key().Kind(), "abc").IsValid())

}

func TestSlice(t *testing.T) {
	p := []int{1, 2, 3}
	assert.Contains(t, exploreSlice(reflect.ValueOf(p), []string{"abc"}).Desc, "strconv.ParseInt")
	p = nil
	assert.Equal(t, "<NIL>", exploreSlice(reflect.ValueOf(p), []string{"abc"}).Desc)
}
