package ssf_test

import (
	"time"

	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

func ExampleRandomlySample() {
	// Sample some metrics at 50% - each of these metrics, if it
	// gets picked, will report with a SampleRate of 0.5:
	samples := ssf.RandomlySample(0.5,
		ssf.Count("cheap.counter", 1, nil),
		ssf.Timing("cheap.timer", 1*time.Second, time.Nanosecond, nil),
	)

	// Sample another metric at 1% - if included, the metric will
	// have a SampleRate of 0.01:
	samples = append(samples, ssf.RandomlySample(0.01,
		ssf.Count("expensive.counter", 20, nil))...)

	// Report these metrics:
	metrics.Report(trace.DefaultClient, samples)
	// Output:
}
