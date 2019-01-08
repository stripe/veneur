package ssf

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type constructor func(name string, value float32, tags map[string]string, opts ...SampleOption) *SSFSample

func TestValidity(t *testing.T) {
	tests := map[string]constructor{"count": Count, "gauge": Gauge, "histogram": Histogram}
	for name, elt := range tests {
		test := elt
		t.Run(fmt.Sprintf("%s", name), func(t *testing.T) {
			t.Parallel()
			sample := test("foo", 1, map[string]string{"purpose": "testing"})
			assert.Equal(t, "foo", sample.Name)
			assert.Equal(t, float32(1), sample.Value)
			assert.Equal(t, map[string]string{"purpose": "testing"}, sample.Tags)
		})
	}
}

func TestTimingMS(t *testing.T) {
	tests := []struct {
		res  time.Duration
		name string
	}{
		{time.Nanosecond, "ns"},
		{time.Microsecond, "µs"},
		{time.Millisecond, "ms"},
		{time.Second, "s"},
		{time.Minute, "min"},
		{time.Hour, "h"},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			sample := Timing("foo", 20*test.res, test.res, nil)
			assert.Equal(t, float32(20), sample.Value)
			assert.Equal(t, test.name, sample.Unit)

		})
	}
}

var testTypes = []string{"count", "gauge", "histogram", "set"}

func testSample(t *testing.T, name string, args ...SampleOption) *SSFSample {
	tags := map[string]string{"purpose": "testing"}
	switch name {
	case "count":
		return Count("foo", 1, tags, args...)
	case "gauge":
		return Gauge("foo", 1, tags, args...)
	case "histogram":
		return Histogram("foo", 1, tags, args...)
	case "set":
		return Set("foo", "bar", tags, args...)
	}
	t.Fatalf("Unknown sample type %s", name)
	return nil // not reached
}

func TestOptions(t *testing.T) {
	then := time.Now().Add(-20 * time.Second)
	testOpts := []struct {
		name  string
		cons  SampleOption
		check func(*SSFSample)
	}{
		{
			"unit",
			Unit("frobnizzles"),
			func(s *SSFSample) {
				assert.Equal(t, "frobnizzles", s.Unit)
			},
		},
		{
			"ts",
			Timestamp(then),
			func(s *SSFSample) {
				assert.Equal(t, then.UnixNano(), s.Timestamp)
			},
		},
	}
	for _, name := range testTypes {
		test := name
		t.Run(fmt.Sprintf("%s", name), func(t *testing.T) {
			t.Parallel()
			for _, elt := range testOpts {
				opt := elt
				t.Run(opt.name, func(t *testing.T) {
					t.Parallel()
					sample := testSample(t, test, opt.cons)
					opt.check(sample)
				})
			}
		})
	}
}

func TestPrefix(t *testing.T) {
	NamePrefix = "testing.the.prefix."
	for _, elt := range testTypes {
		test := elt
		t.Run(fmt.Sprintf("%s", test), func(t *testing.T) {
			sample := testSample(t, elt)
			assert.Equal(t, "testing.the.prefix.foo", sample.Name)
		})
	}
}

func BenchmarkRandomlySample(b *testing.B) {
	// allocate these outside the loop, so we are measuring
	// only the performance of RandomlySample
	// This benchmark is serialized, so even if RandomlySample
	// mutates the data, that is fine
	samples := make([]*SSFSample, 1000000)
	for i := range samples {
		samples[i] = Count("testing.counter", float32(i), nil)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		samples = RandomlySample(0.2, samples...)
	}
}

func BenchmarkRandomlySampleParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Because this benchmark is parallelized and RandomlySample
			// mutates the samples, we need to allocate them fresh for each
			// run. This benchmark will overestimate the time taken in RandomlySample,
			// but only by a constant factor, and the overall difference between implementations
			// is what we care about.
			samples := make([]*SSFSample, 10)
			for i := range samples {
				samples[i] = Count("testing.counter", float32(i), nil)
			}
			samples = RandomlySample(0.2, samples...)
		}
	})
}
