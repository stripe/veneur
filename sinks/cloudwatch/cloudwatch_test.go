package cloudwatch

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
)

// TestServer wraps an internal httptest.Server and provides a convenience
// method for retrieving the most recently written series
type TestServer struct {
	URL             string
	data            []byte
	server          *httptest.Server
	numRequestsSeen int
}

// Close closes the internal test server
func (t *TestServer) Close() {
	t.server.Close()
}

// NewTestServer starts a test server instance. Ensure calls are followed by
// defer server.Close() to avoid hanging connections
func NewTestServer(t *testing.T, handlerDelay time.Duration) *TestServer {
	result := TestServer{}

	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(handlerDelay)

		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error("empty request body")
		}
		result.data = b
		result.numRequestsSeen += 1
	})

	server := httptest.NewServer(router)
	result.URL = server.URL + "/"
	result.server = server
	t.Log("test server listening on", server.URL)

	return &result
}

// Latest returns the most recent write request, or errors if there was none
func (t *TestServer) Latest() ([]byte, error) {
	if t.data == nil {
		return nil, errors.New("no data received")
	}
	return t.data, nil
}

func TestName(t *testing.T) {
	// Implicitly test that cloudwatchMetricsSink implements MetricSink
	var sink sinks.MetricSink
	sink = NewCloudwatchMetricSink("http://localhost/", "test", "us-east-1000", "cloudwatch_standard_unit", time.Second*30, logrus.NewEntry(logrus.New()))

	assert.Equal(t, "cloudwatch", sink.Name())
}

func TestFlush(t *testing.T) {
	// Listen for PutMetricData
	server := NewTestServer(t, 0)
	defer server.Close()

	// input.1.json contains three timeseries samples in InterMetrics format
	jsInput, err := ioutil.ReadFile("testdata/input.1.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Call PutMetricData
	sink := NewCloudwatchMetricSink(server.URL, "test", "us-east-1000", "cloudwatch_standard_unit", time.Second*30, logrus.NewEntry(logrus.New()))
	sink.Start(nil)
	err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	// Inspect data that was flushed
	expectedOutput, err := ioutil.ReadFile("testdata/output.1.txt")
	assert.NoError(t, err)
	assert.Equal(t, string(expectedOutput), string(server.data))
}

func TestFlushWithStandardUnitTagName(t *testing.T) {
	// Listen for PutMetricData
	server := NewTestServer(t, 0)
	defer server.Close()

	// input.2.json contains one timeseries sample in InterMetrics format, with a unit specified
	jsInput, err := ioutil.ReadFile("testdata/input.2.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Call PutMetricData
	sink := NewCloudwatchMetricSink(server.URL, "test", "us-east-1000", "cloudwatch_standard_unit", time.Second*30, logrus.NewEntry(logrus.New()))
	sink.Start(nil)
	err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	// Inspect data that was flushed, which should have a standard unit and no dimensions
	expectedOutput, err := ioutil.ReadFile("testdata/output.2.txt")
	assert.NoError(t, err)
	assert.Equal(t, string(expectedOutput), string(server.data))
}

func TestFlushNoop(t *testing.T) {
	// Listen for PutMetricData
	server := NewTestServer(t, 0)
	defer server.Close()

	// Pass empty metrics slice
	var metrics []samplers.InterMetric

	// Call PutMetricData
	sink := NewCloudwatchMetricSink(server.URL, "test", "us-east-1000", "cloudwatch_standard_unit", time.Second*30, logrus.NewEntry(logrus.New()))
	sink.Start(nil)
	err := sink.Flush(context.Background(), metrics)

	// Assert that the server was never hit
	assert.NoError(t, err)
	assert.Equal(t, 0, server.numRequestsSeen)
}

func TestFlushRemoteTimeout(t *testing.T) {
	// Server handler should return response after the flush timeout expires
	customTimeout := time.Second * 1
	serverDelay := customTimeout + time.Second

	// Listen for PutMetricData
	server := NewTestServer(t, serverDelay)
	defer server.Close()

	// Pass non-empty metrics slice
	metrics := []samplers.InterMetric{{}}

	// Call PutMetricData
	sink := NewCloudwatchMetricSink(server.URL, "test", "us-east-1000", "cloudwatch_standard_unit", customTimeout, logrus.NewEntry(logrus.New()))
	sink.Start(nil)
	err := sink.Flush(context.Background(), metrics)

	// Assert the Flush failed
	assert.Error(t, err)
}
