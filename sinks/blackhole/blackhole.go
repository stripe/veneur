package blackhole

import (
	"context"

	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type blackholeMetricSink struct {
}

var _ sinks.MetricSink = &blackholeMetricSink{}

// NewBlackholeMetricSink creates a new blackholeMetricSink. This sink does
// nothing at flush time, effectively "black holing" any metrics that are flushed.
// It is useful for tests that do not require any inspect of flushed metrics.
func NewBlackholeMetricSink() (*blackholeMetricSink, error) {
	return &blackholeMetricSink{}, nil
}

func (b *blackholeMetricSink) Name() string {
	return "blackhole"
}

func (b *blackholeMetricSink) Start(*trace.Client) error {
	return nil
}

func (b *blackholeMetricSink) Flush(context.Context, []samplers.InterMetric) error {
	return nil
}

func (b *blackholeMetricSink) FlushEventsChecks(ctx context.Context, events []samplers.UDPEvent, checks []samplers.UDPServiceCheck) {
	return
}

type blackholeSpanSink struct {
}

var _ sinks.SpanSink = &blackholeSpanSink{}

// NewBlackholeSpanSink creates a new blackholeSpanSink. This sink does
// nothing at flush time, effectively "black holing" any spans that are flushed.
// It is useful for tests that do not require any inspect of flushed spans.
func NewBlackholeSpanSink() (*blackholeSpanSink, error) {
	return &blackholeSpanSink{}, nil
}

func (b *blackholeSpanSink) Name() string {
	return "blackhole"
}

// Start performs final adjustments on the sink.
func (b *blackholeSpanSink) Start(*trace.Client) error {
	return nil
}

func (b *blackholeSpanSink) Ingest(ssf.SSFSpan) error {
	return nil
}

func (b *blackholeSpanSink) Flush() {
	return
}
