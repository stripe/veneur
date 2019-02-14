// package metricsClient exports a metrics client capable
package metricsClient

import (
	"time"

	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"

	"github.com/stripe/veneur/ssf"
)

// NewSSFAddingClient returns a SSFAddingClient that attaches metrics to an object
// that supports the Sampler interface.
//
// This exists for compatability with the existing convention of adding metrics
// to a Trace or SSFSample object which is later submitted in batch.
//
// Unlike the normal metric client, SSFAddingClient assumes the client will submit
// the sampler.
func NewSSFAddingClient(sampler Sampler) SSFAddingClient {
	return SSFAddingClient{sampler}
}

type SSFAddingClient struct {
	adder Sampler
}

func (s SSFAddingClient) Count(name string, incr float32, tags map[string]string) {
	s.adder.Add(ssf.Count(name, incr, tags))
}

func (s SSFAddingClient) Gauge(name string, value float32, tags map[string]string) {
	s.adder.Add(ssf.Gauge(name, value, tags))
}

func (s SSFAddingClient) Timing(name string, duration time.Duration, tags map[string]string) {
	s.adder.Add(ssf.Timing(name, duration, time.Nanosecond, tags))
}

type Sampler interface {
	Add(...*ssf.SSFSample)
}

// NewSSFDirectClient returns a SSFDirectClient that submits metrics directly using
// the trace client.
func NewSSFDirectClient(tc *trace.Client) SSFDirectClient {
	return SSFDirectClient{tc}
}

type SSFDirectClient struct {
	tc *trace.Client
}

func (s SSFDirectClient) Count(name string, incr float32, tags map[string]string) {
	metrics.ReportOne(s.tc, ssf.Count(name, incr, tags))
}

func (s SSFDirectClient) Gauge(name string, value float32, tags map[string]string) {
	metrics.ReportOne(s.tc, ssf.Gauge(name, value, tags))
}

func (s SSFDirectClient) Timing(name string, duration time.Duration, tags map[string]string) {
	metrics.ReportOne(s.tc, ssf.Timing(name, duration, time.Nanosecond, tags))
}
