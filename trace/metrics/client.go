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

// Report sends one-off metric samples encapsulated in a Samples
// structure to a trace client without waiting for a reply.  If the
// batch of metrics is empty, an error NoMetrics is returned.
func Report(cl *trace.Client, samples *ssf.Samples) error {
	return ReportBatch(cl, samples.Batch)
}

// ReportBatch sends a batch of one-off metrics to a trace client without
// waiting for a reply.  If the batch of metrics is empty, an error
// NoMetrics is returned.
func ReportBatch(cl *trace.Client, samples []*ssf.SSFSample) error {
	return ReportAsync(cl, samples, nil)
}

// ReportAsync sends a batch of one-off metrics to a trace client
// asynchronously. The channel done receives an error (or nil) when
// the span containing the batch of metrics has been sent.
//
// If metrics is empty, an error NoMetrics is returned and done does
// not receive any data.
func ReportAsync(cl *trace.Client, metrics []*ssf.SSFSample, done chan<- error) error {
	if metrics == nil || len(metrics) == 0 {
		return NoMetrics{}
	}
	span := &ssf.SSFSpan{Metrics: metrics}
	return trace.Record(cl, span, done)
}

// ReportOne sends a single metric to a veneur using a trace client
// without waiting for a reply.
func ReportOne(cl *trace.Client, metric *ssf.SSFSample) error {
	return ReportAsync(cl, []*ssf.SSFSample{metric}, nil)
}
