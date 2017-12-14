package ssf

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type constructor func(name string, value float32, tags map[string]string) *SSFSample

func TestValidity(t *testing.T) {
	tests := []constructor{Count, Gauge, Histogram}
	for _, elt := range tests {
		test := elt
		t.Run(fmt.Sprintf("%v", test), func(t *testing.T) {
			t.Parallel()
			sample := test("foo", 1, map[string]string{"purpose": "testing"})
			assert.Equal(t, "foo", sample.Name)
			assert.Equal(t, float32(1), sample.Value)
			assert.Equal(t, map[string]string{"purpose": "testing"}, sample.Tags)
		})
	}
}
