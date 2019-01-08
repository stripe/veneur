package metrics_test

import (
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

func ExampleReport() {
	// Create a slice of metrics and report them in one batch at the end of the function:
	samples := &ssf.Samples{}
	defer metrics.Report(trace.DefaultClient, samples)

	// Let's add some metrics to the batch:
	samples.Add(ssf.Count("a.counter", 2, nil))
	samples.Add(ssf.Gauge("a.gauge", 420, nil))

	// Output:
}
