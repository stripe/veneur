package json_test

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	proxyJson "github.com/stripe/veneur/v14/proxy/json"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/tdigest"
)

var metricsJson = []byte("[{\"name\":\"metric\",\"type\":\"counter\",\"tags\":[\"tag1:value1\"],\"value\":[1,0,0,0,0,0,0,0]}]")

func TestParseRequest(t *testing.T) {
	request := httptest.NewRequest(
		"POST", "/import", bytes.NewReader(metricsJson))

	jsonMetrics, err := proxyJson.ParseRequest(request)

	assert.Nil(t, err)
	if assert.Len(t, jsonMetrics, 1) {
		assert.Equal(t, "metric", jsonMetrics[0].Name)
		assert.Equal(t, "counter", jsonMetrics[0].Type)
		assert.Equal(t, []string{"tag1:value1"}, jsonMetrics[0].Tags)
		assert.Equal(t, []uint8{1, 0, 0, 0, 0, 0, 0, 0}, jsonMetrics[0].Value)
	}
}

func TestParseRequestZlib(t *testing.T) {
	var encodedMetrics bytes.Buffer
	writer := zlib.NewWriter(&encodedMetrics)
	_, err := writer.Write([]byte(metricsJson))
	assert.NoError(t, err)
	err = writer.Close()
	assert.NoError(t, err)
	request := httptest.NewRequest(
		"POST", "/import", bytes.NewReader(encodedMetrics.Bytes()))
	request.Header.Add("Content-Encoding", "deflate")

	jsonMetrics, parseErr := proxyJson.ParseRequest(request)

	assert.Nil(t, parseErr)
	if assert.Len(t, jsonMetrics, 1) {
		assert.Equal(t, "metric", jsonMetrics[0].Name)
		assert.Equal(t, "counter", jsonMetrics[0].Type)
		assert.Equal(t, []string{"tag1:value1"}, jsonMetrics[0].Tags)
		assert.Equal(t, []uint8{1, 0, 0, 0, 0, 0, 0, 0}, jsonMetrics[0].Value)
	}
}

func TestParseRequestInvalidBody(t *testing.T) {
	body := []byte("invalid body")
	request := httptest.NewRequest("POST", "/import", bytes.NewReader(body))

	jsonMetrics, err := proxyJson.ParseRequest(request)

	assert.Nil(t, jsonMetrics)
	assert.IsType(t, &json.SyntaxError{}, err.Err)
	assert.Equal(t, "failed to json decode body", err.Message)
	assert.Equal(t, http.StatusBadRequest, err.Status)
	assert.Equal(t, "error_decode", err.Tag)
}

func TestParseRequestEmptyBody(t *testing.T) {
	body := []byte("[]")
	request := httptest.NewRequest("POST", "/import", bytes.NewReader(body))

	jsonMetrics, err := proxyJson.ParseRequest(request)

	assert.Nil(t, jsonMetrics)
	assert.Equal(t, "received empty import request", err.Err.Error())
	assert.Equal(t, "invalid request", err.Message)
	assert.Equal(t, http.StatusBadRequest, err.Status)
	assert.Equal(t, "error_empty", err.Tag)
}

func TestParseRequestInvalidEncoding(t *testing.T) {
	body := []byte("[]")
	request := httptest.NewRequest("POST", "/import", bytes.NewReader(body))
	request.Header.Add("Content-Encoding", "invalid-encoding")

	jsonMetrics, err := proxyJson.ParseRequest(request)

	assert.Nil(t, jsonMetrics)
	assert.Equal(t, "unknown Content-Encoding: invalid-encoding", err.Err.Error())
	assert.Equal(t, "invalid request", err.Message)
	assert.Equal(t, http.StatusUnsupportedMediaType, err.Status)
	assert.Equal(t, "error_unknown_encoding", err.Tag)
}

func TestParseRequestZlibDecodeError(t *testing.T) {
	body := []byte("[]")
	request := httptest.NewRequest("POST", "/import", bytes.NewReader(body))
	request.Header.Add("Content-Encoding", "deflate")

	jsonMetrics, err := proxyJson.ParseRequest(request)

	assert.Nil(t, jsonMetrics)
	assert.Equal(t, "zlib: invalid header", err.Err.Error())
	assert.Equal(t, "failed to zlib decode body", err.Message)
	assert.Equal(t, http.StatusBadRequest, err.Status)
	assert.Equal(t, "error_zlib", err.Tag)
}

func TestConvertJsonMetricCounter(t *testing.T) {
	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "counter",
		},
		Tags:  []string{"tag1:value1"},
		Value: []byte{10, 0, 0, 0, 0, 0, 0, 0},
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.NoError(t, err) {
		assert.Equal(t, "metric", metric.Name)
		assert.Equal(t, metricpb.Type_Counter, metric.Type)
		assert.Equal(t, []string{"tag1:value1"}, metric.Tags)
		if assert.IsType(t, &metricpb.Metric_Counter{}, metric.Value) {
			assert.Equal(t, int64(10),
				metric.Value.(*metricpb.Metric_Counter).Counter.Value)
		}
	}
}

func TestConvertJsonMetricGauge(t *testing.T) {
	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "gauge",
		},
		Tags:  []string{"tag1:value1"},
		Value: []byte{0, 0, 0, 0, 0, 0, 0x24, 0x40},
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.NoError(t, err) {
		assert.Equal(t, "metric", metric.Name)
		assert.Equal(t, metricpb.Type_Gauge, metric.Type)
		assert.Equal(t, []string{"tag1:value1"}, metric.Tags)
		if assert.IsType(t, &metricpb.Metric_Gauge{}, metric.Value) {
			assert.Equal(t, 10.0,
				metric.Value.(*metricpb.Metric_Gauge).Gauge.Value)
		}
	}
}

func TestConvertJsonMetricSet(t *testing.T) {
	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "set",
		},
		Tags:  []string{"tag1:value1"},
		Value: []byte{1, 2, 3},
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.NoError(t, err) {
		assert.Equal(t, "metric", metric.Name)
		assert.Equal(t, metricpb.Type_Set, metric.Type)
		assert.Equal(t, []string{"tag1:value1"}, metric.Tags)
		if assert.IsType(t, &metricpb.Metric_Set{}, metric.Value) {
			assert.Equal(t, []byte{1, 2, 3},
				metric.Value.(*metricpb.Metric_Set).Set.HyperLogLog)
		}
	}
}

func TestConvertJsonMetricHistogram(t *testing.T) {
	gob, err := tdigest.NewMerging(100.0, true).GobEncode()
	assert.NoError(t, err)

	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "histogram",
		},
		Tags:  []string{"tag1:value1"},
		Value: gob,
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.NoError(t, err) {
		assert.Equal(t, "metric", metric.Name)
		assert.Equal(t, metricpb.Type_Histogram, metric.Type)
		assert.Equal(t, []string{"tag1:value1"}, metric.Tags)
		if assert.IsType(t, &metricpb.Metric_Histogram{}, metric.Value) {
			_, err :=
				metric.Value.(*metricpb.Metric_Histogram).Histogram.Marshal()
			assert.NoError(t, err)
		}
	}
}

func TestConvertJsonMetricTimer(t *testing.T) {
	gob, err := tdigest.NewMerging(100.0, true).GobEncode()
	assert.NoError(t, err)

	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "timer",
		},
		Tags:  []string{"tag1:value1"},
		Value: gob,
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.NoError(t, err) {
		assert.Equal(t, "metric", metric.Name)
		assert.Equal(t, metricpb.Type_Histogram, metric.Type)
		assert.Equal(t, []string{"tag1:value1"}, metric.Tags)
		if assert.IsType(t, &metricpb.Metric_Histogram{}, metric.Value) {
			_, err :=
				metric.Value.(*metricpb.Metric_Histogram).Histogram.Marshal()
			assert.NoError(t, err)
		}
	}
}

func TestConvertJsonMetricUnknownType(t *testing.T) {
	jsonMetric := samplers.JSONMetric{
		MetricKey: samplers.MetricKey{
			Name: "metric",
			Type: "invalid-type",
		},
		Tags:  []string{"tag1:value1"},
		Value: []byte{1, 2, 3},
	}

	metric, err := proxyJson.ConvertJsonMetric(&jsonMetric)

	if assert.Error(t, err) {
		assert.Equal(t, "unknown metric type: invalid-type", err.Error())
	}
	assert.Nil(t, metric)
}
