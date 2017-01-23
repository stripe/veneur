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

func TestConsulOneHost(t *testing.T) {
	config := localConfig()
	config.ForwardAddress = ""
	config.ConsulForwardServiceName = "veneur"
	config.ConsulRefreshInterval = "60s"
	transport := &ConsulOneRoundTripper{}
	server := setupVeneurServer(t, config, transport)

	server.HTTPClient.Transport = transport
	defer server.Shutdown()

	assert.True(t, transport.HealthGotCalled, "Health Service got called")
	assert.Equal(t, "http://10.1.10.12:8000", server.ForwardDestinations.Members()[0], "Get ring member from Consul")
	assert.Len(t, server.ForwardDestinations.Members(), 1, "Only one host in ring")
}
