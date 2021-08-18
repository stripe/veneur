package cortex

import (
	"context"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

// CortexMetricSink writes metrics to one or more configured Cortex instances
// using the prometheus remote-write API. For specifications, see
// https://github.com/prometheus/compliance/tree/main/remote_write
type CortexMetricSink struct{}

// NewCortexMetricSink creates and returns a new instance of the sink
func NewCortexMetricSink() (*CortexMetricSink, error) {
	return &CortexMetricSink{}, nil
}

// Name returns the string cortex
func (s *CortexMetricSink) Name() string {
	return "cortex"
}

func (s *CortexMetricSink) Start(*trace.Client) error {
	return nil
}

func (s *CortexMetricSink) Flush(context.Context, []samplers.InterMetric) error {
	return nil
}

func (s *CortexMetricSink) FlushOtherSamples(context.Context, []ssf.SSFSample) {
	return
}
