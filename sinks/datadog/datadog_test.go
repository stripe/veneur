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
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol/dogstatsd"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"gopkg.in/yaml.v2"
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
	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(
		&veneur.Server{HTTPClient: &http.Client{}}, "datadog", logger, veneur.Config{},
		DatadogSpanSinkConfig{
			SpanBufferSize:  100,
			TraceAPIAddress: "http://example.com",
		})
	if err != nil {
		t.Fatal(err)
	}
	err = sink.Start(nil)
	if err != nil {
		t.Fatal(err)
	}

	datadogSink := sink.(*DatadogSpanSink)
	assert.Equal(t, "http://example.com", datadogSink.traceAddress)
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
	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateSpanSink(
		&veneur.Server{HTTPClient: &http.Client{Transport: transport}}, "datadog", logger, veneur.Config{},
		DatadogSpanSinkConfig{
			SpanBufferSize:  100,
			TraceAPIAddress: "http://example.com",
		})
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
	err = sink.Ingest(testSpan)
	assert.NoError(t, err)

	sink.Flush()
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

func TestDatadogFlushEvents(t *testing.T) {
	transport := &DatadogRoundTripper{Endpoint: "/intake", Contains: ""}
	logger := logrus.NewEntry(logrus.New())
	interval, _ := time.ParseDuration("10s")
	sink, err := CreateMetricSink(&veneur.Server{
		HTTPClient: &http.Client{Transport: transport},
		Interval:   interval,
		Tags:       []string{"gloobles:toots"},
	},
		"datadog", logger, veneur.Config{Hostname: "example.com"},
		DatadogMetricSinkConfig{
			APIKey:                          "secret",
			APIHostname:                     "http://example.com",
			FlushMaxPerBody:                 2500,
			MetricNamePrefixDrops:           nil,
			ExcludeTagsPrefixByPrefixMetric: nil,
		})
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
			"foo":                               "bar",
			"baz":                               "qux",
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

	sink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{testEvent})
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
	logger := logrus.NewEntry(logrus.New())
	interval, _ := time.ParseDuration("10s")
	sink, err := CreateMetricSink(&veneur.Server{
		HTTPClient: &http.Client{Transport: transport},
		Interval:   interval,
		Tags:       []string{"gloobles:toots"},
	},
		"datadog", logger, veneur.Config{Hostname: "example.com"},
		DatadogMetricSinkConfig{
			APIKey:                          "secret",
			APIHostname:                     "http://example.com",
			FlushMaxPerBody:                 2500,
			MetricNamePrefixDrops:           nil,
			ExcludeTagsPrefixByPrefixMetric: nil,
		})
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

	sink.FlushOtherSamples(context.TODO(), []ssf.SSFSample{testCheck})
	assert.NoError(t, err)

	assert.Equal(t, false, transport.GotCalled, "Was not supposed to log a service check in the FlushOtherSamples")
}

func TestDatadogFlushServiceCheck(t *testing.T) {
	transport := &DatadogRoundTripper{Endpoint: "/api/v1/check_run", Contains: ""}
	logger := logrus.NewEntry(logrus.New())
	sink, err := CreateMetricSink(&veneur.Server{
		HTTPClient: &http.Client{Transport: transport},
		Interval:   10 * time.Second,
		Tags:       []string{"gloobles:toots"},
	},
		"datadog", logger, veneur.Config{Hostname: "example.com"},
		DatadogMetricSinkConfig{
			APIKey:                          "secret",
			APIHostname:                     "http://example.com",
			FlushMaxPerBody:                 2500,
			MetricNamePrefixDrops:           nil,
			ExcludeTagsPrefixByPrefixMetric: nil,
		})
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

	sink.Flush(context.TODO(), []samplers.InterMetric{testCheck})
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

func TestDataDogSetExcludeTags(t *testing.T) {
	ddSink := DatadogMetricSink{
		tags: []string{"yay:pie", "boo:snakes"},
	}

	ddSink.SetExcludedTags([]string{"foo", "boo", "host"})

	interMetrics := []samplers.InterMetric{samplers.InterMetric{
		Name:      "a.b.c",
		Timestamp: 1476119058,
		Value:     10,
		Tags: []string{
			"foo:bar",
			"baz:quz",
			"novalue",
		},
		Type: samplers.CounterMetric,
	}}

	ddMetrics, serviceChecks := ddSink.finalizeMetrics(interMetrics)

	assert.Empty(t, serviceChecks, "No service check metrics are reported")
	assert.Equal(t, 1, len(ddMetrics))
	metric := ddMetrics[0]
	assert.Equal(t, "a.b.c", metric.Name, "Metric has wrong name")
	tags := metric.Tags
	assert.Equal(t, 3, len(tags), "Metric has incorrect tag count")

	assert.Equal(t, "yay:pie", tags[0], "Incorrect yay tag in first position")
	assert.Equal(t, "baz:quz", tags[1], "Incorrect baz tag in second position")
	assert.Equal(t, "novalue", tags[2], "Incorrect novalue tag in third position")
}

func TestDataDogDropMetric(t *testing.T) {

	ddSink := DatadogMetricSink{
		metricNamePrefixDrops: []string{"drop"},
	}

	interMetrics := []samplers.InterMetric{
		{Name: "foo.a.b"},
		{Name: "foo.a.b.c"},
		{Name: "drop.a.b.c"},
	}

	ddMetrics, serviceChecks := ddSink.finalizeMetrics(interMetrics)

	assert.Empty(t, serviceChecks, "No service check metrics are reported")
	assert.Equal(t, 2, len(ddMetrics))
}

func TestDataDogDropTagsByMetricPrefix(t *testing.T) {

	ddSink := DatadogMetricSink{
		excludeTagsPrefixByPrefixMetric: map[string][]string{
			"remove.a": []string{"tag-ab"},
		},
	}

	testsMetricCount := []struct {
		Name             string
		Metric           samplers.InterMetric
		expectedTagCount int
	}{
		{"Ignore dropped tags", samplers.InterMetric{Name: "foo.a.b", Tags: []string{"tag-a", "tag-ab", "tag-abc"}}, 3},
		{"dropped tags", samplers.InterMetric{Name: "remove.a.b", Tags: []string{"tag-a", "tag-ab", "tag-abc"}}, 1},
		{"dropped tags", samplers.InterMetric{Name: "remove.a", Tags: []string{"tag-a", "tag-ab"}}, 1},
	}

	for _, test := range testsMetricCount {
		t.Run(test.Name, func(t *testing.T) {
			metrics := []samplers.InterMetric{test.Metric}
			ddMetrics, serviceChecks := ddSink.finalizeMetrics(metrics)
			assert.Empty(t, serviceChecks, "No service check metrics are reported")
			assert.Equal(t, test.expectedTagCount, len(ddMetrics[0].Tags))
		})
	}

}

func TestParseMetricConfig(t *testing.T) {
	testConfigValues := map[string]interface{}{
		"api_key":                  "KEY",
		"api_hostname":             "HOSTNAME",
		"flush_max_per_body":       9001,
		"metric_name_prefix_drops": []string{"prefix1", "prefix2"},
	}

	parsedConfig, err := ParseMetricConfig("datadog", testConfigValues)
	datadogConfig := parsedConfig.(DatadogMetricSinkConfig)
	assert.NoError(t, err)
	assert.Equal(t, datadogConfig.APIKey, testConfigValues["api_key"])
	assert.Equal(t, datadogConfig.APIHostname, testConfigValues["api_hostname"])
	assert.Equal(t, datadogConfig.FlushMaxPerBody, testConfigValues["flush_max_per_body"])
	assert.Equal(t, datadogConfig.MetricNamePrefixDrops, testConfigValues["metric_name_prefix_drops"])
}

func TestMigrateMetricConfig(t *testing.T) {
	testConfigValues := map[string]interface{}{
		"datadog_api_key":                  "KEY",
		"datadog_api_hostname":             "HOSTNAME",
		"datadog_flush_max_per_body":       9001,
		"datadog_metric_name_prefix_drops": []string{"prefix1", "prefix2"},
	}
	bts, _ := yaml.Marshal(testConfigValues)
	config := veneur.Config{}
	_ = yaml.Unmarshal(bts, &config)

	MigrateConfig(&config)
	assert.NotEmpty(t, config.MetricSinks)
	datadogConfig := config.MetricSinks[0].Config.(DatadogMetricSinkConfig)
	assert.Equal(t, datadogConfig.APIKey, testConfigValues["datadog_api_key"])
	assert.Equal(t, datadogConfig.APIHostname, testConfigValues["datadog_api_hostname"])
	assert.Equal(t, datadogConfig.FlushMaxPerBody, testConfigValues["datadog_flush_max_per_body"])
	assert.Equal(t, datadogConfig.MetricNamePrefixDrops, testConfigValues["datadog_metric_name_prefix_drops"])
}

func TestParseSpanConfig(t *testing.T) {
	testConfigValues := map[string]interface{}{
		"datadog_span_buffer_size":  1337,
		"datadog_trace_api_address": "0.0.0.0",
	}

	parsedConfig, err := ParseSpanConfig("datadog", testConfigValues)
	datadogConfig := parsedConfig.(DatadogSpanSinkConfig)
	assert.NoError(t, err)
	assert.Equal(t, datadogConfig.SpanBufferSize, testConfigValues["datadog_span_buffer_size"])
	assert.Equal(t, datadogConfig.TraceAPIAddress, testConfigValues["datadog_trace_api_address"])
}
