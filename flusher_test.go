package veneur

import (
	"context"
	"testing"
	"time"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/internal/forwardtest"
	"github.com/stripe/veneur/v14/samplers/metricpb"
)

func TestServerFlushGRPC(t *testing.T) {
	done := make(chan []string)
	testServer := forwardtest.NewServer(func(ms []*metricpb.Metric) {
		var names []string
		for _, m := range ms {
			names = append(names, m.Name)
		}
		done <- names
	})
	testServer.Start(t)
	defer testServer.Stop()

	localCfg := localConfig()
	localCfg.DebugFlushedMetrics = true
	localCfg.ForwardAddress = testServer.Addr().String()
	localCfg.ForwardUseGrpc = true
	local := setupVeneurServer(t, localCfg, nil, nil, nil, nil)
	defer local.Shutdown()

	inputs := forwardGRPCTestMetrics()
	for _, input := range inputs {
		local.Workers[0].ProcessMetric(input)
	}

	expected := []string{
		testGRPCMetric("histogram"),
		testGRPCMetric("histogram_global"),
		testGRPCMetric("timer"),
		testGRPCMetric("timer_mixed"),
		testGRPCMetric("counter"),
		testGRPCMetric("gauge"),
		testGRPCMetric("set"),
	}

	// Wait until the running server has flushed out the metrics for us:
	select {
	case v := <-done:
		log.Print("got the goods")
		assert.ElementsMatch(t, expected, v,
			"Flush didn't output the right metrics")
	case <-time.After(time.Second):
		log.Print("timed out")
		t.Fatal("Timed out waiting for the gRPC server to receive the flush")
	}
}

func TestServerFlushGRPCTimeout(t *testing.T) {
	testServer := forwardtest.NewServer(func(ms []*metricpb.Metric) {
		time.Sleep(500 * time.Millisecond)
	})
	testServer.Start(t)
	defer testServer.Stop()

	spanCh := make(chan *ssf.SSFSpan, 900)
	cl, err := trace.NewChannelClient(spanCh)
	require.NoError(t, err)
	defer func() {
		cl.Close()
	}()

	got := make(chan *ssf.SSFSample)
	go func() {
		for span := range spanCh {
			for _, sample := range span.Metrics {
				if sample.Name == "forward.error_total" && span.Tags != nil && sample.Tags["cause"] == "deadline_exceeded" {
					got <- sample
					return
				}
			}
		}
	}()

	localCfg := localConfig()
	localCfg.Interval = "20us"
	localCfg.DebugFlushedMetrics = true
	localCfg.ForwardAddress = testServer.Addr().String()
	localCfg.ForwardUseGrpc = true
	local := setupVeneurServer(t, localCfg, nil, nil, nil, cl)
	defer local.Shutdown()

	inputs := forwardGRPCTestMetrics()
	for _, input := range inputs {
		local.Workers[0].ProcessMetric(input)
	}

	// Wait until the running server has flushed out the metrics for us:
	select {
	case v := <-got:
		assert.Equal(t, float32(1.0), v.Value)
	case <-time.After(time.Second):
		t.Fatal("The server didn't time out flushing.")
	}
}

// Just test that a flushing to a bad address is handled without panicing
func TestServerFlushGRPCBadAddress(t *testing.T) {
	rcv := make(chan []samplers.InterMetric, 10)
	sink, err := NewChannelMetricSink(rcv)
	require.NoError(t, err)

	localCfg := localConfig()
	localCfg.ForwardAddress = "bad-address:123"
	localCfg.ForwardUseGrpc = true

	local := setupVeneurServer(t, localCfg, nil, sink, nil, nil)
	defer local.Shutdown()

	local.Workers[0].ProcessMetric(forwardGRPCTestMetrics()[0])
	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "counter",
			Type: counterTypeName,
		},
		Value:      20.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	}
	local.Workers[0].ProcessMetric(&m)
	select {
	case <-rcv:
		t.Log("Received my metric, assume this is working.")
	case <-time.After(time.Second * 5):
		t.Fatal("timed out waiting for global veneur flush")
	}
}

// Ensure that if someone sends a histogram to the global stats box directly,
// it emits both aggregates and percentiles (basically behaves like a global
// histo).
func TestGlobalAcceptsHistogramsOverUDP(t *testing.T) {
	rcv := make(chan []samplers.InterMetric, 10)
	sink, err := NewChannelMetricSink(rcv)
	require.NoError(t, err)

	cfg := globalConfig()
	cfg.Percentiles = []float64{}
	cfg.Aggregates = []string{"min"}
	global := setupVeneurServer(t, cfg, nil, sink, nil, nil)
	defer global.Shutdown()

	// simulate introducing a histogram to a global veneur.
	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "histo",
			Type: histogramTypeName,
		},
		Value:      20.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	}
	global.Workers[0].ProcessMetric(&m)
	global.Flush(context.Background())

	select {
	case results := <-rcv:
		assert.Len(t, results, 1, "too many metrics for global histo flush")
		assert.Equal(t, "histo.min", results[0].Name, "flushed global metric was incorrect aggregate")
	case <-time.After(time.Second * 5):
		t.Fatal("timed out waiting for global veneur flush")
	}
}

func TestFlushResetsWorkerUniqueMTS(t *testing.T) {
	config := localConfig()
	config.CountUniqueTimeseries = true
	config.NumWorkers = 2
	config.Interval = "60s"
	config.StatsdListenAddresses = []string{"udp://127.0.0.1:0", "udp://127.0.0.1:0"}
	ch := make(chan []samplers.InterMetric, 20)
	sink, _ := NewChannelMetricSink(ch)
	f := newFixture(t, config, sink, nil)
	defer f.Close()

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Digest: 1,
		Scope:  samplers.LocalOnly,
	}

	for _, w := range f.server.Workers {
		w.SampleTimeseries(&m)
		assert.Equal(t, uint64(1), w.uniqueMTS.Estimate())
	}

	f.server.Flush(context.Background())
	for _, w := range f.server.Workers {
		assert.Equal(t, uint64(0), w.uniqueMTS.Estimate())
	}
}

func TestTallyTimeseries(t *testing.T) {
	config := localConfig()
	config.CountUniqueTimeseries = true
	config.NumWorkers = 10
	config.Interval = "60s"
	config.StatsdListenAddresses = []string{"udp://127.0.0.1:0", "udp://127.0.0.1:0"}
	ch := make(chan []samplers.InterMetric, 20)
	sink, _ := NewChannelMetricSink(ch)
	f := newFixture(t, config, sink, nil)
	defer f.Close()

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Digest: 1,
		Scope:  samplers.LocalOnly,
	}
	for _, w := range f.server.Workers {
		w.SampleTimeseries(&m)
	}

	m2 := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Digest: 2,
		Scope:  samplers.LocalOnly,
	}
	f.server.Workers[0].SampleTimeseries(&m2)

	summary := f.server.tallyTimeseries()
	assert.Equal(t, int64(2), summary)
}
