// Package metrics provides routines for conveniently reporting
// metrics attached to SSF spans.
package metrics

import (
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

// ReportAsync sends a batch of one-off metrics to a trace client
// asynchronously. The channel done receives an error (or nil) when
// the span containing the batch of metrics has been sent.
func ReportAsync(cl *trace.Client, metrics []*ssf.SSFSample, done chan<- error) error {
	span := &ssf.SSFSpan{Metrics: metrics}
	return trace.Record(cl, span, done)
}

// Report sends a batch of one-off metrics to a trace client without
// waiting for a reply.
func Report(cl *trace.Client, metrics []*ssf.SSFSample) error {
	return ReportAsync(cl, metrics, nil)
}

// ReportOne sends a single metric to a veneur using a trace client
// without waiting for a reply.
func ReportOne(cl *trace.Client, metric *ssf.SSFSample) error {
	return ReportAsync(cl, []*ssf.SSFSample{metric}, nil)
}
