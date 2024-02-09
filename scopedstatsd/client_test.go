package scopedstatsd

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsure(t *testing.T) {
	var theNilOne Client = nil
	ensured := Ensure(theNilOne)
	assert.NotNil(t, ensured)
	assert.Error(t, errors.New("statsd client is nil"), ensured.Count("hi", 0, nil, 1.0))
}

func TestDoesSomething(t *testing.T) {
	type statsFunc func() error

	clients := []struct {
		name   string
		client Client
	}{
		{"nilClient", (*ScopedClient)(nil)},
		{"nilInner", NewClient(nil, []string{}, MetricScopes{})},
	}
	for _, elt := range clients {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			cl := test.client
			testFuncs := []statsFunc{
				func() error {
					return cl.Gauge("hi", 1, nil, 1.0)
				},
				func() error {
					return cl.Count("hi", 1, nil, 1.0)
				},
				func() error {
					return cl.Incr("hi", nil, 1.0)
				},
				func() error {
					return cl.Timing("hi", 1, nil, 1.0)
				},
				func() error {
					return cl.Histogram("hi", 1, nil, 1.0)
				},
				func() error {
					return cl.TimeInMilliseconds("hi", 1, nil, 1.0)
				},
			}
			for _, fn := range testFuncs {
				assert.Error(t, errors.New("statsd client is nil"), fn())
			}
		})
	}
}
