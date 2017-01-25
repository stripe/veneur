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

type ConsulTwoRoundTripper struct {
	t         *testing.T
	wg        *sync.WaitGroup
	aReceived bool
	bReceived bool
}

func (rt *ConsulTwoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if req.URL.Path == "/v1/health/service/veneur" {
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

func TestConsistentForward(t *testing.T) {
	config := localConfig()
	config.ForwardAddress = ""
	config.ConsulForwardServiceName = "veneur"
	config.ConsulRefreshInterval = "86400s" // A long time, so we can control it
	config.Interval = "86400s"
	config.Debug = true
	wg := sync.WaitGroup{}
	wg.Add(1)
	transport := &ConsulTwoRoundTripper{
		t:  t,
		wg: &wg,
	}
	server := setupVeneurServer(t, config, transport)

	server.HTTPClient.Transport = transport
	defer server.Shutdown()

	// Make sure we're sane first
	assert.Len(t, server.ForwardDestinations.Members(), 2, "Only one host in ring")

	server.Workers[0].ProcessMetric(&samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Value:      float64(100),
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	})
	server.Workers[0].ProcessMetric(&samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "x.b.c",
			Type: "histogram",
		},
		Value:      float64(100),
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.MixedScope,
	})

	server.Flush(10*time.Second, 100)

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
