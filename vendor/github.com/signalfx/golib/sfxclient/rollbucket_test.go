package sfxclient

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/signalfx/golib/timekeeper/timekeepertest"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRollingBucketThreadRaces(t *testing.T) {
	r := NewRollingBucket("mname", nil)
	tk := timekeepertest.NewStubClock(time.Now())
	r.Timer = tk
	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				r.Add(.1)
			}
		}()
	}
	go func() {
		for q := 0; q < 1000; q++ {
			r.Datapoints()
		}
	}()
	wg.Wait()
}

func TestRollingBucket(t *testing.T) {
	Convey("With an empty rolling bucket", t, func() {
		r := NewRollingBucket("mname", nil)
		tk := timekeepertest.NewStubClock(time.Now())
		r.Timer = tk
		Convey("Get datapoints should work", func() {
			dps := r.Datapoints()
			So(len(dps), ShouldEqual, 3)
			So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "0")
		})
		Convey("Unadvanced clock should get only the normal points", func() {
			r.Add(1.0)
			dps := r.Datapoints()
			So(len(dps), ShouldEqual, 3)
			So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "1")
		})
		Convey("Advanced clock should get the one point", func() {
			r.Add(1.0)
			tk.Incr(r.BucketWidth)
			r.Add(2.0)
			dps := r.Datapoints()
			So(len(dps), ShouldEqual, 3+len(r.Quantiles)+2)
			So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "3")
			So(dpNamed("mname.p90", dps).Value.String(), ShouldEqual, "1")
		})
		Convey("and adding 100 elements", func() {
			for i := 1; i <= 100; i++ {
				r.Add(float64(i))
			}
			Convey("Percentiles should generally work", func() {
				tk.Incr(r.BucketWidth)
				dps := r.Datapoints()
				So(len(dps), ShouldEqual, 3+len(r.Quantiles)+2)
				So(dpNamed("mname.sum", dps).Value.String(), ShouldEqual, "5050")
				So(dpNamed("mname.p25", dps).Value.String(), ShouldEqual, "25.5")
				So(dpNamed("mname.p50", dps).Value.String(), ShouldEqual, "50")
				So(dpNamed("mname.p90", dps).Value.String(), ShouldEqual, "90")
				So(dpNamed("mname.p99", dps).Value.String(), ShouldEqual, "99")
				So(dpNamed("mname.min", dps).Value.String(), ShouldEqual, "1")
				So(dpNamed("mname.max", dps).Value.String(), ShouldEqual, "100")
			})
			Convey("Strange quantiles should work", func() {
				r.Quantiles = append(r.Quantiles, 0, 1, -1)
				tk.Incr(r.BucketWidth)
				dps := r.Datapoints()
				So(len(dps), ShouldEqual, 3+len(r.Quantiles)+2)
			})
			Convey("Max flush size should be respected", func() {
				r.Quantiles = append(r.Quantiles)
				r.MaxFlushBufferSize = 1
				tk.Incr(r.BucketWidth)
				dps := r.Datapoints()
				So(len(dps), ShouldEqual, 3+1)
			})
		})
	})
}

func BenchmarkRollingBucket(b *testing.B) {
	cb := NewRollingBucket("", nil)
	r := rand.New(rand.NewSource(0))
	t := time.Now()
	for i := 0; i < b.N; i++ {
		cb.AddAt(r.Float64(), t)
	}
}

func BenchmarkRollingBucket10(b *testing.B) {
	benchRB(b, 10)
}

func benchRB(b *testing.B, numGoroutine int) {
	cb := NewRollingBucket("", nil)
	w := sync.WaitGroup{}
	w.Add(numGoroutine)
	t := time.Now()
	for g := 0; g < numGoroutine; g++ {
		go func(g int) {
			r := rand.New(rand.NewSource(0))
			for i := g; i < b.N; i += numGoroutine {
				cb.AddAt(r.Float64(), t)
			}
			w.Done()
		}(g)
	}
	w.Wait()
}

func BenchmarkRollingBucket100(b *testing.B) {
	benchRB(b, 100)
}
