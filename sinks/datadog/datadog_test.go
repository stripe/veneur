package datadog

import (
	"compress/zlib"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

// DDMetricsRequest represents the body of the POST request
// for sending metrics data to Datadog
// Eventually we'll want to define this symmetrically.
type DDMetricsRequest struct {
	Series []DDMetric
}

func TestDatadogRate(t *testing.T) {
	ddSink := DatadogMetricSink{
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
	ddSink := DatadogMetricSink{
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
	ddSink := DatadogMetricSink{
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
	ddSink := DatadogMetricSink{
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

func TestNewDatadogSpanSinkConfig(t *testing.T) {
	// test the variables that have been renamed
	ddSink, err := NewDatadogSpanSink("http://example.com", 100, &http.Client{}, nil, logrus.New())
	if err != nil {
		t.Fatal(err)
	}
	err = ddSink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "http://example.com", ddSink.traceAddress)
}

type DatadogRoundTripper struct {
	Endpoint      string
	Contains      string
	GotCalled     bool
	ThingReceived bool
}

func (rt *DatadogRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if strings.HasPrefix(req.URL.Path, rt.Endpoint) {
		body, _ := ioutil.ReadAll(req.Body)
		defer req.Body.Close()
		if strings.Contains(string(body), rt.Contains) {
			rt.ThingReceived = true
		}

		rec.Code = http.StatusOK
		rt.GotCalled = true
	}

	return rec.Result(), nil
}

func TestDatadogFlushSpans(t *testing.T) {
	// test the variables that have been renamed

	transport := &DatadogRoundTripper{Endpoint: "/v0.3/traces", Contains: "farts-srv"}
	ddSink, err := NewDatadogSpanSink("http://example.com", 100, &http.Client{Transport: transport}, nil, logrus.New())
	assert.NoError(t, err)

	start := time.Now()
	end := start.Add(2 * time.Second)

	testSpan := &ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}
	err = ddSink.Ingest(testSpan)
	assert.NoError(t, err)

	ddSink.Flush()
	assert.Equal(t, true, transport.GotCalled, "Did not call spans endpoint")
}

type result struct {
	received  bool
	contained bool
}

func ddTestServer(t *testing.T, endpoint, contains string) (*httptest.Server, chan result) {
	received := make(chan result)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := result{}
		bstream := r.Body
		if r.Header.Get("Content-Encoding") == "deflate" {
			bstream, _ = zlib.NewReader(r.Body)
		}
		body, _ := ioutil.ReadAll(bstream)
		defer bstream.Close()
		if strings.HasPrefix(r.URL.Path, endpoint) {
			res.received = true
			res.contained = strings.Contains(string(body), contains)
		}
		w.WriteHeader(200)
		received <- res
	})
	return httptest.NewServer(handler), received
}

func TestDatadogMetricRouting(t *testing.T) {
	// test the variables that have been renamed
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

	tests := []struct {
		metric samplers.InterMetric
		expect bool
	}{
		{
			samplers.InterMetric{
				Name:      "to.anybody.in.particular",
				Timestamp: time.Now().Unix(),
				Value:     float64(10),
				Tags:      []string{"gorch:frobble", "x:e"},
				Type:      samplers.CounterMetric,
			},
			true,
		},
		{
			samplers.InterMetric{
				Name:      "to.datadog",
				Timestamp: time.Now().Unix(),
				Value:     float64(10),
				Tags:      []string{"gorch:frobble", "x:e"},
				Type:      samplers.CounterMetric,
				Sinks:     samplers.RouteInformation{"datadog": struct{}{}},
			},
			true,
		},
		{
			samplers.InterMetric{
				Name:      "to.kafka.only",
				Timestamp: time.Now().Unix(),
				Value:     float64(10),
				Tags:      []string{"gorch:frobble", "x:e"},
				Type:      samplers.CounterMetric,
				Sinks:     samplers.RouteInformation{"kafka": struct{}{}},
			},
			false,
		},
	}
	for _, elt := range tests {
		test := elt
		t.Run(test.metric.Name, func(t *testing.T) {
			t.Parallel()
			srv, rcved := ddTestServer(t, "/api/v1/series", test.metric.Name)
			ddSink := DatadogMetricSink{
				DDHostname:      srv.URL,
				HTTPClient:      client,
				flushMaxPerBody: 15,
				log:             logrus.New(),
				tags:            []string{"a:b", "c:d"},
				interval:        10,
			}
			done := make(chan struct{})
			go func() {
				result, ok := <-rcved
				if test.expect {
					// TODO: negative case
					assert.True(t, result.contained, "Should have sent the metric")
				} else {
					if ok {
						assert.False(t, result.contained, "Should definitely not have sent the metric!")
					}
				}
				close(done)
			}()
			err := ddSink.Flush(context.TODO(), []samplers.InterMetric{test.metric})
			require.NoError(t, err)
			close(rcved)
			<-done
		})
	}

}
