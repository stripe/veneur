package channel

import (
	"context"
	"time"

	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type ChannelMetricSink struct {
	metricsChannel chan []samplers.InterMetric
}

// NewChannelMetricSink creates a new ChannelMetricSink. This sink writes any
// flushed metrics to its `metricsChannel` such that the test can inspect
// the metrics for correctness.
func NewChannelMetricSink(ch chan []samplers.InterMetric) (ChannelMetricSink, error) {
	return ChannelMetricSink{
		metricsChannel: ch,
	}, nil
}

func (c ChannelMetricSink) Name() string {
	return "channel"
}

func (c ChannelMetricSink) Start(*trace.Client) error {
	return nil
}

func (c ChannelMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	// Put the whole slice in since many tests want to see all of them and we
	// don't want them to have to loop over and wait on empty or something
	c.metricsChannel <- metrics
	return nil
}

func (c ChannelMetricSink) FlushOtherSamples(ctx context.Context, events []ssf.SSFSample) {
	return
}
