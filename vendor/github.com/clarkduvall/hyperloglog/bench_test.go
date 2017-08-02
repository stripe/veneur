package hyperloglog

import (
	"fmt"
	"hash"
	"hash/fnv"
	"math"
	"math/rand"
	"testing"
)

func hash32(s string) hash.Hash32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h
}

func hash64(s string) hash.Hash64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h
}

func randStr(n int) string {
	i := rand.Uint32()
	return fmt.Sprintf("a%s %s", i, n)
}

func benchmark(precision uint8, n int) {
	h, _ := New(precision)
	hpp, _ := NewPlus(precision)

	for i := 0; i < n; i++ {
		s := randStr(i)
		h.Add(hash32(s))
		h.Add(hash32(s))
		hpp.Add(hash64(s))
		hpp.Add(hash64(s))
	}

	e, epp := h.Count(), hpp.Count()

	var percentErr = func(est uint64) float64 {
		return math.Abs(float64(n)-float64(est)) / float64(n)
	}

	fmt.Printf("\nReal Cardinality: %8d\n", n)
	fmt.Printf("HyperLogLog     : %8d,   Error: %f%%\n", e, percentErr(e))
	fmt.Printf("HyperLogLog++   : %8d,   Error: %f%%\n", epp, percentErr(epp))
}

func BenchmarkHll4(b *testing.B) {
	benchmark(4, b.N)
}

func BenchmarkHll6(b *testing.B) {
	benchmark(6, b.N)
}

func BenchmarkHll8(b *testing.B) {
	benchmark(8, b.N)
}

func BenchmarkHll10(b *testing.B) {
	benchmark(10, b.N)
}

func BenchmarkHll14(b *testing.B) {
	benchmark(14, b.N)
}

func BenchmarkHll16(b *testing.B) {
	benchmark(16, b.N)
}
