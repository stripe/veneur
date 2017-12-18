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

func TestOptions(t *testing.T) {
	then := time.Now().Add(-20 * time.Second)
	testFuns := map[string]constructor{"count": Count, "gauge": Gauge, "histogram": Histogram}
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
	for name, elt := range testFuns {
		test := elt
		t.Run(fmt.Sprintf("%s", name), func(t *testing.T) {
			t.Parallel()
			for _, elt := range testOpts {
				opt := elt
				t.Run(opt.name, func(t *testing.T) {
					t.Parallel()
					sample := test("foo", 1, map[string]string{"purpose": "testing"}, opt.cons)
					opt.check(sample)
				})
			}
		})
	}
}
