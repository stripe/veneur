package gohistogram

import (
	"testing"
	"math/rand"
)

func BenchmarkNumericHistogram20(b *testing.B) {
	runBenchmark(20, b.N)
//	b.Logf("median: %f", h.Quantile(.50) )
}

func BenchmarkNumericHistogram50(b *testing.B) {
	runBenchmark(50, b.N)
//	b.Logf("median: %f", h.Quantile(.50) )
}

func BenchmarkNumericHistogram100(b *testing.B) {
	runBenchmark(100, b.N)
//	b.Logf("median: %f", h.Quantile(.50) )
}

func runBenchmark(maxSize int, iterSize int) *NumericHistogram {
	h := NewHistogram(maxSize)
	r := rand.New(rand.NewSource(0))
	for i := 0; i < iterSize; i++ {
		h.Add(r.Float64())
	}
	return h
}