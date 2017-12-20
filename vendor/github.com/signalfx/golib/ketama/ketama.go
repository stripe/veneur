package ketama

// Package ketama implements consistent hashing compatible with Algorithm::ConsistentHash::Ketama
// This is a fork of https://github.com/dgryski/go-ketama/blob/master/ketama.go written in a
// more extendable way

/*
This implementation draws from the Daisuke Maki's Perl module, which itself is
based on the original libketama code.  That code was licensed under the GPLv2,
and thus so is this.
The major API change from libketama is that Algorithm::ConsistentHash::Ketama allows hashing
arbitrary strings, instead of just memcached server IP addresses.
*/
import (
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"sort"
)

// Bucket is a source we hash into.  The Label() is used as the hash key and Weight represents
// how much we weight this bucket against others
type Bucket interface {
	Label() string
	Weight() uint32
}

type continuumPoint struct {
	bucket Bucket
	point  uint32
}

// Continuum stores the ketama hashring and allows users to hash bytes into the ring
type Continuum struct {
	ring    points
	buckets []Bucket
}

type points []continuumPoint

func (c points) Less(i, j int) bool { return c[i].point < c[j].point }
func (c points) Len() int           { return len(c) }
func (c points) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }

// New creates a new Continuum that uses the passed Buckets as parts of the ring
func New(buckets []Bucket) *Continuum {
	var ret Continuum
	ret.Reset(buckets)
	return &ret
}

// Buckets returns the buckets last set in the continuum
func (c *Continuum) Buckets() []Bucket {
	return c.buckets
}

// Reset the Continuum to use the given buckets in the hashring
func (c *Continuum) Reset(buckets []Bucket) {
	numbuckets := len(buckets)

	ring := make(points, 0, numbuckets*160)

	if numbuckets == 0 {
		c.ring = ring
		return
	}

	totalweight := uint32(0)
	for _, b := range buckets {
		totalweight += b.Weight()
	}

	for i, b := range buckets {
		pct := float32(b.Weight()) / float32(totalweight)

		// this is the equivalent of C's promotion rules, but in Go, to maintain exact compatibility with the C library
		limit := int(float32(float64(pct) * 40.0 * float64(numbuckets)))

		for k := 0; k < limit; k++ {
			/* 40 hashes, 4 numbers per hash = 160 points per bucket */
			ss := fmt.Sprintf("%s-%d", b.Label(), k)
			digest := md5.Sum([]byte(ss))

			for h := 0; h < 4; h++ {
				point := continuumPoint{
					point:  binary.LittleEndian.Uint32(digest[h*4:]),
					bucket: buckets[i],
				}
				ring = append(ring, point)
			}
		}
	}

	sort.Sort(ring)

	c.ring = ring
	c.buckets = buckets
}

// Hash an array of bytes into a location in the ring
func (c *Continuum) Hash(thing []byte) Bucket {
	hash := md5.Sum(thing)

	h := binary.LittleEndian.Uint32(hash[0:4])
	return c.Bucket(h)
}

// Bucket returns the bucket at or after a location in the ring
func (c *Continuum) Bucket(ringLocation uint32) Bucket {
	if c == nil || len(c.ring) == 0 {
		return nil
	}
	i := uint(sort.Search(len(c.ring), func(i int) bool { return c.ring[i].point >= ringLocation }))
	if i >= uint(len(c.ring)) {
		i = 0
	}

	return c.ring[i].bucket
}
