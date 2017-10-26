package veneur

import (
	"net/http"
	"testing"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
)

func TestNewDatadogSpanSinkConfig(t *testing.T) {
	// test the variables that have been renamed
	config := Config{
		DatadogTraceAPIAddress: "http://trace",
	}
	stats, _ := statsd.NewBuffered(config.StatsAddress, 1024)
	ddSink, err := NewDatadogSpanSink(&config, stats, &http.Client{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = ddSink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "http://trace", ddSink.traceAddress)
}
