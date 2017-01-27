package veneur

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type ConsulOneRoundTripper struct {
	HealthGotCalled bool
}

func (rt *ConsulOneRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if req.URL.Path == "/v1/health/service/veneur" {
		resp, _ := ioutil.ReadFile("fixtures/consul/health_service_one.json")
		rec.Write(resp)
		rec.Code = http.StatusOK
		rt.HealthGotCalled = true
	}

	return rec.Result(), nil
}

type ConsulChangingRoundTripper struct {
	Count           int
	HealthGotCalled bool
}

func (rt *ConsulChangingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if req.URL.Path == "/v1/health/service/veneur" {
		var resp []byte
		if rt.Count == 2 {
			// On the second invocation, return zero hosts!
			resp, _ = ioutil.ReadFile("fixtures/consul/health_service_zero.json")
		} else if rt.Count == 1 {
			// On the second invocation, return two hosts!
			resp, _ = ioutil.ReadFile("fixtures/consul/health_service_two.json")
		} else {
			resp, _ = ioutil.ReadFile("fixtures/consul/health_service_one.json")
		}
		rec.Write(resp)
		rec.Code = http.StatusOK
		rt.HealthGotCalled = true
		rt.Count++
	}

	return rec.Result(), nil
}

func TestConsulOneHost(t *testing.T) {
	config := localConfig()
	config.ForwardAddress = ""
	config.ConsulForwardServiceName = "veneur"
	config.ConsulRefreshInterval = "86400s" // A long time, so we can control it
	transport := &ConsulOneRoundTripper{}
	server := setupVeneurServer(t, config, transport)

	defer server.Shutdown()

	assert.True(t, transport.HealthGotCalled, "Health Service got called")
	assert.Equal(t, "http://10.1.10.12:8000", server.ForwardDestinations.Members()[0], "Get ring member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 1, "Only one host in ring")
}

func TestConsulChangingHosts(t *testing.T) {
	config := localConfig()
	config.ForwardAddress = ""
	config.ConsulForwardServiceName = "veneur"
	config.ConsulRefreshInterval = "86400s" // A long time, so we can control it
	transport := &ConsulChangingRoundTripper{}
	server := setupVeneurServer(t, config, transport)

	defer server.Shutdown()

	// First invocation is during startup
	assert.Equal(t, 1, transport.Count, "Health got called once")
	assert.Equal(t, "http://10.1.10.12:8000", server.ForwardDestinations.Members()[0], "Get ring member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 1, "Only one host in ring")

	// Refresh! Should have two now
	server.RefreshDestinations(config.ConsulForwardServiceName, server.ForwardDestinations, &server.ForwardDestinationsMtx)
	assert.Equal(t, 2, transport.Count, "Health got called second time")
	assert.Contains(t, server.ForwardDestinations.Members(), "http://10.1.10.12:8000", "Got first member from Consul")
	assert.Contains(t, server.ForwardDestinations.Members(), "http://10.1.10.13:8000", "Got second member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 2, "Two hosts host in ring")

	// Refresh! Now just none!
	server.RefreshDestinations(config.ConsulForwardServiceName, server.ForwardDestinations, &server.ForwardDestinationsMtx)
	assert.Equal(t, 3, transport.Count, "Health got called a third time.")
	assert.Contains(t, server.ForwardDestinations.Members(), "http://10.1.10.12:8000", "Got first member from Consul")
	assert.Contains(t, server.ForwardDestinations.Members(), "http://10.1.10.13:8000", "Got second member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 2, "One host host in ring")

	// Refresh! Now just one!
	server.RefreshDestinations(config.ConsulForwardServiceName, server.ForwardDestinations, &server.ForwardDestinationsMtx)
	assert.Equal(t, 4, transport.Count, "Health got called fourth time")
	assert.Contains(t, server.ForwardDestinations.Members(), "http://10.1.10.12:8000", "Got first member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 1, "One host host in ring")
}
