package trace_test

import (
	"runtime/debug"
	"testing"

	"github.com/stripe/veneur/v14/trace"
)

func TestSlice(t *testing.T) {
	pool := &trace.SlicePool[int]{}

	buf := pool.Get(8)
	copy(buf, []int{1})
	if buf[0] != 1 {
		t.Fatal("expect copy result is ff, but not")
	}

	// Disable GC to test re-acquire the same data
	gc := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(gc) // Re-enable GC

	pool.Put(buf)

	newBuf := pool.Get(7)
	if &newBuf[0] != &buf[0] {
		t.Fatal("expect newBuf and buf to be the same array")
	}
	if newBuf[0] != 1 {
		t.Fatal("expect the newBuf is the buf, but not")
	}
}

/*
Summary: if you can stay on the stack, do so. If not, use the pool and avoid defer.

BenchmarkByteSlice/Run.baseline.array-10         	1000000000	         0.3296 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.baseline.array.passToMethod-10         	 2332494	       512.8 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.baseline.slice-10                      	1000000000	         0.3149 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.baseline.slice.passToMethod-10         	 7069371	       168.5 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.N-10                                   	100000000	        10.09 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.N.defer-10                             	 4540251	       714.8 ns/op	    5878 B/op	       2 allocs/op
BenchmarkByteSlice/Run.N.passToMethod-10                      	100000000	        10.87 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.N.defer.passToMethod-10                	  528427	      2249 ns/op	    8196 B/op	       2 allocs/op
BenchmarkByteSlice/Run.Parallel-10                            	815722753	         1.356 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.Parallel.defer-10                      	924507840	         1.594 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.Parallel.passToMethod-10               	834840195	         1.696 ns/op	       0 B/op	       0 allocs/op
BenchmarkByteSlice/Run.Parallel.defer.passToMethod-10         	747003232	         1.455 ns/op	       0 B/op	       0 allocs/op
*/
func BenchmarkByteSlice(b *testing.B) {
	pool := &trace.SlicePool[int]{}

	b.Run("Run.baseline.array", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var _ [1024]int
		}
	})
	b.Run("Run.baseline.array.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var a [1024]int
			passToSomeMethod1024(a)
		}
	})
	b.Run("Run.baseline.slice", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = make([]int, 0, 1024)
		}
	})
	b.Run("Run.baseline.slice.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			a := make([]int, 0, 1024)
			passToSomeMethod(a)
		}
	})
	b.Run("Run.N", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bs := pool.Get(1024)
			pool.Put(bs)
		}
	})
	b.Run("Run.N.defer", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bs := pool.Get(1024)
			defer pool.Put(bs)
		}
	})
	b.Run("Run.N.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bs := pool.Get(1024)
			passToSomeMethod(bs)
			pool.Put(bs)
		}
	})
	b.Run("Run.N.defer.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			bs := pool.Get(1024)
			defer pool.Put(bs)
			passToSomeMethod(bs)
		}
	})
	b.Run("Run.Parallel", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				bs := pool.Get(1024)
				pool.Put(bs)
			}
		})
	})
	b.Run("Run.Parallel.defer", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				bs := pool.Get(1024)
				pool.Put(bs)
			}
		})
	})
	b.Run("Run.Parallel.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				bs := pool.Get(1024)
				passToSomeMethod(bs)
				pool.Put(bs)
			}
		})
	})
	b.Run("Run.Parallel.defer.passToMethod", func(b *testing.B) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				bs := pool.Get(1024)
				passToSomeMethod(bs)
				pool.Put(bs)
			}
		})
	})
}

//go:noinline
func passToSomeMethod(arg []int) {
	// Do nothing, but don't inline it
}

//go:noinline
func passToSomeMethod1024(arg [1024]int) {
	// Do nothing, but don't inline it
}
