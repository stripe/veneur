package veneur

import (
	"compress/zlib"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

type ConsulTwoRoundTripper struct {
	t                 *testing.T
	TwelveGotMetric   bool
	ThirteenGotMetric bool
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
		// fmt.Println("12 !!!")
		// fmt.Println(string(body))
		defer req.Body.Close()
		if strings.Contains(string(body), "a.b.c") {
			rt.TwelveGotMetric = true
		}
		rec.Code = http.StatusOK
	} else if req.URL.Path == "/import" && req.Host == "10.1.10.13:8000" {
		z, _ := zlib.NewReader(req.Body)
		body, _ := ioutil.ReadAll(z)
		// fmt.Println("13")
		// fmt.Println(string(body))
		defer req.Body.Close()
		if strings.Contains(string(body), "x.b.c") {
			rt.ThirteenGotMetric = true
		}
		rec.Code = http.StatusOK
	} else {
		assert.Fail(rt.t, "Received an unexpected request: %s %s", req.Host, req.URL.Path)
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
	transport := &ConsulTwoRoundTripper{
		t: t,
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
	assert.True(t, transport.TwelveGotMetric, "Host twelve got metrics!")
	assert.True(t, transport.ThirteenGotMetric, "Host thirteen got metrics!")
}
