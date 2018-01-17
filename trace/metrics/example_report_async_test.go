package metrics_test

import (
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

func ExampleReportAsync() {
	// Create a slice of metrics and report them in one batch at the end of the function:
	samples := []*ssf.SSFSample{}

	// Let's add some metrics to the batch:
	samples = append(samples, ssf.Count("a.counter", 2, nil))
	samples = append(samples, ssf.Gauge("a.gauge", 420, nil))

	// report the batch:
	done := make(chan error)
	metrics.ReportAsync(trace.DefaultClient, samples, done)
	// ... and wait for it to send:
	<-done
	// Output:
}
