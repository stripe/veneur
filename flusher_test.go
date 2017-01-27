package veneur

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

func TestServerTags(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("somehostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "somehostname", metrics[0].Hostname, "Metric hostname uses argument")
	assert.Contains(t, metrics[0].Tags, "a:b", "Tags should contain server tags")
}

func TestHostMagicTag(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "host:abc123", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("badhostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "abc123", metrics[0].Hostname, "Metric hostname should be from tag")
	assert.NotContains(t, metrics[0].Tags, "host:abc123", "Host tag should be removed")
	assert.Contains(t, metrics[0].Tags, "x:e", "Last tag is still around")
}

func TestDeviceMagicTag(t *testing.T) {
	metrics := []samplers.DDMetric{{
		Name:       "foo.bar.baz",
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(1.0)}},
		Tags:       []string{"gorch:frobble", "device:abc123", "x:e"},
		MetricType: "rate",
		Interval:   10,
	}}

	finalizeMetrics("badhostname", []string{"a:b", "c:d"}, metrics)
	assert.Equal(t, "abc123", metrics[0].DeviceName, "Metric devicename should be from tag")
	assert.NotContains(t, metrics[0].Tags, "device:abc123", "Host tag should be removed")
	assert.Contains(t, metrics[0].Tags, "x:e", "Last tag is still around")
}

func TestFlushTraces(t *testing.T) {
	type TestCase struct {
		Name         string
		ProtobufFile string
		JSONFile     string
	}

	cases := []TestCase{
		{
			Name:         "Success",
			ProtobufFile: filepath.Join("fixtures", "protobuf", "trace.pb"),
			JSONFile:     filepath.Join("fixtures", "tracing_agent", "spans", "trace.pb.json"),
		},
		{
			Name:         "Critical",
			ProtobufFile: filepath.Join("fixtures", "protobuf", "trace_critical.pb"),
			JSONFile:     filepath.Join("fixtures", "tracing_agent", "spans", "trace_critical.pb.json"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			pb, err := os.Open(tc.ProtobufFile)
			assert.NoError(t, err)
			defer pb.Close()

			js, err := os.Open(tc.JSONFile)
			assert.NoError(t, err)
			defer js.Close()

			testFlushTrace(t, pb, js)
		})
	}
}

func testFlushTrace(t *testing.T, protobuf, jsn io.Reader) {
	remoteResponseChan := make(chan struct{}, 1)
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var expected []*DatadogTraceSpan
		err := json.NewDecoder(jsn).Decode(&expected)
		assert.NoError(t, err)

		var actual []*DatadogTraceSpan
		err = json.NewDecoder(r.Body).Decode(&actual)
		assert.NoError(t, err)

		assert.Equal(t, expected, actual)

		w.WriteHeader(http.StatusAccepted)

		remoteResponseChan <- struct{}{}
	}))
	defer remoteServer.Close()

	config := globalConfig()
	config.TraceAPIAddress = remoteServer.URL

	server := setupVeneurServer(t, config, nil)
	defer server.Shutdown()

	assert.Equal(t, server.DDTraceAddress, config.TraceAPIAddress)

	packet, err := ioutil.ReadAll(protobuf)
	assert.NoError(t, err)

	server.HandleTracePacket(packet)

	assert.NoError(t, err)
	server.Flush()

	// wait for remoteServer to process the POST
	select {
	case <-remoteResponseChan:
		// all is safe
		break
	case <-time.After(10 * time.Second):
		assert.Fail(t, "Global server did not complete all responses before test terminated!")
	}
}
