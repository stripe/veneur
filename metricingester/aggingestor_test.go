package metricingester_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/metricingester"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks/channel"
)

var testCases = []struct {
	in  []metricingester.Metric
	out []samplers.TestMetric
	msg string
}{

	// TODO(clin): Migrate test cases in server_test to here. Keep cases there
	// that test import path specific code.

	// set tests

	{
		metrics(set("test", "a"), set("test", "b"), set("test", "c")),
		samplers.TMetrics(
			samplers.TGauge("test", 3),
		),
		"set with multiple values failed",
	},

	// mixed histo tests

	{
		metrics(mixedhisto("test", 1, hn("a")), mixedhisto("test", 2, hn("b")), mixedhisto("test", 3, hn("c"))),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1, samplers.OptHostname("a")),
			samplers.TGauge("test.max", 1, samplers.OptHostname("a")),
			samplers.TCounter("test.count", 1, samplers.OptHostname("a")),
			samplers.TGauge("test.min", 2, samplers.OptHostname("b")),
			samplers.TGauge("test.max", 2, samplers.OptHostname("b")),
			samplers.TCounter("test.count", 1, samplers.OptHostname("b")),
			samplers.TGauge("test.min", 3, samplers.OptHostname("c")),
			samplers.TGauge("test.max", 3, samplers.OptHostname("c")),
			samplers.TCounter("test.count", 1, samplers.OptHostname("c")),
			samplers.TGauge("test.95percentile", 2.925),
			samplers.TGauge("test.50percentile", 2),
		),
		"mixed histogram with multiple values failed",
	},
	{
		metrics(mixedhisto("test", 1), mixedhisto("test", 2), mixedhisto("test", 3)),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1),
			samplers.TGauge("test.max", 3),
			samplers.TCounter("test.count", 3),
			samplers.TGauge("test.95percentile", 2.925),
			samplers.TGauge("test.50percentile", 2),
		),
		"mixed histogram with multiple hosts failed",
	},

	// histo tests

	{
		metrics(histo("test", 1), histo("test", 2), histo("test", 3)),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1),
			samplers.TGauge("test.max", 3),
			samplers.TCounter("test.count", 3),
			samplers.TGauge("test.95percentile", 2.925),
			samplers.TGauge("test.50percentile", 2),
		),
		"histogram with multiple values failed",
	},
	{
		metrics(histo("test", 1)),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1),
			samplers.TGauge("test.max", 1),
			samplers.TCounter("test.count", 1),
			samplers.TGauge("test.95percentile", 1),
			samplers.TGauge("test.50percentile", 1),
		),
		"basic histogram with single value failed",
	},

	// status check tests

	{
		metrics(statusCheck("test", "message")),
		samplers.TMetrics(samplers.TStatus("test", "message")),
		"basic status check failed",
	},
	{
		metrics(statusCheck("test", "message"), statusCheck("test2", "message2")),
		samplers.TMetrics(samplers.TStatus("test", "message"), samplers.TStatus("test2", "message2")),
		"multiple status checks failed",
	},
	{
		metrics(statusCheck("test", "message"), statusCheck("test", "message2")),
		samplers.TMetrics(samplers.TStatus("test", "message2")),
		"status check not last write wins",
	},
	{
		metrics(statusCheck("test", "message", hn("myhost"))),
		samplers.TMetrics(samplers.TStatus("test", "message", samplers.OptHostname("myhost"))),
		"status check incorrect host name",
	},
	{
		metrics(
			statusCheck("test", "message", tags("a:b")),
			statusCheck("test", "message", tags("c:d")),
		),
		samplers.TMetrics(
			samplers.TStatus("test", "message", samplers.OptTags("a:b")),
			samplers.TStatus("test", "message", samplers.OptTags("c:d")),
		),
		"status check not aggregated by tags",
	},
	{
		metrics(
			statusCheck("test", "message"),
			counter("test", 3),
		),
		samplers.TMetrics(
			samplers.TStatus("test", "message"),
			samplers.TCounter("test", 3),
		),
		"status check not aggregated separatelyby type",
	},
}

func TestAggIngestor(t *testing.T) {
	for _, tc := range testCases {
		t.Run(tc.msg, testAggIngestor(tc.in, tc.out))
	}
}

func testAggIngestor(ins []metricingester.Metric, outs []samplers.TestMetric) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// SETUP TEST

		rc := make(chan []samplers.InterMetric, 20)
		cms, _ := channel.NewChannelMetricSink(rc)

		flushc := make(chan time.Time)
		workers := 1
		ing := metricingester.NewFlushingIngester(
			workers, // we want to test the parallel case
			1,       // this field is practically meaningless since we override the flush channel
			[]metricingester.Sink{cms},
			[]float64{0.5, 0.95},
			samplers.AggregateMin|samplers.AggregateMax|samplers.AggregateCount,
			metricingester.OptFlushChan(flushc), // override the flush ticker channel so we control when flush
		)

		// EXECUTE THE TEST

		ing.Start()
		defer ing.Stop()
		for _, in := range ins {
			err := ing.Ingest(context.Background(), in)
			require.NoError(t, err)
		}
		flushc <- time.Now()

		// COLLECT RESULTS

		var results []samplers.InterMetric
		for i := 0; i < workers; i++ {
			select {
			case result := <-rc:
				results = append(results, result...)
			case <-time.After(1 * time.Second):
				t.Fatal("took too long to flush metric results or no metrics to flush!")
			}
		}

		// ASSERT

		assert.ElementsMatch(
			t,
			outs,
			samplers.ToTestMetrics(results),
			"EXPECTED: %+v\nACTUAL: %+v",
			outs,
			samplers.ToTestMetrics(results),
		)
	}
}
