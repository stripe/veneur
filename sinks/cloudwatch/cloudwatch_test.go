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
)

// TestServer wraps an internal httptest.Server and provides a convenience
// method for retrieving the most recently written series
type TestServer struct {
	URL    string
	server *httptest.Server
}

// Close closes the internal test server
func (t *TestServer) Close() {
	t.server.Close()
}

// NewTestServer starts a test server instance. Ensure calls are followed by
// defer server.Close() to avoid hanging connections
func NewTestServer(t *testing.T, handlerDelay time.Duration, reqBodyCh chan []byte) *TestServer {
	result := TestServer{}

	router := http.NewServeMux()
	router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(handlerDelay)

		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error("empty request body")
		}
		reqBodyCh <- b
	})

	server := httptest.NewServer(router)
	result.URL = server.URL + "/"
	result.server = server
	t.Log("test server listening on", server.URL)

	return &result
}

// latest returns the most recent write request, or errors if there was none
func latest(reqBodyCh chan []byte, timeoutSecs int) ([]byte, error) {
	timeout := time.After(time.Duration(timeoutSecs) * time.Second)
	select {
	case data := <-reqBodyCh:
		return data, nil
	case <-timeout:
		return nil, errors.New("no data received")
	}
}

func TestName(t *testing.T) {
	sink := NewCloudwatchMetricSink(
		"http://localhost/", "test", "us-east-1000", "cloudwatch_standard_unit",
		time.Second*30, true, []string{}, logrus.NewEntry(logrus.New()),
	)
	assert.Equal(t, "cloudwatch", sink.Name())
}

func TestFlush(t *testing.T) {
	// Listen for PutMetricData
	reqBodyCh := make(chan []byte)
	server := NewTestServer(t, 0, reqBodyCh)
	defer server.Close()

	// input.1.json contains three timeseries samples in InterMetrics format
	jsInput, err := ioutil.ReadFile("testdata/input.1.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Initialize sink
	sink := NewCloudwatchMetricSink(
		server.URL, "test", "us-east-1000", "cloudwatch_standard_unit",
		time.Second*30, true, []string{}, logrus.NewEntry(logrus.New()),
	)
	sink.Start(nil)

	// Assert data is as we expect
	done := make(chan bool)
	go func(reqBodyCh chan []byte, done chan bool) {
		expectedOutput, err := ioutil.ReadFile("testdata/output.1.txt")
		assert.NoError(t, err)
		data, err := latest(reqBodyCh, 3)
		assert.NoError(t, err)
		assert.Equal(t, string(expectedOutput), string(data))
		done <- true
	}(reqBodyCh, done)

	// Flush the sink
	err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	<-done
}

func TestFlushWithStandardUnitTagName(t *testing.T) {
	// Listen for PutMetricData
	reqBodyCh := make(chan []byte)
	server := NewTestServer(t, 0, reqBodyCh)
	defer server.Close()

	// input.2.json contains one timeseries sample in InterMetrics format, with a unit specified
	jsInput, err := ioutil.ReadFile("testdata/input.2.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Initialize sink
	sink := NewCloudwatchMetricSink(
		server.URL, "test", "us-east-1000", "cloudwatch_standard_unit",
		time.Second*30, true, []string{}, logrus.NewEntry(logrus.New()),
	)
	sink.Start(nil)

	// Inspect data that was flushed, which should have a standard unit and no dimensions
	done := make(chan bool)
	go func(reqBodyCh chan []byte, done chan bool) {
		expectedOutput, err := ioutil.ReadFile("testdata/output.2.txt")
		assert.NoError(t, err)
		data, err := latest(reqBodyCh, 3)
		assert.NoError(t, err)
		assert.Equal(t, string(expectedOutput), string(data))
		done <- true
	}(reqBodyCh, done)

	// Flush the sink
	err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	<-done
}

func TestFlushWithStripTags(t *testing.T) {
	// Listen for PutMetricData
	reqBodyCh := make(chan []byte)
	server := NewTestServer(t, 0, reqBodyCh)
	defer server.Close()

	// input.2.json contains timeseries with tags to strip, and tags to keep
	jsInput, err := ioutil.ReadFile("testdata/input.3.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Initialize sink
	stripTags := []string{"baz"}
	sink := NewCloudwatchMetricSink(
		server.URL, "test", "us-east-1000", "cloudwatch_standard_unit",
		time.Second*30, true, stripTags, logrus.NewEntry(logrus.New()),
	)
	sink.Start(nil)

	// Inspect data that was flushed
	// - Datapoint (1, 3) should not have tags
	// - Datapoint (2) should have one tag
	done := make(chan bool)
	go func(reqBodyCh chan []byte, done chan bool) {
		expectedOutput, err := ioutil.ReadFile("testdata/output.3.txt")
		assert.NoError(t, err)
		data, err := latest(reqBodyCh, 3)
		assert.NoError(t, err)
		assert.Equal(t, string(expectedOutput), string(data))
		done <- true
	}(reqBodyCh, done)

	// Flush the sink
	err = sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	<-done
}

func TestFlushNoop(t *testing.T) {
	// Listen for PutMetricData
	reqBodyCh := make(chan []byte)
	server := NewTestServer(t, 0, reqBodyCh)
	defer server.Close()

	// Pass empty metrics slice
	var metrics []samplers.InterMetric

	// Initialize the sink
	sink := NewCloudwatchMetricSink(
		server.URL, "test", "us-east-1000", "cloudwatch_standard_unit",
		time.Second*30, true, []string{}, logrus.NewEntry(logrus.New()),
	)
	sink.Start(nil)

	// Assert the server was never hit
	done := make(chan bool)
	go func(reqBodyCh chan []byte, done chan bool) {
		_, err := latest(reqBodyCh, 3)
		assert.Error(t, err)
		done <- true
	}(reqBodyCh, done)

	// Flush the sink
	err := sink.Flush(context.Background(), metrics)
	assert.NoError(t, err)

	<-done
}

func TestFlushRemoteTimeout(t *testing.T) {
	// Server handler should return response after the flush timeout expires
	customTimeout := time.Second * 1
	serverDelay := customTimeout + time.Second

	// Listen for PutMetricData
	reqBodyCh := make(chan []byte)
	server := NewTestServer(t, serverDelay, reqBodyCh)
	defer server.Close()

	// Pass non-empty metrics slice
	metrics := []samplers.InterMetric{{}}

	// Initialize the sink
	sink := NewCloudwatchMetricSink(
		server.URL, "test", "us-east-1000", "cloudwatch_standard_unit",
		customTimeout, true, []string{}, logrus.NewEntry(logrus.New()),
	)
	sink.Start(nil)

	// Make sure that the server can Close, eventually
	done := make(chan bool)
	go func(reqBodyCh chan []byte, done chan bool) {
		<-reqBodyCh
		done <- true
	}(reqBodyCh, done)

	// Assert the flush failed
	err := sink.Flush(context.Background(), metrics)
	assert.Error(t, err)

	<-done
}
