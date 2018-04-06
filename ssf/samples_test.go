package ssf

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type constructor func(name string, value float32, tags map[string]string) SSFSample

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

func testSample(t *testing.T, name string) SSFSample {
	tags := map[string]string{"purpose": "testing"}
	switch name {
	case "count":
		return Count("foo", 1, tags)
	case "gauge":
		return Gauge("foo", 1, tags)
	case "histogram":
		return Histogram("foo", 1, tags)
	case "set":
		return Set("foo", "bar", tags)
	}
	t.Fatalf("Unknown sample type %s", name)
	return SSFSample{} // not reached
}

func TestPrefix(t *testing.T) {
	NamePrefix = "testing.the.prefix."
	for _, elt := range testTypes {
		test := elt
		t.Run(fmt.Sprintf("%s", test), func(t *testing.T) {
			sample := testSample(t, elt)
			assert.Equal(t, "testing.the.prefix.foo", sample.Name, "prefix does not match")
		})
	}
}

func BenchmarkRandomlySample(b *testing.B) {
	samples := make([]SSFSample, 1000000)
	for i := range samples {
		cnt := Count("testing.counter", float32(i), nil)
		samples[i] = cnt
	}
	b.ResetTimer()
	samples = RandomlySample(0.2, samples...)
}
