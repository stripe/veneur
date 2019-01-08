package metrics_test

import (
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

func ExampleReportOne() {
	// Let's report a single metric (without any batching) using
	// the default client:
	metrics.ReportOne(trace.DefaultClient, ssf.Count("a.oneoff.counter", 1, nil))
	// Output:
}
