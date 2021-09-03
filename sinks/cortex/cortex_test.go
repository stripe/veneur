package cortex

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
)

func TestName(t *testing.T) {
	// Implicitly test that CortexMetricsSink implements MetricSink
	var sink sinks.MetricSink
	sink, err := NewCortexMetricSink("https://localhost/")

	assert.NoError(t, err)
	assert.Equal(t, "cortex", sink.Name())
}

func TestFlush(t *testing.T) {
	// Listen for prometheus writes
	server := NewTestServer(t)
	defer server.Close()

	// Set up a sink
	sink, err := NewCortexMetricSink(server.URL)
	assert.NoError(t, err)

	// input.json contains three timeseries samples in InterMetrics format
	jsInput, err := os.ReadFile("testdata/input.json")
	assert.NoError(t, err)
	var metrics []samplers.InterMetric
	assert.NoError(t, json.Unmarshal(jsInput, &metrics))

	// Perform the flush to the test server
	assert.NoError(t, sink.Flush(context.Background(), metrics))

	// Retrieve the data which the server received
	data, err := server.Latest()
	assert.NoError(t, err)

	// Pretty-print output for readability, and to match expected
	actual, err := json.MarshalIndent(data, "", "  ")
	assert.NoError(t, err)

	//  Load in the expected data and compare
	expected, err := os.ReadFile("testdata/expected.json")
	assert.NoError(t, err)
	assert.Equal(t, string(expected), string(actual))

}

// TestServer wraps an internal httptest.Server and provides a convenience
// method for retrieving the most recently written series
type TestServer struct {
	URL    string
	data   *prompb.WriteRequest
	server *httptest.Server
}

// Close closes the internal test server
func (t *TestServer) Close() {
	t.server.Close()
}

// Latest returns the most recent write request, or errors if there was none
func (t *TestServer) Latest() (*prompb.WriteRequest, error) {
	if t.data == nil {
		return nil, errors.New("no data received")
	}
	return t.data, nil
}

// NewTestServer starts a test server instance. Ensure calls are followed by
// defer server.Close()
// to avoid hanging connections
func NewTestServer(t *testing.T) *TestServer {
	result := TestServer{}

	router := http.NewServeMux()
	router.HandleFunc("/receive", func(w http.ResponseWriter, r *http.Request) {
		wr, err := readpb(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// keep a record of the most recently received request
		result.data = wr
	})

	server := httptest.NewServer(router)
	result.URL = server.URL + "/receive"
	result.server = server
	t.Log("test server listening on", server.URL)

	return &result
}

// readpb reads, decompresses and unmarshals a WriteRequest from a reader
func readpb(r io.Reader) (*prompb.WriteRequest, error) {
	cdata, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	data, err := snappy.Decode(nil, cdata)
	if err != nil {
		return nil, err
	}

	var wr prompb.WriteRequest
	if err := proto.Unmarshal(data, &wr); err != nil {
		return nil, err
	}

	return &wr, nil
}