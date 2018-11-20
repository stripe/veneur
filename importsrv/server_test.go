package importsrv_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/importsrv"
	"github.com/stripe/veneur/metricingester"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/sinks/channel"
)

var testE2EFlushingCases = []struct {
	in  []*metricpb.Metric
	out []samplers.TestMetric
	msg string
}{

	// basic test cases

	{
		pbmetrics(pbcounter("test", 5)),
		samplers.TMetrics(samplers.TCounter("test", 5)),
		"counter not present",
	},
	{
		pbmetrics(pbgauge("test", 100.5)),
		samplers.TMetrics(samplers.TGauge("test", 100.5)),
		"gauge not present",
	},
	{
		pbmetrics(pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_MixedGlobal))),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1),
			samplers.TGauge("test.max", 3),
			samplers.TGauge("test.count", 3),
			samplers.TGauge("test.50percentile", 2),
			samplers.TGauge("test.95percentile", 2.925),
		),
		"mixed global histo not present",
	},
	{
		pbmetrics(pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_Mixed))),
		samplers.TMetrics(
			samplers.TGauge("test.50percentile", 2),
			samplers.TGauge("test.95percentile", 2.925),
		),
		"mixed histo not correct",
	},
	{
		pbmetrics(pbset("test", []string{"asf", "clin", "aditya"})),
		samplers.TMetrics(samplers.TGauge("test", 3)),
		"set not present",
	},

	// basic tests that things are aggregated correctly

	{
		pbmetrics(pbcounter("test", 5), pbcounter("test", 10), pbcounter("test2", 3)),
		samplers.TMetrics(samplers.TCounter("test", 15), samplers.TCounter("test2", 3)),
		"counters not aggregated correctly",
	},
	{
		pbmetrics(pbgauge("test", 5), pbgauge("test", 10)),
		samplers.TMetrics(samplers.TGauge("test", 10)),
		"gauge not aggregated correctly",
	},
	{
		pbmetrics(pbset("test", []string{"hey", "hey2"}), pbset("test", []string{"hey3", "hey4"})),
		samplers.TMetrics(samplers.TGauge("test", 4)),
		"sets not aggregated correctly",
	},
	{
		pbmetrics(pbcounter("test", 5), pbcounter("test", 10)),
		samplers.TMetrics(samplers.TCounter("test", 15)),
		"counters not aggregated correctly",
	},
	{
		pbmetrics(pbcounter("test", 3), pbgauge("test", 4), pbset("test", []string{"a", "b"})),
		samplers.TMetrics(samplers.TCounter("test", 3), samplers.TGauge("test", 4), samplers.TGauge("test", 2)),
		"different types not aggregated separately",
	},

	// test special aggregation rules

	{
		pbmetrics(
			pbcounter("test", 5, pbtags("a:b", "c:d")),
			pbcounter("test", 3, pbtags("c:d", "a:b")),
		),
		samplers.TMetrics(samplers.TCounter("test", 8, samplers.OptTags("a:b", "c:d"))),
		"out of order tags don't aggregate together",
	},
	{
		pbmetrics(
			pbcounter("test", 5, pbhostname("a")),
			pbcounter("test", 5, pbhostname("a")),
			pbcounter("test", 3, pbhostname("b")),
			pbcounter("test", 3, pbhostname("")),
		),
		samplers.TMetrics(
			samplers.TCounter("test", 10, samplers.OptHostname("a")),
			samplers.TCounter("test", 3, samplers.OptHostname("b")),
			samplers.TCounter("test", 3, samplers.OptHostname("")),
		),
		"hostnames not being aggregated separately",
	},
	{
		pbmetrics(
			pbcounter("test", 5, pbhostname("a"), pbscope(metricpb.Scope_Local)),
			pbcounter("test", 5, pbhostname("a"), pbscope(metricpb.Scope_Global)),
			pbcounter("test", 5, pbhostname("a"), pbscope(metricpb.Scope_Mixed)),
			pbcounter("test", 3, pbhostname("a")),
		),
		samplers.TMetrics(
			samplers.TCounter("test", 18, samplers.OptHostname("a")),
		),
		"scope field should be ignored for counter types",
	},
	{
		pbmetrics(
			pbgauge("test", 5, pbhostname("a"), pbscope(metricpb.Scope_Local)),
			pbgauge("test", 6, pbhostname("a"), pbscope(metricpb.Scope_Global)),
			pbgauge("test", 7, pbhostname("a"), pbscope(metricpb.Scope_Mixed)),
			pbgauge("test", 3, pbhostname("a")),
		),
		samplers.TMetrics(
			samplers.TGauge("test", 3, samplers.OptHostname("a")),
		),
		"scope field should be ignored for gauge types",
	},

	// mixed histogram fun

	{
		pbmetrics(
			pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_MixedGlobal), pbhostname("a")),
		),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1, samplers.OptHostname("a")),
			samplers.TGauge("test.max", 3, samplers.OptHostname("a")),
			samplers.TGauge("test.count", 3, samplers.OptHostname("a")),
			samplers.TGauge("test.50percentile", 2),
			samplers.TGauge("test.95percentile", 2.925),
		),
		"global mixed histos not reporting host level aggregates for one host",
	},
	{
		pbmetrics(
			pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_Mixed), pbhostname("a")),
		),
		samplers.TMetrics(
			samplers.TGauge("test.50percentile", 2),
			samplers.TGauge("test.95percentile", 2.925),
		),
		"mixed histos should not report host metrics",
	},
	{
		pbmetrics(
			pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_MixedGlobal), pbhostname("a")),
			pbhisto("test", []float64{4, 5, 6}, pbscope(metricpb.Scope_MixedGlobal), pbhostname("b")),
		),
		samplers.TMetrics(
			samplers.TGauge("test.min", 1, samplers.OptHostname("a")),
			samplers.TGauge("test.max", 3, samplers.OptHostname("a")),
			samplers.TGauge("test.count", 3, samplers.OptHostname("a")),
			samplers.TGauge("test.min", 4, samplers.OptHostname("b")),
			samplers.TGauge("test.max", 6, samplers.OptHostname("b")),
			samplers.TGauge("test.count", 3, samplers.OptHostname("b")),
			samplers.TGauge("test.50percentile", 3.5),
			samplers.TGauge("test.95percentile", 5.85),
		),
		"global mixed histos not reporting host level aggregates for two hosts",
	},
	{
		pbmetrics(
			pbhisto("test", []float64{1, 2, 3}, pbscope(metricpb.Scope_Mixed), pbhostname("a")),
			pbhisto("test", []float64{4, 5, 6}, pbscope(metricpb.Scope_Mixed), pbhostname("b")),
		),
		samplers.TMetrics(
			samplers.TGauge("test.50percentile", 3.5),
			samplers.TGauge("test.95percentile", 5.85),
		),
		"mixed histos not reporting host level aggregates for two hosts",
	},

	// sink tests

	// crazy random "real world" tests
}

// TestE2EFlushingIngester tests the integration of the import endpoint with
// the flushing ingester.
func TestE2EFlushingIngester(t *testing.T) {
	for _, tc := range testE2EFlushingCases {
		t.Run(tc.msg, test(tc.in, tc.out, tc.msg))
	}
}

func test(in []*metricpb.Metric, out []samplers.TestMetric, msg string) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()

		// SETUP TEST

		// this sink collects the results
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
		s := importsrv.New(ing)
		defer s.Stop()

		// EXECUTE THE TEST

		ing.Start()
		defer ing.Stop()
		_, err := s.SendMetrics(context.Background(), &forwardrpc.MetricList{Metrics: in})
		require.NoError(t, err)
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
			out,
			samplers.ToTestMetrics(results),
			"EXPECTED: %+v\nACTUAL: %+v",
			out,
			samplers.ToTestMetrics(results),
		)
	}
}

//func TestOptions_WithTraceClient(t *testing.T) {
//	c, err := trace.NewClient(trace.DefaultVeneurAddress)
//	if err != nil {
//		t.Fatalf("failed to initialize a trace client: %v", err)
//	}
//
//	s := New([]MetricIngester{}, WithTraceClient(c))
//	assert.Equal(t, c, s.opts.traceClient, "WithTraceClient didn't correctly "+
//		"set the trace client")
//}
//
//func BenchmarkImportServerSendMetrics(b *testing.B) {
//	rand.Seed(time.Now().Unix())
//
//	metrics := metrictest.RandomForwardMetrics(10000)
//	for _, inputSize := range []int{10, 100, 1000, 10000} {
//		ingesters := make([]MetricIngester, 100)
//		for i := range ingesters {
//			ingester := newNoopChannelMetricIngester()
//			ingester.start()
//			defer ingester.stop()
//			ingesters[i] = ingester
//		}
//		s := New(ingesters)
//		ctx := context.Background()
//		input := &forwardrpc.MetricList{Metrics: metrics[:inputSize]}
//
//		b.Run(fmt.Sprintf("InputSize=%d", inputSize), func(b *testing.B) {
//			for i := 0; i < b.N; i++ {
//				s.SendMetrics(ctx, input)
//			}
//		})
//	}
//}
