package veneur

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
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

func newForwardGRPCFixture(t testing.TB, localConfig Config, sink sinks.MetricSink) *forwardGRPCFixture {
	// Create a global Veneur
	globalCfg := globalConfig()
	globalCfg.GrpcAddress = unusedLocalTCPAddress(t)
	global := setupVeneurServer(t, globalCfg, nil, sink, nil)
	go func() {
		global.Serve()
	}()

	// Create a proxy Veneur
	proxyCfg := generateProxyConfig()
	proxyCfg.GrpcForwardAddress = globalCfg.GrpcAddress
	proxyCfg.GrpcAddress = unusedLocalTCPAddress(t)
	proxyCfg.ConsulForwardServiceName = ""
	proxy, err := NewProxyFromConfig(logrus.New(), proxyCfg)
	go func() {
		proxy.Serve()
	}()
	assert.NoError(t, err)

	localConfig.ForwardAddress = proxyCfg.GrpcAddress
	localConfig.ForwardUseGrpc = true
	local := setupVeneurServer(t, localConfig, nil, nil, nil)

	return &forwardGRPCFixture{t: t, proxy: &proxy, global: global, local: local}
}

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
		metrics := <-ch

		expectedNames := []string{
			testGRPCMetric("histogram.50percentile"),
			testGRPCMetric("histogram.75percentile"),
			testGRPCMetric("histogram.99percentile"),
			testGRPCMetric("timer.50percentile"),
			testGRPCMetric("timer.75percentile"),
			testGRPCMetric("timer.99percentile"),
			testGRPCMetric("counter"),
			testGRPCMetric("gauge"),
			testGRPCMetric("set"),
		}

		actualNames := make([]string, len(metrics))
		for i, metric := range metrics {
			actualNames[i] = metric.Name
		}

		assert.ElementsMatch(t, expectedNames, actualNames,
			"The global Veneur didn't flush the right metrics")
		close(done)
	}()
	ff.local.Flush(context.TODO())
	ff.global.Flush(context.TODO())
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Timed out waiting for a metric after 3 seconds")
	}
}
