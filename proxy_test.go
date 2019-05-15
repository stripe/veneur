package veneur

import (
	"compress/zlib"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/samplers"
	"github.com/zenazn/goji/graceful"
)

func generateProxyConfig() ProxyConfig {
	return ProxyConfig{
		Debug:                    false,
		ConsulRefreshInterval:    "86400s",
		ConsulForwardServiceName: "forwardServiceName",
		ConsulTraceServiceName:   "traceServiceName",
		TraceAddress:             "127.0.0.1:8128",
		TraceAPIAddress:          "127.0.0.1:8135",
		HTTPAddress:              "127.0.0.1:0",
		GrpcAddress:              "127.0.0.1:0",
		StatsAddress:             "127.0.0.1:8201",
	}
}

type ConsulTwoMetricRoundTripper struct {
	t         *testing.T
	wg        *sync.WaitGroup
	aReceived bool
	bReceived bool

	mtx sync.Mutex
}

func (rt *ConsulTwoMetricRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Ensure that only one RoundTrip is happening at once
	// to prevent dataraces on aReceived and bReceived

	rt.mtx.Lock()
	defer rt.mtx.Unlock()

	rec := httptest.NewRecorder()
	if req.URL.Path == "/v1/health/service/forwardServiceName" {
		resp, _ := ioutil.ReadFile("testdata/consul/health_service_two.json")
		rec.Write(resp)
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/v1/health/service/traceServiceName" {
		resp, _ := ioutil.ReadFile("testdata/consul/health_service_two.json")
		rec.Write(resp)
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/api/v1/series" {
		// Just make the datadog bit work
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/import" && req.Host == "10.1.10.12:8000" {
		z, _ := zlib.NewReader(req.Body)
		body, _ := ioutil.ReadAll(z)
		defer req.Body.Close()
		if strings.Contains(string(body), "y.b.c") {
			rt.aReceived = true
		}
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/import" && req.Host == "10.1.10.13:8000" {
		z, _ := zlib.NewReader(req.Body)
		body, _ := ioutil.ReadAll(z)
		defer req.Body.Close()
		if strings.Contains(string(body), "a.b.c") {
			rt.bReceived = true
		}
		rec.Code = http.StatusOK
	} else {
		assert.Fail(rt.t, "Received an unexpected request: %s %s", req.Host, req.URL.Path)
	}

	// If we've gotten all of them, fire!
	if rt.aReceived && rt.bReceived {
		rt.wg.Done()
	}

	return rec.Result(), nil
}

func TestAllowStaticServices(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ConsulForwardServiceName = ""
	proxyConfig.ConsulTraceServiceName = ""
	proxyConfig.ForwardAddress = "localhost:1234"
	proxyConfig.TraceAddress = "localhost:1234"

	server, error := NewProxyFromConfig(logrus.New(), proxyConfig)
	assert.NoError(t, error, "Should start with just static services")
	assert.False(t, server.usingConsul, "Server isn't using consul")
}

func TestMissingServices(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ForwardAddress = ""
	proxyConfig.TraceAddress = ""
	proxyConfig.ConsulForwardServiceName = ""
	proxyConfig.ConsulTraceServiceName = ""

	_, error := NewProxyFromConfig(logrus.New(), proxyConfig)
	assert.Error(t, error, "No consul services means Proxy won't start")
}

func TestAcceptingBooleans(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ConsulTraceServiceName = ""
	proxyConfig.TraceAddress = ""

	server, _ := NewProxyFromConfig(logrus.New(), proxyConfig)
	assert.True(t, server.AcceptingForwards, "Server accepts forwards")
	assert.False(t, server.AcceptingTraces, "Server does not forward traces")
}

func TestConsistentForward(t *testing.T) {

	// We need to set up a proxy, have a local veneur send to it, then verify
	// that the proxy forwards to two downstream fake globals.
	// TIME FOR SOME GAME THEORY

	// Make the proxy
	proxyConfig := generateProxyConfig()
	proxyConfig.ForwardAddress = "localhost:1234"
	proxyConfig.ConsulForwardServiceName = "forwardServiceName"
	proxyConfig.Debug = true
	wg := sync.WaitGroup{}
	wg.Add(1)
	transport := &ConsulTwoMetricRoundTripper{
		t:  t,
		wg: &wg,
	}
	server, _ := NewProxyFromConfig(logrus.New(), proxyConfig)

	server.HTTPClient.Transport = transport
	defer server.Shutdown()

	server.Start()
	srv := httptest.NewServer(server.Handler())
	defer srv.Close()

	// Make sure we're sane first
	assert.Len(t, server.ForwardDestinations.Members(), 2, "Incorrect host count in ring")

	// Cool, now let's make a veneur to process some bits!
	config := localConfig()
	config.ForwardAddress = srv.URL
	f := newFixture(t, config, nil, nil)
	defer f.Close()

	f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Value:      float64(100),
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	})
	f.server.Workers[0].ProcessMetric(&samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "y.b.c",
			Type: "histogram",
		},
		Value:      float64(100),
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	})

	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()

	select {
	case <-c:
		fmt.Println("GOT 'IM")
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Failed to receive all metrics before timeout")
	}
}

func TestTimeout(t *testing.T) {
	defer log.SetLevel(log.Level)
	log.SetLevel(logrus.ErrorLevel)

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
	cfg.ForwardTimeout = "1ns" // just really really short
	server, _ := NewProxyFromConfig(logrus.New(), cfg)

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
	p, err := NewProxyFromConfig(logrus.New(), generateProxyConfig())
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
	p, err := NewProxyFromConfig(logrus.New(), generateProxyConfig())
	assert.NoError(t, err, "Creating a Proxy shouldn't have caused an error")

	done := make(chan struct{})
	go func() {
		p.Serve()
		defer p.Shutdown()
		close(done)
	}()

	// Stop the HTTP server only, causing Serve to exit.
	waitForHTTPStart(t, &p, 3*time.Second)
	graceful.ShutdownNow()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		assert.Fail(t, "Stopping the Proxy over HTTP did not stop both listeners")
	}
}
