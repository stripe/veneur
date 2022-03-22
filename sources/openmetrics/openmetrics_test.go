package openmetrics_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	dto "github.com/prometheus/client_model/go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sources/openmetrics"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v2"
)

func TestParseConfig(t *testing.T) {
	yamlConfig := `---
allowlist: ^foo.*$
denylist: ^bar.*$
scrape_interval: 10s
scrape_target: https://example.com/
`
	parsedConfig := map[string]interface{}{}
	yaml.Unmarshal([]byte(yamlConfig), &parsedConfig)

	config, err := openmetrics.ParseConfig("openmetrics", parsedConfig)
	assert.NoError(t, err)
	openMetricsConfig, ok := config.(openmetrics.OpenMetricsSourceConfig)
	assert.True(t, ok)

	assert.Equal(t, "le", openMetricsConfig.HistogramBucketTag)
	if assert.NotNil(t, openMetricsConfig.ScrapeTarget.Value) {
		assert.Equal(
			t, "https://example.com/", openMetricsConfig.ScrapeTarget.Value.String())
	}
	if assert.NotNil(t, openMetricsConfig.Allowlist.Value) {
		assert.Equal(t, "^foo.*$", openMetricsConfig.Allowlist.Value.String())
	}
	if assert.NotNil(t, openMetricsConfig.Denylist.Value) {
		assert.Equal(t, "^bar.*$", openMetricsConfig.Denylist.Value.String())
	}
	assert.Equal(t, 10*time.Second, openMetricsConfig.ScrapeInterval)
	assert.Equal(t, "quantile", openMetricsConfig.SummaryQuantileTag)
}

func CreateSource(
	t *testing.T, httpServer *httptest.Server,
	config openmetrics.OpenMetricsSourceConfig,
) *openmetrics.OpenMetricsSource {
	source, err := openmetrics.Create(
		&veneur.Server{
			HTTPClient: httpServer.Client(),
		}, "openmetrics", logrus.NewEntry(logrus.StandardLogger()), config)
	require.NoError(t, err)
	openMetricsSource, ok := source.(openmetrics.OpenMetricsSource)
	require.True(t, ok)

	return &openMetricsSource
}

func TestName(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{})
	assert.Equal(t, "openmetrics", source.Name())
}

func TestQuery(t *testing.T) {
	testHttpServer := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(`
# TYPE requests counter
requests{status="200"} 3
requests{status="500"} 1
# EOF
`))
		},
	))
	testUrl, err := url.Parse(testHttpServer.URL)
	assert.NoError(t, err)

	source := CreateSource(
		t, testHttpServer,
		openmetrics.OpenMetricsSourceConfig{
			ScrapeTarget: util.Url{
				Value: testUrl,
			},
		})

	queryResponse, err := source.Query(context.Background())
	assert.NoError(t, err)

	results := []dto.MetricFamily{}
	for result := range queryResponse {
		assert.NoError(t, result.Error)
		results = append(results, result.MetricFamily)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "requests", *results[0].Name)
		assert.Equal(t, dto.MetricType_COUNTER, *results[0].Type)
		if assert.Len(t, results[0].Metric, 2) {
			if assert.Len(t, results[0].Metric[0].Label, 1) {
				assert.Equal(t, "status", *results[0].Metric[0].Label[0].Name)
				assert.Equal(t, "200", *results[0].Metric[0].Label[0].Value)
			}
			if assert.NotNil(t, results[0].Metric[0].Counter) {
				assert.Equal(t, 3.0, *results[0].Metric[0].Counter.Value)
			}

			if assert.Len(t, results[0].Metric[1].Label, 1) {
				assert.Equal(t, "status", *results[0].Metric[1].Label[0].Name)
				assert.Equal(t, "500", *results[0].Metric[1].Label[0].Value)
			}
			if assert.NotNil(t, results[0].Metric[1].Counter) {
				assert.Equal(t, 1.0, *results[0].Metric[1].Counter.Value)
			}
		}
	}
}

func TestQueryMetricsAllowlist(t *testing.T) {
	testHttpServer := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(`
foo 5
bar 7
# EOF
`))
		},
	))
	testUrl, err := url.Parse(testHttpServer.URL)
	assert.NoError(t, err)

	source := CreateSource(
		t, testHttpServer,
		openmetrics.OpenMetricsSourceConfig{
			ScrapeTarget: util.Url{
				Value: testUrl,
			},
			Allowlist: util.Regexp{
				Value: regexp.MustCompile("^foo$"),
			},
		})

	queryResponse, err := source.Query(context.Background())
	assert.NoError(t, err)

	results := []dto.MetricFamily{}
	for result := range queryResponse {
		assert.NoError(t, result.Error)
		results = append(results, result.MetricFamily)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "foo", *results[0].Name)
		assert.Equal(t, dto.MetricType_UNTYPED, *results[0].Type)
		if assert.Len(t, results[0].Metric, 1) {
			if assert.NotNil(t, results[0].Metric[0].Untyped) {
				assert.Equal(t, 5.0, *results[0].Metric[0].Untyped.Value)
			}
		}
	}
}

func TestQueryMetricsDenylist(t *testing.T) {
	testHttpServer := httptest.NewServer(http.HandlerFunc(
		func(writer http.ResponseWriter, request *http.Request) {
			writer.Write([]byte(`
foo 5
bar 7
# EOF
`))
		},
	))
	testUrl, err := url.Parse(testHttpServer.URL)
	assert.NoError(t, err)

	source := CreateSource(
		t, testHttpServer,
		openmetrics.OpenMetricsSourceConfig{
			ScrapeTarget: util.Url{
				Value: testUrl,
			},
			Denylist: util.Regexp{
				Value: regexp.MustCompile("^foo$"),
			},
		})

	queryResponse, err := source.Query(context.Background())
	assert.NoError(t, err)

	results := []dto.MetricFamily{}
	for result := range queryResponse {
		assert.NoError(t, result.Error)
		results = append(results, result.MetricFamily)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "bar", *results[0].Name)
		assert.Equal(t, dto.MetricType_UNTYPED, *results[0].Type)
		if assert.Len(t, results[0].Metric, 1) {
			if assert.NotNil(t, results[0].Metric[0].Untyped) {
				assert.Equal(t, 7.0, *results[0].Metric[0].Untyped.Value)
			}
		}
	}
}

const counterMetric = `
name: "metric_name"
type: 0  # counter
metric: {
	counter: {
		value: 3.0
	}
	label: {
		name: "tag1"
		value: "value1"
	}
	label: {
		name: "tag2"
		value: "value2"
	}
	timestamp_ms: 101
}
`

func TestConvertCounter(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{})

	metricFamily := dto.MetricFamily{}
	err := proto.UnmarshalText(counterMetric, &metricFamily)
	require.NoError(t, err)

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			MetricFamily: metricFamily,
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []*samplers.UDPMetric{}
	for result := range convertResults {
		assert.NoError(t, result.Error)
		results = append(results, result.Metric)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "counter", results[0].Type)
		assert.Equal(t, "metric_name", results[0].Name)
		if assert.Len(t, results[0].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[0].Tags[0])
			assert.Equal(t, "tag2:value2", results[0].Tags[1])
		}
		assert.Equal(t, 3.0, results[0].Value)
		assert.Equal(t, int64(101), results[0].Timestamp)
	}
}

const gaugeMetric = `
name: "metric_name"
type: 1  # gauge
metric: {
	gauge: {
		value: 3.0
	}
	label: {
		name: "tag1"
		value: "value1"
	}
	label: {
		name: "tag2"
		value: "value2"
	}
	timestamp_ms: 107
}
`

func TestConvertGauge(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{})

	metricFamily := dto.MetricFamily{}
	err := proto.UnmarshalText(gaugeMetric, &metricFamily)
	require.NoError(t, err)

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			MetricFamily: metricFamily,
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []*samplers.UDPMetric{}
	for result := range convertResults {
		assert.NoError(t, result.Error)
		results = append(results, result.Metric)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "gauge", results[0].Type)
		assert.Equal(t, "metric_name", results[0].Name)
		if assert.Len(t, results[0].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[0].Tags[0])
			assert.Equal(t, "tag2:value2", results[0].Tags[1])
		}
		assert.Equal(t, 3.0, results[0].Value)
		assert.Equal(t, int64(107), results[0].Timestamp)
	}
}

const summaryMetric = `
name: "metric_name"
type: 2  # summary
metric: {
	summary: {
		quantile: {
			quantile: 90
			value: 1.0
		}
		quantile: {
			quantile: 99
			value: 2.0
		}
		sample_count: 3
		sample_sum: 4.0
	}
	label: {
		name: "tag1"
		value: "value1"
	}
	label: {
		name: "tag2"
		value: "value2"
	}
	timestamp_ms: 127
}
`

func TestConvertSummary(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{
			SummaryQuantileTag: "quantile",
		})

	metricFamily := dto.MetricFamily{}
	err := proto.UnmarshalText(summaryMetric, &metricFamily)
	require.NoError(t, err)

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			MetricFamily: metricFamily,
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []*samplers.UDPMetric{}
	for result := range convertResults {
		assert.NoError(t, result.Error)
		results = append(results, result.Metric)
	}

	if assert.Len(t, results, 4) {
		assert.Equal(t, "gauge", results[0].Type)
		assert.Equal(t, "metric_name", results[0].Name)
		if assert.Len(t, results[0].Tags, 3) {
			assert.Equal(t, "tag1:value1", results[0].Tags[0])
			assert.Equal(t, "tag2:value2", results[0].Tags[1])
			assert.Equal(t, "quantile:90.000000", results[0].Tags[2])
		}
		assert.Equal(t, 1.0, results[0].Value)
		assert.Equal(t, int64(127), results[0].Timestamp)

		assert.Equal(t, "gauge", results[1].Type)
		assert.Equal(t, "metric_name", results[1].Name)
		if assert.Len(t, results[1].Tags, 3) {
			assert.Equal(t, "tag1:value1", results[1].Tags[0])
			assert.Equal(t, "tag2:value2", results[1].Tags[1])
			assert.Equal(t, "quantile:99.000000", results[1].Tags[2])
		}
		assert.Equal(t, 2.0, results[1].Value)
		assert.Equal(t, int64(127), results[1].Timestamp)

		assert.Equal(t, "counter", results[2].Type)
		assert.Equal(t, "metric_name.count", results[2].Name)
		if assert.Len(t, results[2].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[2].Tags[0])
			assert.Equal(t, "tag2:value2", results[2].Tags[1])
		}
		assert.Equal(t, 3.0, results[2].Value)
		assert.Equal(t, int64(127), results[2].Timestamp)

		assert.Equal(t, "counter", results[3].Type)
		assert.Equal(t, "metric_name.sum", results[3].Name)
		if assert.Len(t, results[3].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[3].Tags[0])
			assert.Equal(t, "tag2:value2", results[3].Tags[1])
		}
		assert.Equal(t, 4.0, results[3].Value)
		assert.Equal(t, int64(127), results[3].Timestamp)
	}
}

const histogramMetric = `
name: "metric_name"
type: 4  # histogram
metric: {
	histogram: {
		bucket: {
			cumulative_count: 2
			upper_bound: 3.0
		}
		bucket: {
			cumulative_count: 4
			upper_bound: 5.0
		}
		sample_count: 6
		sample_sum: 8.0
	}
	label: {
		name: "tag1"
		value: "value1"
	}
	label: {
		name: "tag2"
		value: "value2"
	}
	timestamp_ms: 149
}
`

func TestConvertHistogram(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{
			HistogramBucketTag: "le",
		})

	metricFamily := dto.MetricFamily{}
	err := proto.UnmarshalText(histogramMetric, &metricFamily)
	require.NoError(t, err)

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			MetricFamily: metricFamily,
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []*samplers.UDPMetric{}
	for result := range convertResults {
		assert.NoError(t, result.Error)
		results = append(results, result.Metric)
	}

	if assert.Len(t, results, 4) {
		assert.Equal(t, "counter", results[0].Type)
		assert.Equal(t, "metric_name.bucket", results[0].Name)
		if assert.Len(t, results[0].Tags, 3) {
			assert.Equal(t, "tag1:value1", results[0].Tags[0])
			assert.Equal(t, "tag2:value2", results[0].Tags[1])
			assert.Equal(t, "le:3.000000", results[0].Tags[2])
		}
		assert.Equal(t, 2.0, results[0].Value)
		assert.Equal(t, int64(149), results[0].Timestamp)

		assert.Equal(t, "counter", results[1].Type)
		assert.Equal(t, "metric_name.bucket", results[1].Name)
		if assert.Len(t, results[1].Tags, 3) {
			assert.Equal(t, "tag1:value1", results[1].Tags[0])
			assert.Equal(t, "tag2:value2", results[1].Tags[1])
			assert.Equal(t, "le:5.000000", results[1].Tags[2])
		}
		assert.Equal(t, 4.0, results[1].Value)
		assert.Equal(t, int64(149), results[1].Timestamp)

		assert.Equal(t, "counter", results[2].Type)
		assert.Equal(t, "metric_name.count", results[2].Name)
		if assert.Len(t, results[2].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[2].Tags[0])
			assert.Equal(t, "tag2:value2", results[2].Tags[1])
		}
		assert.Equal(t, 6.0, results[2].Value)
		assert.Equal(t, int64(149), results[2].Timestamp)

		assert.Equal(t, "counter", results[3].Type)
		assert.Equal(t, "metric_name.sum", results[3].Name)
		if assert.Len(t, results[3].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[3].Tags[0])
			assert.Equal(t, "tag2:value2", results[3].Tags[1])
		}
		assert.Equal(t, 8.0, results[3].Value)
		assert.Equal(t, int64(149), results[3].Timestamp)
	}
}

const untypedMetric = `
name: "metric_name"
type: 3  # untyped
metric: {
	untyped: {
		value: 9.0
	}
	label: {
		name: "tag1"
		value: "value1"
	}
	label: {
		name: "tag2"
		value: "value2"
	}
	timestamp_ms: 151
}
`

func TestConvertUntyped(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{
			HistogramBucketTag: "le",
		})

	metricFamily := dto.MetricFamily{}
	err := proto.UnmarshalText(untypedMetric, &metricFamily)
	require.NoError(t, err)

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			MetricFamily: metricFamily,
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []*samplers.UDPMetric{}
	for result := range convertResults {
		assert.NoError(t, result.Error)
		results = append(results, result.Metric)
	}

	if assert.Len(t, results, 1) {
		assert.Equal(t, "gauge", results[0].Type)
		assert.Equal(t, "metric_name", results[0].Name)
		if assert.Len(t, results[0].Tags, 2) {
			assert.Equal(t, "tag1:value1", results[0].Tags[0])
			assert.Equal(t, "tag2:value2", results[0].Tags[1])
		}
		assert.Equal(t, 9.0, results[0].Value)
		assert.Equal(t, int64(151), results[0].Timestamp)
	}
}

func TestConvertError(t *testing.T) {
	source := CreateSource(
		t, httptest.NewServer(http.DefaultServeMux),
		openmetrics.OpenMetricsSourceConfig{
			HistogramBucketTag: "le",
		})

	queryResults := make(chan openmetrics.QueryResults)
	go func() {
		queryResults <- openmetrics.QueryResults{
			Error: errors.New("test error"),
		}
		close(queryResults)
	}()

	convertResults := source.Convert(queryResults)

	results := []error{}
	for result := range convertResults {
		assert.Error(t, result.Error)
		results = append(results, result.Error)
	}
	assert.Len(t, results, 1)
}
