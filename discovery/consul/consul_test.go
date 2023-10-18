package consul_test

import (
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/discovery/consul"
)

type ConsulRoundTripper struct {
	HealthGotCalled bool
	Response        string
}

func (roundTripper *ConsulRoundTripper) RoundTrip(
	request *http.Request,
) (*http.Response, error) {
	if request.URL.Path != "/v1/health/service/service-name" {
		return nil, errors.New("invalid request path")
	}

	response, err := ioutil.ReadFile(roundTripper.Response)
	if err != nil {
		return nil, err
	}

	recorder := httptest.NewRecorder()
	recorder.Write(response)
	recorder.Code = http.StatusOK
	roundTripper.HealthGotCalled = true

	return recorder.Result(), nil
}

func TestConsulZeroHosts(t *testing.T) {
	roundTripper := &ConsulRoundTripper{
		Response: "testdata/health_service_zero.json",
	}
	config := api.DefaultConfig()
	config.HttpClient = &http.Client{
		Transport: roundTripper,
	}
	client, err := consul.NewConsul(config)
	assert.NoError(t, err)

	_, err = client.GetDestinationsForService("service-name")
	if assert.Error(t, err) {
		assert.Equal(t, "received no hosts from Consul", err.Error())
	}
}

func TestConsulOneHost(t *testing.T) {
	roundTripper := &ConsulRoundTripper{
		Response: "testdata/health_service_one.json",
	}
	config := api.DefaultConfig()
	config.HttpClient = &http.Client{
		Transport: roundTripper,
	}
	client, err := consul.NewConsul(config)
	assert.NoError(t, err)

	destinations, err := client.GetDestinationsForService("service-name")
	assert.NoError(t, err)

	assert.True(t, roundTripper.HealthGotCalled)
	if assert.Len(t, destinations, 1) {
		assert.Equal(t, "10.1.10.12:8000", destinations[0])
	}
}

func TestConsulTwoHosts(t *testing.T) {
	roundTripper := &ConsulRoundTripper{
		Response: "testdata/health_service_two.json",
	}
	config := api.DefaultConfig()
	config.HttpClient = &http.Client{
		Transport: roundTripper,
	}
	client, err := consul.NewConsul(config)
	assert.NoError(t, err)

	destinations, err := client.GetDestinationsForService("service-name")
	assert.NoError(t, err)

	assert.True(t, roundTripper.HealthGotCalled)
	if assert.Len(t, destinations, 2) {
		assert.Equal(t, "10.1.10.12:8000", destinations[0])
		assert.Equal(t, "10.1.10.13:8000", destinations[1])
	}
}
