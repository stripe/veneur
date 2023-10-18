package safepool

import (
	"sync"
)

// Pool is a generic, safe wrapper around sync.Pool.
type Pool[T any] struct {
	p sync.Pool
}

// NewPool returns a safe wrapper around sync.Pool for a given type.
func NewPool[T any](newFn func() T) *Pool[T] {
	return &Pool[T]{
		p: sync.Pool{
			New: func() interface{} {
				return newFn()
			},
		},
	}
}

// Get returns an item of type T.
func (p *Pool[T]) Get() T {
	return p.p.Get().(T)
}

// Put returns an item of type T to the pool for reuse.
func (p *Pool[T]) Put(item T) {
	p.p.Put(item)
}
