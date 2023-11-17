package prometheus

import (
	"container/list"
	"context"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/google/uuid"
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
	sink, err := CreateRWMetricSink(
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
			Tags:        []string{"default:abc"},
		},
		"prometheus_rw",
		testLogger(),
		veneur.Config{
			Hostname: "localhost",
		},
		PrometheusRemoteWriteSinkConfig{
			WriteAddress:        remoteServer.URL,
			BearerToken:         "token",
			FlushMaxConcurrency: 1,
			FlushMaxPerBody:     batchSize,
			FlushTimeout:        35,
			BufferQueueSize:     512,
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
	parsedConfig, err := ParseRWMetricConfig("prometheus_rw", map[string]interface{}{
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

func TestQueueEnqueueOperations(t *testing.T) {
	q := NewConcurrentQueue()
	q.maxByteSize = 100
	q.byteSize = 0
	q.list = list.New()
	q.logger = testLogger()

	ctx := context.Background()
	id := uuid.New()
	ts := time.Now()

	req1 := CreateRwRequest(ctx, 40, ts, id)
	req2 := CreateRwRequest(ctx, 25, ts, id)
	req3 := CreateRwRequest(ctx, 33, ts, id)
	req4 := CreateRwRequest(ctx, 27, ts, id)
	req5 := CreateRwRequest(ctx, 19, ts, id)
	req6 := CreateRwRequest(ctx, 10, ts, id)

	client := trace.DefaultClient
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(client)

	// normal use-case, append elements in the back of the queue
	q.Enqueue(span, req1)
	q.Enqueue(span, req2)
	q.Enqueue(span, req3)

	// we are still below max queue size
	assert.Equal(t, q.CurrentQueueByteSize(), 98)
	AssertQueueState(t, q, []RWRequest{req1, req2, req3})

	// we append an additional element, we must free space in the queue by removing the older ones
	q.Enqueue(span, req4)
	assert.Equal(t, q.CurrentQueueByteSize(), 85)
	AssertQueueState(t, q, []RWRequest{req2, req3, req4})

	// if we try to pre-pend and the queue doesn't have enough space the element is dropped
	q.EnqueueFront(span, req5)
	assert.Equal(t, q.CurrentQueueByteSize(), 85)
	AssertQueueState(t, q, []RWRequest{req2, req3, req4})

	// now there is space, it will be put in the front
	q.EnqueueFront(span, req6)
	assert.Equal(t, q.CurrentQueueByteSize(), 95)
	AssertQueueState(t, q, []RWRequest{req6, req2, req3, req4})
}

func TestQueueGetFirstBatch(t *testing.T) {

	q := NewConcurrentQueue()
	q.maxByteSize = 1000
	q.byteSize = 0
	q.list = list.New()
	q.logger = testLogger()

	ctx := context.Background()

	id := uuid.New()
	id1 := uuid.New()
	id2 := uuid.New()

	ts := time.Now()
	ts1 := ts.Add(time.Second * 10)
	ts2 := ts1.Add(time.Second * 10)

	req1 := CreateRwRequest(ctx, 40, ts, id)
	req2 := CreateRwRequest(ctx, 25, ts, id)
	req3 := CreateRwRequest(ctx, 33, ts1, id1)
	req4 := CreateRwRequest(ctx, 27, ts2, id2)
	req5 := CreateRwRequest(ctx, 19, ts2, id2)
	req6 := CreateRwRequest(ctx, 10, ts2, id2)

	client := trace.DefaultClient
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(client)

	q.Enqueue(span, req1)
	q.Enqueue(span, req2)
	q.Enqueue(span, req3)
	q.Enqueue(span, req4)
	q.Enqueue(span, req5)
	q.Enqueue(span, req6)

	// first batch will get all requests with "id"
	AssertOrderedRequests(t, q.GetFirstBatch(), []RWRequest{req1, req2})

	// second batch will get all requests with "id1"
	AssertOrderedRequests(t, q.GetFirstBatch(), []RWRequest{req3})

	// third batch will get all requests with "id2"
	AssertOrderedRequests(t, q.GetFirstBatch(), []RWRequest{req4, req5, req6})

	// no more data
	assert.Empty(t, q.GetFirstBatch())
	assert.True(t, q.IsEmpty())
}

func CreateRwRequest(ctx context.Context, size int, ts time.Time, id uuid.UUID) RWRequest {
	return RWRequest{
		request:   nil,
		size:      size,
		ctx:       ctx,
		timestamp: ts,
		id:        id,
	}
}

func AssertQueueState(t *testing.T, q *ConcurrentQueue, expectedRequests []RWRequest) {
	assert.Equal(t, q.list.Len(), len(expectedRequests))

	expectedTotSize := 0
	it := q.list.Front()
	for _, expected := range expectedRequests {
		actual := it.Value
		assert.Equal(t, expected, actual)
		it = it.Next()
		expectedTotSize += expected.size
	}
	assert.Equal(t, q.CurrentQueueByteSize(), expectedTotSize)
}

func AssertOrderedRequests(t *testing.T, actualRequests []list.Element, expectedRequests []RWRequest) {
	assert.Equal(t, len(actualRequests), len(expectedRequests))
	for i, expected := range expectedRequests {
		actual := actualRequests[i].Value
		assert.Equal(t, expected, actual)
	}
}
