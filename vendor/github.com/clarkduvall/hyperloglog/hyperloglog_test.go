package hyperloglog

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"testing"
)

type fakeHash32 uint32

func (f fakeHash32) Sum32() uint32 { return uint32(f) }

func TestHLLAdd(t *testing.T) {
	h, _ := New(16)

	h.Add(fakeHash32(0x00010fff))
	n := h.reg[1]
	if n != 5 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x0002ffff))
	n = h.reg[2]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00030000))
	n = h.reg[3]
	if n != 17 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00030001))
	n = h.reg[3]
	if n != 17 {
		t.Error(n)
	}

	h.Add(fakeHash32(0xff037000))
	n = h.reg[0xff03]
	if n != 2 {
		t.Error(n)
	}

	h.Add(fakeHash32(0xff030800))
	n = h.reg[0xff03]
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLCount(t *testing.T) {
	h, _ := New(16)

	n := h.Count()
	if n != 0 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00010fff))
	h.Add(fakeHash32(0x00020fff))
	h.Add(fakeHash32(0x00030fff))
	h.Add(fakeHash32(0x00040fff))
	h.Add(fakeHash32(0x00050fff))
	h.Add(fakeHash32(0x00050fff))

	n = h.Count()
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLMergeError(t *testing.T) {
	h, _ := New(16)
	h2, _ := New(10)

	err := h.Merge(h2)
	if err == nil {
		t.Error("different precision should return error")
	}
}

func TestHLLMerge(t *testing.T) {
	h, _ := New(16)
	h.Add(fakeHash32(0x00010fff))
	h.Add(fakeHash32(0x00020fff))
	h.Add(fakeHash32(0x00030fff))
	h.Add(fakeHash32(0x00040fff))
	h.Add(fakeHash32(0x00050fff))
	h.Add(fakeHash32(0x00050fff))

	h2, _ := New(16)
	h2.Merge(h)
	n := h2.Count()
	if n != 5 {
		t.Error(n)
	}

	h2.Merge(h)
	n = h2.Count()
	if n != 5 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00060fff))
	h.Add(fakeHash32(0x00070fff))
	h.Add(fakeHash32(0x00080fff))
	h.Add(fakeHash32(0x00090fff))
	h.Add(fakeHash32(0x000a0fff))
	h.Add(fakeHash32(0x000a0fff))
	n = h.Count()
	if n != 10 {
		t.Error(n)
	}

	h2.Merge(h)
	n = h2.Count()
	if n != 10 {
		t.Error(n)
	}
}

func TestHLLClear(t *testing.T) {
	h, _ := New(16)
	h.Add(fakeHash32(0x00010fff))

	n := h.Count()
	if n != 1 {
		t.Error(n)
	}
	h.Clear()

	n = h.Count()
	if n != 0 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00010fff))
	n = h.Count()
	if n != 1 {
		t.Error(n)
	}
}

func TestHLLPrecision(t *testing.T) {
	h, _ := New(4)

	h.Add(fakeHash32(0x1fffffff))
	n := h.reg[1]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash32(0xffffffff))
	n = h.reg[0xf]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash32(0x00ffffff))
	n = h.reg[0]
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLError(t *testing.T) {
	_, err := New(3)
	if err == nil {
		t.Error("precision 3 should return error")
	}

	_, err = New(17)
	if err == nil {
		t.Error("precision 17 should return error")
	}
}

func TestHLLGob(t *testing.T) {
	var c1, c2 struct {
		HLL   *HyperLogLog
		Count int
	}
	c1.HLL, _ = New(8)
	for _, h := range []fakeHash32{0x10fff, 0x20fff, 0x30fff, 0x40fff, 0x50fff} {
		c1.HLL.Add(h)
		c1.Count++
	}

	var buf bytes.Buffer

	if err := gob.NewEncoder(&buf).Encode(&c1); err != nil {
		t.Error(err)
	}
	if err := gob.NewDecoder(&buf).Decode(&c2); err != nil {
		t.Error(err)
	}

	if !reflect.DeepEqual(c1, c2) {
		t.Error("unmarshaled structure differs")
	}
}
