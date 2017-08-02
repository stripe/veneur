package hyperloglog

import (
	"bytes"
	"encoding/gob"
	"math"
	"reflect"
	"testing"
)

type fakeHash64 uint64

func (f fakeHash64) Sum64() uint64 { return uint64(f) }

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestHLLPPAddNoSparse(t *testing.T) {
	h, _ := NewPlus(16)
	h.toNormal()

	h.Add(fakeHash64(0x00010fffffffffff))
	n := h.reg[1]
	if n != 5 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x0002ffffffffffff))
	n = h.reg[2]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x0003000000000000))
	n = h.reg[3]
	if n != 49 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x0003000000000001))
	n = h.reg[3]
	if n != 49 {
		t.Error(n)
	}

	h.Add(fakeHash64(0xff03700000000000))
	n = h.reg[0xff03]
	if n != 2 {
		t.Error(n)
	}

	h.Add(fakeHash64(0xff03080000000000))
	n = h.reg[0xff03]
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLPPPrecisionNoSparse(t *testing.T) {
	h, _ := NewPlus(4)
	h.toNormal()

	h.Add(fakeHash64(0x1fffffffffffffff))
	n := h.reg[1]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash64(0xffffffffffffffff))
	n = h.reg[0xf]
	if n != 1 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x00ffffffffffffff))
	n = h.reg[0]
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLPPToNormal(t *testing.T) {
	h, _ := NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))
	h.toNormal()
	c := h.Count()
	if c != 1 {
		t.Error(c)
	}

	if h.sparse {
		t.Error("toNormal should convert to normal")
	}

	h, _ = NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x0002ffffffffffff))
	h.Add(fakeHash64(0x0003000000000000))
	h.Add(fakeHash64(0x0003000000000001))
	h.Add(fakeHash64(0xff03700000000000))
	h.Add(fakeHash64(0xff03080000000000))
	h.mergeSparse()
	h.toNormal()

	n := h.reg[1]
	if n != 5 {
		t.Error(n)
	}
	n = h.reg[2]
	if n != 1 {
		t.Error(n)
	}
	n = h.reg[3]
	if n != 49 {
		t.Error(n)
	}
	n = h.reg[0xff03]
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLPPEstimateBias(t *testing.T) {
	h, _ := NewPlus(4)
	b := h.estimateBias(14.0988)
	if math.Abs(b-7.5988) > 0.00001 {
		t.Error(b)
	}

	// 10 is less than the first entry in the estimate table for p=4.
	biasTable := biasData[0]
	b = h.estimateBias(10)
	if math.Abs(b-biasTable[0]) > 0.00001 {
		t.Error(b)
	}

	// 80 is greater than the first entry in the estimate table for p-4.
	b = h.estimateBias(80)
	if math.Abs(b-biasTable[len(biasTable)-1]) > 0.00001 {
		t.Error(b)
	}

	h, _ = NewPlus(16)
	b = h.estimateBias(55391.4373)
	if math.Abs(b-39416.9373) > 0.00001 {
		t.Error(b)
	}
}

func TestHLLPPCount(t *testing.T) {
	h, _ := NewPlus(16)

	n := h.Count()
	if n != 0 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x00020fffffffffff))
	h.Add(fakeHash64(0x00030fffffffffff))
	h.Add(fakeHash64(0x00040fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))

	n = h.Count()
	if n != 5 {
		t.Error(n)
	}
}

func TestHLLPPMergeError(t *testing.T) {
	h, _ := NewPlus(16)
	h2, _ := NewPlus(10)

	err := h.Merge(h2)
	if err == nil {
		t.Error("different precision should return error")
	}
}

func TestHLLMergeSparse(t *testing.T) {
	h, _ := NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x00020fffffffffff))
	h.Add(fakeHash64(0x00030fffffffffff))
	h.Add(fakeHash64(0x00040fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))

	h2, _ := NewPlus(16)
	h2.Merge(h)
	n := h2.Count()
	if n != 5 {
		t.Error(n)
	}

	if !h2.sparse {
		t.Error("Merge should not convert to normal")
	}

	if !h.sparse {
		t.Error("Merge should not modify argument")
	}

	h2.Merge(h)
	n = h2.Count()
	if n != 5 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x00060fffffffffff))
	h.Add(fakeHash64(0x00070fffffffffff))
	h.Add(fakeHash64(0x00080fffffffffff))
	h.Add(fakeHash64(0x00090fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
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

func TestHLLMergeNormal(t *testing.T) {
	h, _ := NewPlus(16)
	h.toNormal()
	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x00020fffffffffff))
	h.Add(fakeHash64(0x00030fffffffffff))
	h.Add(fakeHash64(0x00040fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))

	h2, _ := NewPlus(16)
	h2.toNormal()
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

	h.Add(fakeHash64(0x00060fffffffffff))
	h.Add(fakeHash64(0x00070fffffffffff))
	h.Add(fakeHash64(0x00080fffffffffff))
	h.Add(fakeHash64(0x00090fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
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

func TestHLLMergeMixed(t *testing.T) {
	h, _ := NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x00020fffffffffff))
	h.Add(fakeHash64(0x00030fffffffffff))
	h.Add(fakeHash64(0x00040fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))

	h2, _ := NewPlus(16)
	h2.toNormal()
	h2.Merge(h)
	n := h2.Count()
	if n != 5 {
		t.Error(n)
	}

	if !h.sparse {
		t.Error("Merge should not modify argument")
	}

	h2.Merge(h)
	n = h2.Count()
	if n != 5 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x00060fffffffffff))
	h.Add(fakeHash64(0x00070fffffffffff))
	h.Add(fakeHash64(0x00080fffffffffff))
	h.Add(fakeHash64(0x00090fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
	h.Add(fakeHash64(0x000a0fffffffffff))
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

func TestHLLMergeMixedConvertToNormal(t *testing.T) {
	h, _ := NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))
	h.Add(fakeHash64(0x00020fffffffffff))
	h.Add(fakeHash64(0x00030fffffffffff))
	h.Add(fakeHash64(0x00040fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	h.Add(fakeHash64(0x00050fffffffffff))
	// h is normal, h2 should be converted too.
	h.toNormal()

	h2, _ := NewPlus(16)
	h2.Merge(h)
	n := h2.Count()
	if n != 5 {
		t.Error(n)
	}

	if h2.sparse {
		t.Error("Merge should convert to normal")
	}
}

func TestHLLPPClear(t *testing.T) {
	h, _ := NewPlus(16)
	h.Add(fakeHash64(0x00010fffffffffff))

	n := h.Count()
	if n != 1 {
		t.Error(n)
	}
	h.Clear()

	n = h.Count()
	if n != 0 {
		t.Error(n)
	}

	h.Add(fakeHash64(0x00010fffffffffff))
	n = h.Count()
	if n != 1 {
		t.Error(n)
	}
}

func TestHLLPPMerge(t *testing.T) {
	h, _ := NewPlus(16)

	k1 := uint64(0xf000017000000000)
	h.Add(fakeHash64(k1))
	if !h.tmpSet[h.encodeHash(k1)] {
		t.Error("key not in hash")
	}

	k2 := uint64(0x000fff8f00000000)
	h.Add(fakeHash64(k2))
	if !h.tmpSet[h.encodeHash(k2)] {
		t.Error("key not in hash")
	}

	if len(h.tmpSet) != 2 {
		t.Error(h.tmpSet)
	}

	h.mergeSparse()
	if len(h.tmpSet) != 0 {
		t.Error(h.tmpSet)
	}
	if h.sparseList.Count != 2 {
		t.Error(h.sparseList)
	}

	iter := h.sparseList.Iter()
	n := iter.Next()
	if n != h.encodeHash(k2) {
		t.Error(n)
	}
	n = iter.Next()
	if n != h.encodeHash(k1) {
		t.Error(n)
	}

	k3 := uint64(0x0f00017000000000)
	h.Add(fakeHash64(k3))
	if !h.tmpSet[h.encodeHash(k3)] {
		t.Error("key not in hash")
	}

	h.mergeSparse()
	if len(h.tmpSet) != 0 {
		t.Error(h.tmpSet)
	}
	if h.sparseList.Count != 3 {
		t.Error(h.sparseList)
	}

	iter = h.sparseList.Iter()
	n = iter.Next()
	if n != h.encodeHash(k2) {
		t.Error(n)
	}
	n = iter.Next()
	if n != h.encodeHash(k3) {
		t.Error(n)
	}
	n = iter.Next()
	if n != h.encodeHash(k1) {
		t.Error(n)
	}

	h.Add(fakeHash64(k1))
	if !h.tmpSet[h.encodeHash(k1)] {
		t.Error("key not in hash")
	}

	h.mergeSparse()
	if len(h.tmpSet) != 0 {
		t.Error(h.tmpSet)
	}
	if h.sparseList.Count != 3 {
		t.Error(h.sparseList)
	}

	iter = h.sparseList.Iter()
	n = iter.Next()
	if n != h.encodeHash(k2) {
		t.Error(n)
	}
	n = iter.Next()
	if n != h.encodeHash(k3) {
		t.Error(n)
	}
	n = iter.Next()
	if n != h.encodeHash(k1) {
		t.Error(n)
	}
}

func TestHLLPPEncodeDecode(t *testing.T) {
	h, _ := NewPlus(8)
	i, r := h.decodeHash(h.encodeHash(0xffffff8000000000))
	if i != 0xff {
		t.Error(i)
	}
	if r != 1 {
		t.Error(r)
	}

	i, r = h.decodeHash(h.encodeHash(0xff00000000000000))
	if i != 0xff {
		t.Error(i)
	}
	if r != 57 {
		t.Error(r)
	}

	i, r = h.decodeHash(h.encodeHash(0xff30000000000000))
	if i != 0xff {
		t.Error(i)
	}
	if r != 3 {
		t.Error(r)
	}

	i, r = h.decodeHash(h.encodeHash(0xaa10000000000000))
	if i != 0xaa {
		t.Error(i)
	}
	if r != 4 {
		t.Error(r)
	}

	i, r = h.decodeHash(h.encodeHash(0xaa0f000000000000))
	if i != 0xaa {
		t.Error(i)
	}
	if r != 5 {
		t.Error(r)
	}
}

func TestHLLPPError(t *testing.T) {
	_, err := NewPlus(3)
	if err == nil {
		t.Error("precision 3 should return error")
	}

	_, err = NewPlus(18)
	if err != nil {
		t.Error(err)
	}

	_, err = NewPlus(19)
	if err == nil {
		t.Error("precision 17 should return error")
	}
}

func TestHLLPPGob(t *testing.T) {
	var c1, c2 struct {
		HLL   *HyperLogLogPlus
		Count int
	}
	c1.HLL, _ = NewPlus(8)
	for _, h := range []fakeHash64{0x10fff, 0x20fff, 0x30fff, 0x40fff, 0x50fff} {
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

	if c1.HLL.Count() != c2.HLL.Count() {
		t.Error("HLL count differs")
	}
	if !reflect.DeepEqual(c1, c2) {
		t.Error("unmarshaled structure differs")
	}
}

func TestHLLPPEstimateBiasCount(t *testing.T) {
	h, _ := NewPlus(4)
	h.toNormal()

	// Adding ten entries in different buckets for p=4 skips linear counting.
	for i := 0; i < 10; i++ {
		h.Add(fakeHash64(i<<60 + 0xfffffffffffffff))
	}
	c := h.Count()
	// Count should be at most 1 off.
	if abs(10-int64(c)) > 1 {
		t.Error(c)
	}
}

func TestHLLPPToNormalWhenSparseIsTooBig(t *testing.T) {
	h, _ := NewPlus(4)

	for i := 0; i < 16; i++ {
		h.Add(fakeHash64(1 << uint(i)))
	}

	if !h.sparse {
		t.Error("h should still be sparse")
	}

	h.Add(fakeHash64(1 << 16))
	if h.sparse {
		t.Error("h should be converted to normal")
	}
}
