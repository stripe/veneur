// Package metrics provides routines for conveniently reporting
// metrics attached to SSF spans.
package metrics

import (
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

// NoMetrics indicates that no metrics were included in the batch of
// metrics to submit.
type NoMetrics struct{}

func (nm NoMetrics) Error() string {
	return "No metrics to send."
}

// ReportAsync sends a batch of one-off metrics to a trace client
// asynchronously. The channel done receives an error (or nil) when
// the span containing the batch of metrics has been sent.
//
// If metrics is empty, an error NoMetrics is returned and done does
// not receive any data.
func ReportAsync(cl *trace.Client, metrics []*ssf.SSFSample, done chan<- error) error {
	if len(metrics) == 0 {
		return NoMetrics{}
	}
	span := &ssf.SSFSpan{Metrics: metrics}
	return trace.Record(cl, span, done)
}

// Report sends a batch of one-off metrics to a trace client without
// waiting for a reply.  If metrics is empty, an error NoMetrics is
// returned
func Report(cl *trace.Client, metrics []*ssf.SSFSample) error {
	return ReportAsync(cl, metrics, nil)
}

// ReportOne sends a single metric to a veneur using a trace client
// without waiting for a reply.
func ReportOne(cl *trace.Client, metric *ssf.SSFSample) error {
	return ReportAsync(cl, []*ssf.SSFSample{metric}, nil)
}
