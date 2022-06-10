package prometheus

import (
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks/prometheus/prompb"
	"github.com/stripe/veneur/v14/trace"
)

func TestNewRemoteWriteExporter(t *testing.T) {
	for name, tc := range map[string]struct {
		addr        string
		bearerToken string
		wantErr     bool
	}{
		"valid http address and auth token": {
			addr:        "http://127.0.0.1:5000/remote/write",
			bearerToken: "test1234",
			wantErr:     false,
		},
		"valid https address and auth token": {
			addr:        "https://api.service.com/remote/write",
			bearerToken: "test1234",
			wantErr:     false,
		},
		"invalid address": {
			addr:        "hi",
			bearerToken: "foo",
			wantErr:     true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := CreateRWMetricSink(
				&veneur.Server{
					TraceClient: nil,
				},
				"prometheus_rw",
				testLogger(),
				veneur.Config{},
				PrometheusRemoteWriteSinkConfig{
					WriteAddress:        tc.addr,
					BearerToken:         tc.bearerToken,
					FlushMaxConcurrency: 5,
					FlushMaxPerBody:     5,
				})
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoteWriteSinkName(t *testing.T) {
	sink, err := CreateMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"prometheus-test-rw",
		testLogger(),
		veneur.Config{},
		PrometheusRemoteWriteSinkConfig{
			WriteAddress: "http://127.0.0.1:5000",
			BearerToken:  "foobar",
		})
	assert.NotNil(t, sink)
	assert.NoError(t, err)
	assert.Equal(t, "prometheus-test-rw", sink.Name())
}

func TestRemoteWriteMetricFlush(t *testing.T) {
	// Create an HTTP server emulating the remote write endpoint and saving the request
	resChan := make(chan prompb.WriteRequest, 5)
	remoteServer := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			compressed, err := ioutil.ReadAll(r.Body)
			assert.NoError(t, err)
			uncompressed, err := snappy.Decode(nil, compressed)
			assert.NoError(t, err)
			var res prompb.WriteRequest
			err = proto.Unmarshal(uncompressed, &res)
			assert.NoError(t, err)

			w.WriteHeader(http.StatusOK)
			resChan <- res
		}))
	defer remoteServer.Close()

	// Limit batchSize for testing.
	batchSize := 3
	expectedRequests := []prompb.WriteRequest{
		{
			Metadata: []prompb.MetricMetadata{
				{Type: prompb.MetricMetadata_DELTA_COUNTER, MetricFamilyName: "a_b_counter"},
				{Type: prompb.MetricMetadata_DELTA_COUNTER, MetricFamilyName: "a_b_counter2"},
				{Type: prompb.MetricMetadata_GAUGE, MetricFamilyName: "a_b_gauge"},
				{Type: prompb.MetricMetadata_GAUGE, MetricFamilyName: "a_b_gauge2"},
			},
		},
		{
			Timeseries: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "a_b_gauge"},
						{Name: "foo", Value: "bar"},
						{Name: "baz", Value: "quz"},
						{Name: "default", Value: "abc"},
						{Name: "host", Value: "localhost"},
					},
					Samples: []prompb.Sample{
						{Timestamp: 1000, Value: float64(100)}, // timestamp in ms
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "a_b_counter"},
						{Name: "foo", Value: "bar"},
						{Name: "default", Value: "abc"},
						{Name: "host", Value: "localhost"},
					},
					Samples: []prompb.Sample{
						{Timestamp: 1000, Value: float64(2)}, // timestamp in ms
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "a_b_status"},
						{Name: "default", Value: "abc"},
						{Name: "host", Value: "localhost"},
					},
					Samples: []prompb.Sample{
						{Timestamp: 1000, Value: float64(5)}, // timestamp in ms
					},
				},
			},
		},
		{
			Timeseries: []prompb.TimeSeries{
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "a_b_gauge2"},
						{Name: "foo", Value: "bar"},
						{Name: "baz", Value: "zazz"},
						{Name: "default", Value: "abc"},
						{Name: "host", Value: "localhost"},
					},
					Samples: []prompb.Sample{
						{Timestamp: 1000, Value: float64(222)}, // timestamp in ms
					},
				},
				{
					Labels: []prompb.Label{
						{Name: "__name__", Value: "a_b_counter2"},
						{Name: "foo", Value: "bar"},
						{Name: "default", Value: "override"},
						{Name: "host", Value: "localhost"},
					},
					Samples: []prompb.Sample{
						{Timestamp: 1000, Value: float64(33)}, // timestamp in ms
					},
				},
			},
		},
	}

	sink, err := CreateRWMetricSink(
		&veneur.Server{
			TraceClient: nil,
		},
		"prometheus_rw",
		testLogger(),
		veneur.Config{},
		PrometheusRemoteWriteSinkConfig{
			WriteAddress:        remoteServer.URL,
			BearerToken:         "token",
			FlushMaxConcurrency: 1,
			FlushMaxPerBody:     batchSize,
		})
	assert.NoError(t, err)

	assert.NoError(t, sink.Start(trace.DefaultClient))
	assert.NoError(t, sink.Flush(context.Background(), []samplers.InterMetric{
		samplers.InterMetric{
			Name:      "a.b.gauge",
			Timestamp: 1,
			Value:     float64(100),
			Tags: []string{
				"foo:bar",
				"baz:quz",
			},
			Type: samplers.GaugeMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.counter",
			Timestamp: 1,
			Value:     float64(2),
			Tags: []string{
				"foo:bar",
			},
			Type: samplers.CounterMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.status",
			Timestamp: 1,
			Value:     float64(5),
			Type:      samplers.StatusMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.gauge2",
			Timestamp: 1,
			Value:     float64(222),
			Tags: []string{
				"foo:bar",
				"foo:quz",
				"baz:zazz",
			},
			Type: samplers.GaugeMetric,
		},
		samplers.InterMetric{
			Name:      "a.b.counter2",
			Timestamp: 1,
			Value:     float64(33),
			Tags: []string{
				"foo:bar",
				"default:override",
			},
			Type: samplers.CounterMetric,
		},
	}))

	for _, want := range expectedRequests {
		select {
		case res := <-resChan:
			sort.Slice(res.Metadata, func(i, j int) bool { return res.Metadata[i].MetricFamilyName < res.Metadata[j].MetricFamilyName })
			assert.Equal(t, want, res)
		}
	}
}

func TestParseRemoteWriteConfig(t *testing.T) {
	parsedConfig, err := ParseMetricConfig("prometheus_rw", map[string]interface{}{
		"write_address":         "127.0.0.1:5000",
		"bearer_token":          "test_token",
		"flush_max_concurrency": 55,
		"flush_max_per_body":    88,
	})
	prometheusRWConfig := parsedConfig.(PrometheusRemoteWriteSinkConfig)
	assert.NoError(t, err)
	assert.Equal(t, prometheusRWConfig.WriteAddress, "127.0.0.1:5000")
	assert.Equal(t, prometheusRWConfig.BearerToken, "test_token")
	assert.Equal(t, prometheusRWConfig.FlushMaxConcurrency, 55)
	assert.Equal(t, prometheusRWConfig.FlushMaxPerBody, 88)
}
