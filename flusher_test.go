package veneur

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
	"github.com/stripe/veneur/v14/util/matcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/internal/forwardtest"
	"github.com/stripe/veneur/v14/samplers/metricpb"
)

func forwardGRPCTestMetrics() []*samplers.UDPMetric {
	return []*samplers.UDPMetric{{
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.histogram",
			Type: HistogramTypeName,
		},
		Value:      20.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.histogram_global",
			Type: HistogramTypeName,
		},
		Value:      20.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.gauge",
			Type: GaugeTypeName,
		},
		Value:      1.0,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.counter",
			Type: CounterTypeName,
		},
		Value:      2.0,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.timer_mixed",
			Type: TimerTypeName,
		},
		Value:      100.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.timer",
			Type: TimerTypeName,
		},
		Value:      100.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.set",
			Type: SetTypeName,
		},
		Value:      "test",
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}, {
		MetricKey: samplers.MetricKey{
			Name: "test.grpc.counter.local",
			Type: CounterTypeName,
		},
		Value:      100.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	}}
}

func TestServerFlushGRPC(t *testing.T) {
	logger := logrus.NewEntry(logrus.New())
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
		"test.grpc.histogram",
		"test.grpc.histogram_global",
		"test.grpc.timer",
		"test.grpc.timer_mixed",
		"test.grpc.counter",
		"test.grpc.gauge",
		"test.grpc.set",
	}

	// Wait until the running server has flushed out the metrics for us:
	select {
	case v := <-done:
		logger.Print("got the goods")
		assert.ElementsMatch(t, expected, v,
			"Flush didn't output the right metrics")
	case <-time.After(time.Second):
		logger.Print("timed out")
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
	localCfg.Interval = time.Duration(20 * time.Microsecond)
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
			Type: CounterTypeName,
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
			Type: HistogramTypeName,
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
	config.Interval = time.Duration(time.Minute)
	config.StatsdListenAddresses = []util.Url{{
		Value: &url.URL{
			Scheme: "udp",
			Host:   "127.0.0.1:0",
		}}, {
		Value: &url.URL{
			Scheme: "udp",
			Host:   "127.0.0.1:0",
		}}}
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
	config.Interval = time.Duration(time.Minute)
	config.StatsdListenAddresses = []util.Url{{
		Value: &url.URL{
			Scheme: "udp",
			Host:   "127.0.0.1:0",
		}}, {
		Value: &url.URL{
			Scheme: "udp",
			Host:   "127.0.0.1:0",
		}}}
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

func TestStripTags(t *testing.T) {
	config := localConfig()
	config.Features.EnableMetricSinkRouting = true
	config.MetricSinks = []SinkConfig{{
		Kind: "channel",
		Name: "channel",
		StripTags: []matcher.TagMatcher{
			matcher.CreateTagMatcher(&matcher.TagMatcherConfig{
				Kind:  "prefix",
				Value: "foo",
			})},
	}}
	config.MetricSinkRouting = []SinkRoutingConfig{{
		Name: "default",
		Match: []matcher.Matcher{{
			Name: matcher.CreateNameMatcher(&matcher.NameMatcherConfig{
				Kind: "any",
			}),
			Tags: []matcher.TagMatcher{},
		}},
		Sinks: SinkRoutingSinks{
			Matched: []string{"channel"},
		},
	}}

	channel := make(chan []samplers.InterMetric)
	server, err := NewFromConfig(ServerConfig{
		Logger: logrus.New(),
		Config: config,
		MetricSinkTypes: MetricSinkTypes{
			"channel": {
				Create: func(
					server *Server, s2 string, logger *logrus.Entry, config Config,
					sinkConfig MetricSinkConfig,
				) (sinks.MetricSink, error) {
					sink, err := NewChannelMetricSink(channel)
					if err != nil {
						return nil, err
					}
					return sink, nil
				},
				ParseConfig: func(s string, i interface{}) (MetricSinkConfig, error) {
					return nil, nil
				},
			},
		},
	})
	assert.NoError(t, err)
	go server.Start()
	defer server.Shutdown()

	server.Workers[0].PacketChan <- samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name:       "test.metric",
			Type:       "counter",
			JoinedTags: "foo:value1,bar:value2",
		},
		Digest:     0,
		Scope:      samplers.LocalOnly,
		Tags:       []string{"foo:value1", "bar:value2"},
		Value:      1.0,
		SampleRate: 1.0,
	}

	result := <-channel
	assert.Len(t, result, 1)
	assert.Equal(t, "test.metric", result[0].Name)
	assert.Len(t, result[0].Tags, 1)
	assert.Equal(t, "bar:value2", result[0].Tags[0])
}
