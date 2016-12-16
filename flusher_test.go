package veneur

import (
	"encoding/json"
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

	RemoteResponseChan := make(chan struct{}, 1)
	defer func() {
		select {
		case <-RemoteResponseChan:
			// all is safe
			return
		case <-time.After(10 * time.Second):
			assert.Fail(t, "Global server did not complete all responses before test terminated!")
		}
	}()

	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, err := os.Open(filepath.Join("fixtures", "tracing_agent", "spans", "trace.pb.json"))
		assert.NoError(t, err)

		var expected []*DatadogTraceSpan
		err = json.NewDecoder(f).Decode(&expected)
		assert.NoError(t, err)

		var actual []*DatadogTraceSpan
		err = json.NewDecoder(r.Body).Decode(&actual)
		assert.NoError(t, err)

		assert.Equal(t, expected, actual)

		w.WriteHeader(http.StatusAccepted)

		RemoteResponseChan <- struct{}{}
	}))

	config := globalConfig()
	config.TraceAPIAddress = remoteServer.URL

	server := setupVeneurServer(t, config)
	defer server.Shutdown()

	assert.Equal(t, server.DDTraceAddress, config.TraceAPIAddress)

	packet, err := ioutil.ReadFile(filepath.Join("fixtures", "protobuf", "trace.pb"))
	assert.NoError(t, err)

	server.HandleTracePacket(packet)

	interval, err := config.ParseInterval()
	assert.NoError(t, err)
	server.Flush(interval, config.FlushMaxPerBody)
}
