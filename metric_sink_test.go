package veneur

import (
	"net/http"
	"testing"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
)

func TestNewDatadogMetricSinkConfig(t *testing.T) {
	// test the variables that have been renamed
	config := Config{
		DatadogAPIKey:          "apikey",
		DatadogAPIHostname:     "http://api",
		DatadogTraceAPIAddress: "http://trace",
		SsfAddress:             "127.0.0.1:99",

		// required or NewFromConfig fails
		Interval:     "10s",
		StatsAddress: "localhost:62251",
	}
	stats, _ := statsd.NewBuffered(config.StatsAddress, 1024)
	ddSink, err := NewDatadogMetricSink(&config, float64(10.0), &http.Client{}, stats)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "apikey", ddSink.apiKey)
	assert.Equal(t, "http://api", ddSink.ddHostname)
}
