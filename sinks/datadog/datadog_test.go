package datadog

import (
	"compress/zlib"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

// DDMetricsRequest represents the body of the POST request
// for sending metrics data to Datadog
// Eventually we'll want to define this symmetrically.
type DDMetricsRequest struct {
	Series []DDMetric
}

// fixture sets up a mock Datadog API server and Veneur
type fixture struct {
	api             *httptest.Server
	server          *Server
	ddmetrics       chan DDMetricsRequest
	interval        time.Duration
	flushMaxPerBody int
}

func newFixture(t *testing.T, config Config) *fixture {
	interval, err := config.ParseInterval()
	assert.NoError(t, err)

	// Set up a remote server (the API that we're sending the data to)
	// (e.g. Datadog)
	f := &fixture{nil, &Server{}, make(chan DDMetricsRequest, 10), interval, config.FlushMaxPerBody}
	f.api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		zr, err := zlib.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}

		var ddmetrics DDMetricsRequest

		err = json.NewDecoder(zr).Decode(&ddmetrics)
		if err != nil {
			t.Fatal(err)
		}
		f.ddmetrics <- ddmetrics
		w.WriteHeader(http.StatusAccepted)
	}))

	config.DatadogAPIHostname = f.api.URL
	config.NumWorkers = 1
	f.server = setupVeneurServer(t, config, nil)
	return f
}

func (f *fixture) Close() {
	// make Close safe to call multiple times
	if f.ddmetrics == nil {
		return
	}

	f.api.Close()
	f.server.Shutdown()
	close(f.ddmetrics)
	f.ddmetrics = nil
}

// assertMetrics checks that all expected metrics are present
// and have the correct value
func assertMetrics(t *testing.T, metrics DDMetricsRequest, expectedMetrics map[string]float64) {
	// it doesn't count as accidentally quadratic if it's intentional
	for metricName, expectedValue := range expectedMetrics {
		assertMetric(t, metrics, metricName, expectedValue)
	}
}

func assertMetric(t *testing.T, metrics DDMetricsRequest, metricName string, value float64) {
	defer func() {
		if r := recover(); r != nil {
			assert.Fail(t, "error extracting metrics", r)
		}
	}()
	for _, metric := range metrics.Series {
		if metric.Name == metricName {
			assert.Equal(t, int(value+.5), int(metric.Value[0][1]+.5), "Incorrect value for metric %s", metricName)
			return
		}
	}
	assert.Fail(t, "did not find expected metric", metricName)
}

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

func TestDatadogRate(t *testing.T) {
	ddSink := datadogMetricSink{
		hostname: "somehostname",
		tags:     []string{"a:b", "c:d"},
		interval: 10,
	}

	metrics := []samplers.InterMetric{{
		Name:      "foo.bar.baz",
		Timestamp: time.Now().Unix(),
		Value:     float64(10),
		Tags:      []string{"gorch:frobble", "x:e"},
		Type:      samplers.CounterMetric,
	}}
	ddMetrics := ddSink.finalizeMetrics(metrics)
	assert.Equal(t, "rate", ddMetrics[0].MetricType, "Metric type should be rate")
	assert.Equal(t, float64(1.0), ddMetrics[0].Value[0][1], "Metric rate wasnt computed correctly")
}

func TestServerTags(t *testing.T) {
	ddSink := datadogMetricSink{
		hostname: "somehostname",
		tags:     []string{"a:b", "c:d"},
		interval: 10,
	}

	metrics := []samplers.InterMetric{{
		Name:      "foo.bar.baz",
		Timestamp: time.Now().Unix(),
		Value:     float64(10),
		Tags:      []string{"gorch:frobble", "x:e"},
		Type:      samplers.CounterMetric,
	}}

	ddMetrics := ddSink.finalizeMetrics(metrics)
	assert.Equal(t, "somehostname", ddMetrics[0].Hostname, "Metric hostname uses argument")
	assert.Contains(t, ddMetrics[0].Tags, "a:b", "Tags should contain server tags")
}

func TestHostMagicTag(t *testing.T) {
	ddSink := datadogMetricSink{
		hostname: "badhostname",
		tags:     []string{"a:b", "c:d"},
	}

	metrics := []samplers.InterMetric{{
		Name:      "foo.bar.baz",
		Timestamp: time.Now().Unix(),
		Value:     float64(10),
		Tags:      []string{"gorch:frobble", "host:abc123", "x:e"},
		Type:      samplers.CounterMetric,
	}}

	ddMetrics := ddSink.finalizeMetrics(metrics)
	assert.Equal(t, "abc123", ddMetrics[0].Hostname, "Metric hostname should be from tag")
	assert.NotContains(t, ddMetrics[0].Tags, "host:abc123", "Host tag should be removed")
	assert.Contains(t, ddMetrics[0].Tags, "x:e", "Last tag is still around")
}

func TestDeviceMagicTag(t *testing.T) {
	ddSink := datadogMetricSink{
		hostname: "badhostname",
		tags:     []string{"a:b", "c:d"},
	}

	metrics := []samplers.InterMetric{{
		Name:      "foo.bar.baz",
		Timestamp: time.Now().Unix(),
		Value:     float64(10),
		Tags:      []string{"gorch:frobble", "device:abc123", "x:e"},
		Type:      samplers.CounterMetric,
	}}

	ddMetrics := ddSink.finalizeMetrics(metrics)
	assert.Equal(t, "abc123", ddMetrics[0].DeviceName, "Metric devicename should be from tag")
	assert.NotContains(t, ddMetrics[0].Tags, "device:abc123", "Host tag should be removed")
	assert.Contains(t, ddMetrics[0].Tags, "x:e", "Last tag is still around")
}
