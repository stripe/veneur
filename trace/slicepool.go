package trace

import (
	"math"
	"math/bits"
	"reflect"
	"runtime"
	"sync"
	"unsafe"
)

// SlicePool consists of 32 sync.Pool, representing slices of length from 0 to 32 in powers of 2.
type SlicePool[T any] struct {
	pools [32]sync.Pool
}

// Get retrieves a slice of the length requested by the caller from pool or allocates a new one.
func (p *SlicePool[T]) Get(size int) (buf []T) {
	if size <= 0 {
		return nil
	}
	if size > math.MaxInt32 {
		return make([]T, size)
	}
	idx := index(uint32(size))
	ptr, _ := p.pools[idx].Get().(unsafe.Pointer)
	if ptr == nil {
		return make([]T, 1<<idx)[:size]
	}
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	sh.Data = uintptr(ptr)
	sh.Len = size
	sh.Cap = 1 << idx
	runtime.KeepAlive(ptr)
	return
}

// Put returns the slice to the pool.
func (p *SlicePool[T]) Put(buf []T) {
	size := cap(buf)
	if size == 0 || size > math.MaxInt32 {
		return
	}
	idx := index(uint32(size))
	if size != 1<<idx { // this slice is not from Pool.Get(), put it into the previous interval of idx
		idx--
	}
	// array pointer
	p.pools[idx].Put(unsafe.Pointer(&buf[:1][0]))
}

func index(n uint32) uint32 {
	return uint32(bits.Len32(n - 1))
}
