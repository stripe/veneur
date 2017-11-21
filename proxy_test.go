package veneur

import (
	"compress/zlib"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

// On the CI server, we can't be guaranteed that the port will be
// released immediately after the server is shut down. Instead, use
// a unique port for each test. As long as we don't have an insane number
// of integration tests, we should be fine.
var ProxyHTTPAddrPort = 8229

func generateProxyConfig() ProxyConfig {
	port := ProxyHTTPAddrPort
	ProxyHTTPAddrPort++

	return ProxyConfig{
		Debug: false,
		ConsulRefreshInterval:    "86400s",
		ConsulForwardServiceName: "forwardServiceName",
		ConsulTraceServiceName:   "traceServiceName",
		TraceAddress:             "127.0.0.1:8128",
		TraceAPIAddress:          "127.0.0.1:8135",
		HTTPAddress:              fmt.Sprintf("127.0.0.1:%d", port),
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
		resp, _ := ioutil.ReadFile("fixtures/consul/health_service_two.json")
		rec.Write(resp)
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/v1/health/service/traceServiceName" {
		resp, _ := ioutil.ReadFile("fixtures/consul/health_service_two.json")
		rec.Write(resp)
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/api/v1/series" {
		// Just make the datadog bit work
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/import" && req.Host == "10.1.10.12:8000" {
		z, _ := zlib.NewReader(req.Body)
		body, _ := ioutil.ReadAll(z)
		defer req.Body.Close()
		if strings.Contains(string(body), "a.b.c") {
			rt.aReceived = true
		}
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/import" && req.Host == "10.1.10.13:8000" {
		z, _ := zlib.NewReader(req.Body)
		body, _ := ioutil.ReadAll(z)
		defer req.Body.Close()
		if strings.Contains(string(body), "x.b.c") {
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

	server, error := NewProxyFromConfig(proxyConfig)
	assert.NoError(t, error, "Should start with just static services")
	assert.False(t, server.usingConsul, "Server isn't using consul")
}

func TestMissingServices(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ForwardAddress = ""
	proxyConfig.TraceAddress = ""
	proxyConfig.ConsulForwardServiceName = ""
	proxyConfig.ConsulTraceServiceName = ""

	_, error := NewProxyFromConfig(proxyConfig)
	assert.Error(t, error, "No consul services means Proxy won't start")
}

func TestAcceptingBooleans(t *testing.T) {
	proxyConfig := generateProxyConfig()
	proxyConfig.ConsulTraceServiceName = ""
	proxyConfig.TraceAddress = ""

	server, _ := NewProxyFromConfig(proxyConfig)
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
	server, _ := NewProxyFromConfig(proxyConfig)

	server.HTTPClient.Transport = transport
	defer server.Shutdown()

	server.Start()
	go server.HTTPServe()

	// Make sure we're sane first
	assert.Len(t, server.ForwardDestinations.Members(), 2, "Incorrect host count in ring")

	// Cool, now let's make a veneur to process some bits!
	config := localConfig()
	config.ForwardAddress = fmt.Sprintf("http://%s", proxyConfig.HTTPAddress)
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
			Name: "x.b.c",
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
