package hyperloglog

type iterable interface {
	decode(i int, last uint32) (uint32, int)
	Len() int
	Iter() *iterator
}

type iterator struct {
	i    int
	last uint32
	v    iterable
}

func (iter *iterator) Next() uint32 {
	n, i := iter.v.decode(iter.i, iter.last)
	iter.last = n
	iter.i = i
	return n
}

func (iter *iterator) Peek() uint32 {
	n, _ := iter.v.decode(iter.i, iter.last)
	return n
}

func (iter iterator) HasNext() bool {
	return iter.i < iter.v.Len()
}

type compressedList struct {
	Count uint32
	b     variableLengthList
	last  uint32
}

func newCompressedList(size int) *compressedList {
	v := &compressedList{}
	v.b = make(variableLengthList, 0, size)
	return v
}

func (v *compressedList) Len() int {
	return len(v.b)
}

func (v *compressedList) decode(i int, last uint32) (uint32, int) {
	n, i := v.b.decode(i, last)
	return n + last, i
}

func (v *compressedList) Append(x uint32) {
	v.Count++
	v.b = v.b.Append(x - v.last)
	v.last = x
}

func (v *compressedList) Iter() *iterator {
	return &iterator{0, 0, v}
}

type variableLengthList []uint8

func (v variableLengthList) Len() int {
	return len(v)
}

func (v *variableLengthList) Iter() *iterator {
	return &iterator{0, 0, v}
}

func (v variableLengthList) decode(i int, last uint32) (uint32, int) {
	var x uint32
	j := i
	for ; v[j]&0x80 != 0; j++ {
		x |= uint32(v[j]&0x7f) << (uint(j-i) * 7)
	}
	x |= uint32(v[j]) << (uint(j-i) * 7)
	return x, j + 1
}

func (v variableLengthList) Append(x uint32) variableLengthList {
	for x&0xffffff80 != 0 {
		v = append(v, uint8((x&0x7f)|0x80))
		x >>= 7
	}
	return append(v, uint8(x&0x7f))
}
