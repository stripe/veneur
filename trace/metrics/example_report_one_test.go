package metrics_test

import (
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
)

func ExampleReportOne() {
	// Let's report a single metric (without any batching) using
	// the default client:
	metrics.ReportOne(trace.DefaultClient, ssf.Count("a.oneoff.counter", 1, nil))
	// Output:
}
