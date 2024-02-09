package fastrand

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand"

	"github.com/stripe/veneur/v14/internal/safepool"
)

func seed() int64 {
	var buf [8]byte
	n, err := crand.Read(buf[:])
	if n != 8 || err != nil {
		panic("invariant broken: crypto/rand.Read failed")
	}
	i := binary.LittleEndian.Uint64(buf[:])
	return int64(i)
}

// RandPool is a typesafe wrapper around sync.Pool for *rand.Rand instances
type RandPool struct {
	pool safepool.Pool[*rand.Rand]
}

// NewRandPool returns a typesafe wrapper around sync.Pool for *rand.Rand instances.
// The API of RandPool is safe for use by concurrent goroutines, but the returned objects
// are not.  Objects are seeded with real randomness from crypto/rand.
func NewRandPool() *RandPool {
	return &RandPool{
		pool: *safepool.NewPool(func() *rand.Rand {
			source := rand.NewSource(seed())
			// nolint:gosec G404: Use of weak random number generator (math/rand instead of crypto/rand) (gosec)
			return rand.New(source)
		}),
	}
}

// Get returns a *rand.Rand for random number generation.  This method is safe for use
// from concurrent goroutines, but the returned *rand.Rand must only be used from a single
// goroutine.
func (p *RandPool) Get() *rand.Rand {
	return p.pool.Get()
}

// Put returns a *rand.Rand to the pool.  This method is safe for use from concurrent goroutines.
func (p *RandPool) Put(r *rand.Rand) {
	p.pool.Put(r)
}

// Float64 returns, as a float64, a pseudo-random number in the half-open interval [0.0,1.0).
func (p *RandPool) Float64() float64 {
	r := p.Get()
	defer p.Put(r)
	return r.Float64()
}

// Int63 returns a non-negative pseudo-random 63-bit integer as an int64.
func (p *RandPool) Int63() int64 {
	r := p.Get()
	defer p.Put(r)
	return r.Int63()
}
