package metrics

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func TestEmptyMetrics(t *testing.T) {
	err := Report(trace.DefaultClient, &ssf.Samples{})
	assert.Error(t, err)
	assert.IsType(t, NoMetrics{}, err)

	assert.Error(t, ReportBatch(trace.DefaultClient, []*ssf.SSFSample{}))
	assert.Error(t, err)
	assert.IsType(t, NoMetrics{}, err)

	assert.Error(t, ReportAsync(trace.DefaultClient, []*ssf.SSFSample{}, nil))
	assert.Error(t, err)
	assert.IsType(t, NoMetrics{}, err)
}

type testBackend struct {
	ch chan *ssf.SSFSpan
}

func (be *testBackend) Close() error {
	return nil
}

func (be *testBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	be.ch <- span
	return nil
}

func (be *testBackend) FlushSync(ctx context.Context) error {
	return nil
}

func newClient(t *testing.T) (*trace.Client, chan *ssf.SSFSpan) {
	ch := make(chan *ssf.SSFSpan, 1)
	cl, err := trace.NewBackendClient(&testBackend{ch})
	require.NoError(t, err)
	return cl, ch
}

func TestDeferring(t *testing.T) {
	client, ch := newClient(t)
	defer func() {
		span := <-ch
		assert.Equal(t, 3, len(span.Metrics))
	}()

	samples := &ssf.Samples{}
	defer Report(client, samples)

	samples.Add(ssf.Count("foo", 1, nil))
	samples.Add(ssf.Count("bar", 2, nil),
		ssf.Gauge("baz", 3, nil))
}
