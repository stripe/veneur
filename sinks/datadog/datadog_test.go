package datadog

import (
	"compress/zlib"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/protocol/dogstatsd"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
)

// DDMetricsRequest represents the body of the POST request
// for sending metrics data to Datadog
// Eventually we'll want to define this symmetrically.
type DDMetricsRequest struct {
	Series []DDMetric
}

type DDEventRequest struct {
	Events struct {
		Api []DDEvent
	}
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
	ddMetrics, serviceChecks := ddSink.finalizeMetrics(metrics)
	assert.Empty(t, serviceChecks, "No service check metrics are reported")
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

	ddMetrics, serviceChecks := ddSink.finalizeMetrics(metrics)
	assert.Empty(t, serviceChecks, "No service check metrics are reported")
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

	ddMetrics, serviceChecks := ddSink.finalizeMetrics(metrics)
	assert.Empty(t, serviceChecks, "No service check metrics are reported")
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

	ddMetrics, serviceChecks := ddSink.finalizeMetrics(metrics)
	assert.Empty(t, serviceChecks, "No service check metrics are reported")
	assert.Equal(t, "abc123", ddMetrics[0].DeviceName, "Metric devicename should be from tag")
	assert.NotContains(t, ddMetrics[0].Tags, "device:abc123", "Host tag should be removed")
	assert.Contains(t, ddMetrics[0].Tags, "x:e", "Last tag is still around")
}

func TestNewDatadogSpanSinkConfig(t *testing.T) {
	// test the variables that have been renamed
	ddSink, err := NewDatadogSpanSink("http://example.com", 100, &http.Client{}, logrus.New())
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
	Contents      string
}

func (rt *DatadogRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	if strings.HasPrefix(req.URL.Path, rt.Endpoint) {
		bstream := req.Body
		if req.Header.Get("Content-Encoding") == "deflate" {
			bstream, _ = zlib.NewReader(req.Body)
		}
		body, _ := ioutil.ReadAll(bstream)
		defer bstream.Close()
		if rt.Contains != "" {
			if strings.Contains(string(body), rt.Contains) {
				rt.ThingReceived = true
			}
		}
		rt.Contents = string(body)

		rec.Code = http.StatusOK
		rt.GotCalled = true
	}

	return rec.Result(), nil
}

func TestDatadogFlushSpans(t *testing.T) {
	// test the variables that have been renamed

	transport := &DatadogRoundTripper{Endpoint: "/v0.3/traces", Contains: "farts-srv"}
	ddSink, err := NewDatadogSpanSink("http://example.com", 100, &http.Client{Transport: transport}, logrus.New())
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

	ddSink.Flush(context.TODO())
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

func TestDatadogFlushEvents(t *testing.T) {
	transport := &DatadogRoundTripper{Endpoint: "/intake", Contains: ""}
	ddSink, err := NewDatadogMetricSink(10, 2500, "example.com", []string{"gloobles:toots"}, "http://example.com", "secret", &http.Client{Transport: transport}, logrus.New())
	assert.NoError(t, err)

	testEvent := ssf.SSFSample{
		Name:      "foo",
		Message:   "bar",
		Timestamp: 1136239445,
		Tags: map[string]string{
			dogstatsd.EventIdentifierKey:        "",
			dogstatsd.EventAggregationKeyTagKey: "foos",
			dogstatsd.EventSourceTypeTagKey:     "test",
			dogstatsd.EventAlertTypeTagKey:      "success",
			dogstatsd.EventPriorityTagKey:       "low",
			dogstatsd.EventHostnameTagKey:       "example.com",
			"foo": "bar",
			"baz": "qux",
		},
	}
	ddFixtureEvent := DDEvent{
		Title:       testEvent.Name,
		Text:        testEvent.Message,
		Timestamp:   testEvent.Timestamp,
		Hostname:    testEvent.Tags[dogstatsd.EventHostnameTagKey],
		Aggregation: testEvent.Tags[dogstatsd.EventAggregationKeyTagKey],
		Source:      testEvent.Tags[dogstatsd.EventSourceTypeTagKey],
		Priority:    testEvent.Tags[dogstatsd.EventPriorityTagKey],
		AlertType:   testEvent.Tags[dogstatsd.EventAlertTypeTagKey],
		Tags: []string{
			"foo:bar",
			"baz:qux",
			"gloobles:toots", // This one needs to be here because of the Sink's common tags!
		},
	}

	ddSink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{testEvent})
	assert.NoError(t, err)

	assert.Equal(t, true, transport.GotCalled, "Did not call endpoint")
	ddEvents := DDEventRequest{}
	jsonErr := json.Unmarshal([]byte(transport.Contents), &ddEvents)
	assert.NoError(t, jsonErr)
	event := ddEvents.Events.Api[0]

	assert.Subset(t, ddFixtureEvent.Tags, event.Tags, "Event tags do not match")
	assert.Equal(t, ddFixtureEvent.Aggregation, event.Aggregation, "Event aggregation doesn't match")
	assert.Equal(t, ddFixtureEvent.AlertType, event.AlertType, "Event alert type doesn't match")
	assert.Equal(t, ddFixtureEvent.Hostname, event.Hostname, "Event hostname doesn't match")
	assert.Equal(t, ddFixtureEvent.Priority, event.Priority, "Event priority doesn't match")
	assert.Equal(t, ddFixtureEvent.Source, event.Source, "Event source doesn't match")
	assert.Equal(t, ddFixtureEvent.Text, event.Text, "Event text doesn't match")
	assert.Equal(t, ddFixtureEvent.Timestamp, event.Timestamp, "Event timestamp doesn't match")
	assert.Equal(t, ddFixtureEvent.Title, event.Title, "Event title doesn't match")
}

func TestDatadogFlushOtherMetricsForServiceChecks(t *testing.T) {
	transport := &DatadogRoundTripper{Endpoint: "/api/v1/check_run", Contains: ""}
	ddSink, err := NewDatadogMetricSink(10, 2500, "example.com", []string{"gloobles:toots"}, "http://example.com", "secret", &http.Client{Transport: transport}, logrus.New())
	assert.NoError(t, err)

	testCheck := ssf.SSFSample{
		Name:      "foo",
		Message:   "bar",
		Status:    ssf.SSFSample_WARNING, // Notably setting this to something that isn't the default value to ensure it works
		Timestamp: 1136239445,
		Tags: map[string]string{
			"host": "example.com",
			"foo":  "bar",
			"baz":  "qux",
		},
	}

	ddSink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{testCheck})
	assert.NoError(t, err)

	assert.Equal(t, false, transport.GotCalled, "Was not supposed to log a service check in the FlushOtherSamples")
}

func TestDatadogFlushServiceCheck(t *testing.T) {
	transport := &DatadogRoundTripper{Endpoint: "/api/v1/check_run", Contains: ""}
	ddSink, err := NewDatadogMetricSink(10, 2500, "example.com", []string{"gloobles:toots"}, "http://example.com", "secret", &http.Client{Transport: transport}, logrus.New())
	assert.NoError(t, err)

	testCheck := samplers.InterMetric{
		Name:      "foo",
		Type:      samplers.StatusMetric,
		Message:   "bar",
		Timestamp: 1136239445,
		Value:     float64(ssf.SSFSample_OK),
		Tags: []string{
			"foo:bar",
			"baz:qux",
		},
	}

	ddSink.Flush(context.TODO(), []samplers.InterMetric{testCheck})
	assert.NoError(t, err)

	assert.Equal(t, true, transport.GotCalled, "Should have called the datadog transport")

	ddFixtureCheck := DDServiceCheck{
		Name:      testCheck.Name,
		Status:    int(testCheck.Value),
		Hostname:  "example.com",
		Timestamp: testCheck.Timestamp,
		Message:   testCheck.Message,
		Tags: []string{
			"foo:bar",
			"baz:qux",
			"gloobles:toots", // This one needs to be here because of the Sink's common tags!
		},
	}
	ddChecks := []DDServiceCheck{}
	jsonErr := json.Unmarshal([]byte(transport.Contents), &ddChecks)
	assert.NoError(t, jsonErr)

	assert.Equal(t, ddFixtureCheck.Name, ddChecks[0].Name, "Check name doesn't match")
	assert.Equal(t, ddFixtureCheck.Hostname, ddChecks[0].Hostname, "Check hostname doesn't match")
	assert.Equal(t, ddFixtureCheck.Message, ddChecks[0].Message, "Check message doesn't match")
	assert.Equal(t, ddFixtureCheck.Status, ddChecks[0].Status, "Check status doesn't match")
	assert.Equal(t, ddFixtureCheck.Timestamp, ddChecks[0].Timestamp, "Check timestamp doesn't match")
	assert.Subset(t, ddFixtureCheck.Tags, ddChecks[0].Tags, "Check posted to DD does not have matching tags")

}
