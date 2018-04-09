package jody

import (
	"reflect"
	"unsafe"
)

const (
	shift    = 11
	constant = 0x1f3d5b79

	// Init64 is what 64 bits hash values should be initialized with.
	Init64 = uint64(0)
)

var mask64 = [...]uint64{
	0x0000000000000000,
	0x00000000000000ff,
	0x000000000000ffff,
	0x0000000000ffffff,
	0x00000000ffffffff,
	0x000000ffffffffff,
	0x0000ffffffffffff,
	0x00ffffffffffffff,
	0xffffffffffffffff,
}

// HashString64 returns the hash of s.
func HashString64(s string) uint64 {
	return AddString64(Init64, s)
}

// HashUint64 returns the hash of u.
func HashUint64(u uint64) uint64 {
	return AddUint64(Init64, u)
}

// AddString64 adds the hash of s to the precomputed hash value h.
func AddString64(h uint64, s string) uint64 {
	/*
		This is an implementation of the jody hashing algorithm as found here:

		- https://github.com/jbruchon/jodyhash

		It was revisited a bit and only the 64 bit version is available for now,
		but it's indeed really fast:

		- BenchmarkHash64/jody/algorithm-4   100000000   11.8 ns/op   3050.83 MB/s   0 B/op   0 allocs/op

		Here's what the reference implementation looks like:

		hash_t hash = start_hash;
		hash_t element;
		hash_t partial_salt;
		size_t len;

		len = count / sizeof(hash_t);
		for (; len > 0; len--) {
			element = *data;
			hash += element;
			hash += constant;
			hash = (hash << shift) | hash >> (sizeof(hash_t) * 8 - shift);
			hash ^= element;
			hash = (hash << shift) | hash >> (sizeof(hash_t) * 8 - shift);
			hash ^= constant;
			hash += element;
			data++;
		}

		len = count & (sizeof(hash_t) - 1);
		if (len) {
			partial_salt = constant & tail_mask[len];
			element = *data & tail_mask[len];
			hash += element;
			hash += partial_salt;
			hash = (hash << shift) | hash >> (sizeof(hash_t) * 8 - shift);
			hash ^= element;
			hash = (hash << shift) | hash >> (sizeof(hash_t) * 8 - shift);
			hash ^= partial_salt;
			hash += element;
		}

		return hash;
	*/

	r := *(*reflect.StringHeader)(unsafe.Pointer(&s))
	p := unsafe.Pointer((*reflect.StringHeader)(unsafe.Pointer(&s)).Data)

	for n := r.Len / 8; n != 0; n-- {
		v := *(*uint64)(p)

		h = h + v + constant
		h = (h<<shift | h>>(64-shift)) ^ v
		h = ((h<<shift | h>>(64-shift)) ^ constant) + v

		p = unsafe.Pointer(uintptr(p) + 8)
	}

	if n := (r.Len & 7); n != 0 {
		m := mask64[n]
		c := constant & m
		v := *(*uint64)(p) & m // risk of segfault here?

		h = h + v + c
		h = (h<<shift | h>>(64-shift)) ^ v
		h = ((h<<shift | h>>(64-shift)) ^ c) + v
	}

	return h
}

// AddUint64 adds the hash value of the 8 bytes of u to h.
func AddUint64(h uint64, u uint64) uint64 {
	h = h + u + constant
	h = (h<<shift | h>>(64-shift)) ^ u
	h = ((h<<shift | h>>(64-shift)) ^ constant) + u
	return h
}
