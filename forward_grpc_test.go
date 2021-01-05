package veneur

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
)

const (
	grpcTestMetricPrefix = "test.grpc."
)

type forwardGRPCFixture struct {
	t      testing.TB
	proxy  *Proxy
	global *Server
	local  *Server
}

// newForwardGRPCFixture creates a set of resources that forward to each other
// over gRPC.  Specifically this includes a local Server, which forwards
// metrics over gRPC to a Proxy, which then forwards over gRPC again to a
// global Server.
func newForwardGRPCFixture(t testing.TB, localConfig Config, sink sinks.MetricSink) *forwardGRPCFixture {
	// Create a global Veneur
	globalCfg := globalConfig()
	globalCfg.GrpcAddress = unusedLocalTCPAddress(t)
	global := setupVeneurServer(t, globalCfg, nil, sink, nil, nil)
	go func() {
		global.Serve()
	}()
	waitForHTTPStart(t, global, 3*time.Second)

	// Create a proxy Veneur
	proxyCfg := generateProxyConfig()
	proxyCfg.GrpcForwardAddress = globalCfg.GrpcAddress
	proxyCfg.GrpcAddress = unusedLocalTCPAddress(t)
	proxyCfg.ConsulForwardServiceName = ""
	proxy, err := NewProxyFromConfig(logrus.New(), proxyCfg)
	assert.NoError(t, err)
	go func() {
		proxy.Serve()
	}()
	waitForHTTPStart(t, &proxy, 3*time.Second)

	localConfig.ForwardAddress = proxyCfg.GrpcAddress
	localConfig.ForwardUseGrpc = true
	local := setupVeneurServer(t, localConfig, nil, nil, nil, nil)

	return &forwardGRPCFixture{t: t, proxy: &proxy, global: global, local: local}
}

// stop stops all of the various servers inside the fixture.
func (ff *forwardGRPCFixture) stop() {
	ff.proxy.Shutdown()
	ff.global.Shutdown()
	ff.local.Shutdown()
}

// IngestMetric synchronously writes a metric to the forwarding
// fixture's local veneur span worker. The fixture must be flushed via
// the (*forwardFixture).local.Flush method so the ingestion effects can be
// observed.
func (ff *forwardGRPCFixture) IngestMetric(m *samplers.UDPMetric) {
	ff.local.Workers[0].ProcessMetric(m)
}

// unusedLocalTCPAddress returns a host:port combination on the loopback
// interface that *should* be available for usage.

// This is definitely pretty hacky, but I was having a tricky time coming
// up with a good way to expose the gRPC listening address of the server or
// proxy without awkwardly saving the listener or listen address, and then
// modifying the gRPC serve function to create the network listener before
// spawning the goroutine.
func unusedLocalTCPAddress(t testing.TB) string {
	ln, err := net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatalf("Failed to bind to a test port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// testGRPCMetric appends a common prefix to the given metric name.
func testGRPCMetric(name string) string {
	return grpcTestMetricPrefix + name
}

func forwardGRPCTestMetrics() []*samplers.UDPMetric {
	return []*samplers.UDPMetric{
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("histogram"),
				Type: histogramTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("histogram_global"),
				Type: histogramTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("gauge"),
				Type: gaugeTypeName,
			},
			Value:      1.0,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("counter"),
				Type: counterTypeName,
			},
			Value:      2.0,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("timer_mixed"),
				Type: timerTypeName,
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("timer"),
				Type: timerTypeName,
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("set"),
				Type: setTypeName,
			},
			Value:      "test",
			SampleRate: 1.0,
			Scope:      samplers.GlobalOnly,
		},
		// Only global metrics should be forwarded
		&samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: testGRPCMetric("counter.local"),
				Type: counterTypeName,
			},
			Value:      100.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		},
	}
}

// TestE2EForwardingGRPCMetrics inputs a set of metrics to a local Veneur,
// and verifies that the same metrics are later flushed by the global Veneur
// after passing through a proxy.
func TestE2EForwardingGRPCMetrics(t *testing.T) {
	ch := make(chan []samplers.InterMetric)
	sink, _ := NewChannelMetricSink(ch)

	ff := newForwardGRPCFixture(t, localConfig(), sink)
	defer ff.stop()

	input := forwardGRPCTestMetrics()
	for _, metric := range input {
		ff.IngestMetric(metric)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)

		expected := map[string]bool{}
		for _, name := range []string{
			testGRPCMetric("histogram.50percentile"),
			testGRPCMetric("histogram.75percentile"),
			testGRPCMetric("histogram.99percentile"),
			testGRPCMetric("histogram_global.99percentile"),
			testGRPCMetric("histogram_global.50percentile"),
			testGRPCMetric("histogram_global.75percentile"),
			testGRPCMetric("histogram_global.max"),
			testGRPCMetric("histogram_global.min"),
			testGRPCMetric("histogram_global.count"),
			testGRPCMetric("timer_mixed.50percentile"),
			testGRPCMetric("timer_mixed.75percentile"),
			testGRPCMetric("timer_mixed.99percentile"),
			testGRPCMetric("timer.50percentile"),
			testGRPCMetric("timer.75percentile"),
			testGRPCMetric("timer.99percentile"),
			testGRPCMetric("timer.max"),
			testGRPCMetric("timer.min"),
			testGRPCMetric("timer.count"),
			testGRPCMetric("counter"),
			testGRPCMetric("gauge"),
			testGRPCMetric("set"),
		} {
			expected[name] = false
		}

	metrics:
		for {
			metrics := <-ch

			for _, metric := range metrics {
				_, ok := expected[metric.Name]
				if !ok {
					t.Errorf("unexpected metric %q", metric.Name)
					continue
				}
				expected[metric.Name] = true
			}
			for name, got := range expected {
				if !got {
					// we have more metrics to read:
					t.Logf("metric %q still missing", name)
					continue metrics
				}
			}
			// if there had been metrics to read, we'd
			// have restarted the loop:
			return
		}

	}()
	ff.local.Flush(context.TODO())
	ff.global.Flush(context.TODO())
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for a metric after 3 seconds")
	}
}
