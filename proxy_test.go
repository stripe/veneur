package veneur

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/discovery"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/zenazn/goji/graceful"
)

func generateProxyConfig() ProxyConfig {
	return ProxyConfig{
		Debug:                    false,
		ConsulRefreshInterval:    24 * time.Hour,
		ConsulForwardServiceName: "forwardServiceName",
		ConsulTraceServiceName:   "traceServiceName",
		TraceAddress:             "127.0.0.1:8128",
		TraceAPIAddress:          "127.0.0.1:8135",
		HTTPAddress:              "127.0.0.1:0",
		GrpcAddress:              "127.0.0.1:0",
		StatsAddress:             "127.0.0.1:8201",
	}
}

func TestAllowStaticServices(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ConsulForwardServiceName = ""
	proxyConfig.ConsulTraceServiceName = ""
	proxyConfig.ForwardAddress = "localhost:1234"
	proxyConfig.TraceAddress = "localhost:1234"

	server, error := NewProxyFromConfig(logrus.New(), proxyConfig, nil)
	assert.NoError(t, error, "Should start with just static services")
	assert.Nil(t, server.Discoverer)
}

func TestMissingServices(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ForwardAddress = ""
	proxyConfig.TraceAddress = ""
	proxyConfig.ConsulForwardServiceName = ""
	proxyConfig.ConsulTraceServiceName = ""

	_, error := NewProxyFromConfig(logrus.New(), proxyConfig, nil)
	assert.Error(t, error, "No consul services means Proxy won't start")
}

func TestAcceptingBooleans(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ConsulTraceServiceName = ""
	proxyConfig.TraceAddress = ""

	server, _ := NewProxyFromConfig(logrus.New(), proxyConfig, nil)
	assert.True(t, server.AcceptingForwards, "Server accepts forwards")
}

func TestDiscovererQuery(t *testing.T) {
	// Make the discoverer
	ctrl := gomock.NewController(t)
	discoverer := discovery.NewMockDiscoverer(ctrl)

	discoverer.EXPECT().GetDestinationsForService(gomock.Any()).Return([]string{
		"localhost:9000", "localhost:9001",
	}, nil)

	// Make the proxy
	proxyConfig := generateProxyConfig()
	proxyConfig.ForwardAddress = "localhost:1234"
	proxyConfig.ConsulForwardServiceName = "forwardServiceName"
	proxyConfig.Debug = true

	server, _ := NewProxyFromConfig(logrus.New(), proxyConfig, discoverer)
	defer server.Shutdown()

	server.Start()
	srv := httptest.NewServer(server.Handler())
	defer srv.Close()

	// Make sure we're sane first
	assert.Len(t, server.ForwardDestinations.Members(), 2, "Incorrect host count in ring")
}

func TestTimeout(t *testing.T) {
	cfg := generateProxyConfig()
	cfg.ConsulTraceServiceName = ""
	cfg.ConsulForwardServiceName = ""

	never := make(chan struct{})
	defer close(never)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-never
	}))
	defer ts.Close()

	cfg.ForwardAddress = ts.URL
	cfg.ForwardTimeout = time.Nanosecond // just really really short
	server, _ := NewProxyFromConfig(logrus.New(), cfg, nil)

	ctr := samplers.Counter{Name: "foo", Tags: []string{}}
	ctr.Sample(20.0, 1.0)
	jsonCtr, err := ctr.Export()
	require.NoError(t, err)
	metrics := []samplers.JSONMetric{jsonCtr}

	// Now send the metrics to a server that never responds; we
	// expect this to return before our (sufficiently long)
	// timeout:
	ch := make(chan struct{})
	go func() {
		server.ProxyMetrics(context.Background(), metrics, "foo.com")
		close(ch)
	}()
	select {
	case <-ch:
		t.Log("Returned quickly, great.")
	case <-time.After(3 * time.Second):
		t.Fatal("Proxy took too long to time out. Is the deadline behavior working?")
	}
}

// Test that (*Proxy).Serve quits when just the gRPC server is stopped.  The
// expected behavior is that both listeners (gRPC and HTTP) stop when either
// of them are stopped.
//
// This also verifies that it is safe to call (*Proxy).gRPCStop() multiple times
// as the cleanup routing (*Proxy).Shutdown() will call it again after it is
// called in this test.
func TestProxyStopGRPC(t *testing.T) {
	p, err := NewProxyFromConfig(logrus.New(), generateProxyConfig(), nil)
	assert.NoError(t, err, "Creating a proxy server shouldn't have caused an error")

	done := make(chan struct{})
	go func() {
		p.Serve()
		defer p.Shutdown()
		close(done)
	}()

	// Only stop the gRPC server. This should cause (*Proxy).Serve to exit.
	p.gRPCStop()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Stopping the gRPC server did not cause both listeners to exit")
	}
}

// Test that stopping the HTTP server causes the Proxy to stop serving over
// both HTTP and gRPC.  As Serve will attempt to close any open HTTP servers
// again, this also tests that graceful.Shutdown is safe to be called multiple times.
func TestProxyServeStopHTTP(t *testing.T) {
	p, err := NewProxyFromConfig(logrus.New(), generateProxyConfig(), nil)
	assert.NoError(t, err, "Creating a Proxy shouldn't have caused an error")

	done := make(chan struct{})
	go func() {
		p.Serve()
		defer p.Shutdown()
		close(done)
	}()

	// Stop the HTTP server only, causing Serve to exit.
	waitForHTTPStart(t, p, 3*time.Second)
	graceful.ShutdownNow()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Stopping the Proxy over HTTP did not stop both listeners")
	}
}
