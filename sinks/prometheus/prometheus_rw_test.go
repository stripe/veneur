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
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
			_, err := NewRemoteWriteExporter(tc.addr, tc.bearerToken, 5, 5, "localhost", []string{}, nil)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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

	logger := logrus.StandardLogger()
	sink, err := NewRemoteWriteExporter(remoteServer.URL, "token", batchSize, 1, "localhost", []string{"default:abc"}, logger)
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
