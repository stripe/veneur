package ssf

import (
	"fmt"
	"testing"

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
		t.Run(fmt.Sprintf("%s/unit", name), func(t *testing.T) {
			t.Parallel()
			sample := test("foo", 1, map[string]string{"purpose": "testing"}, Unit("farts"))
			assert.Equal(t, "foo", sample.Name)
			assert.Equal(t, float32(1), sample.Value)
			assert.Equal(t, map[string]string{"purpose": "testing"}, sample.Tags)
			assert.Equal(t, "farts", sample.Unit)
		})
	}
}
