package splunk_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	hec "github.com/fuyufjh/splunk-hec-go"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/sinks/splunk"
	"github.com/stripe/veneur/ssf"
)

func jsonEndpoint(t testing.TB, ch chan<- hec.Event) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failed := false
		input := hec.Event{}
		j := json.NewDecoder(r.Body)
		defer r.Body.Close()

		j.DisallowUnknownFields()
		for {
			err := j.Decode(&input)
			if err != nil {
				if err == io.EOF {
					return
				}
				t.Errorf("Decoding JSON: %v", err)
				failed = true
				break
			}
			if ch != nil {
				ch <- input
			}
		}
		if failed {
			w.WriteHeader(400)
			w.Write([]byte(`{"text": "Error processing event", "code": 90}`))
		} else {
			w.Write([]byte(`{"text":"Success","code":0}`))
		}
	})
}

func TestSpanIngestBatch(t *testing.T) {
	const nToFlush = 10
	logger := logrus.StandardLogger()

	ch := make(chan hec.Event, nToFlush)
	ts := httptest.NewServer(jsonEndpoint(t, ch))
	defer ts.Close()
	sink, err := splunk.NewSplunkSpanSink(ts.URL, "00000000-0000-0000-0000-000000000000",
		"test-host", "", logger, time.Duration(0))
	require.NoError(t, err)
	err = sink.Start(nil)
	require.NoError(t, err)

	start := time.Unix(100000, 1000000)
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		ParentId:       4,
		TraceId:        6,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Service:        "test-srv",
		Name:           "test-span",
		Indicator:      true,
		Error:          true,
		Tags: map[string]string{
			"farts": "mandatory",
		},
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
		},
	}
	for i := 0; i < nToFlush; i++ {
		span.Id = int64(i + 1)
		err = sink.Ingest(span)
		require.NoError(t, err)
	}

	sink.Flush()

	for i := 0; i < nToFlush; i++ {
		event := <-ch
		fakeEvent := hec.Event{}
		fakeEvent.SetTime(start)

		assert.Equal(t, *fakeEvent.Time, *event.Time)
		assert.Equal(t, "test-srv", *event.SourceType)
		assert.Equal(t, "test-host", *event.Host)

		spanB, err := json.Marshal(event.Event)
		require.NoError(t, err)
		output := splunk.SerializedSSF{}
		err = json.Unmarshal(spanB, &output)

		assert.Equal(t, float64(span.StartTimestamp)/float64(time.Second), output.StartTimestamp)
		assert.Equal(t, float64(span.EndTimestamp)/float64(time.Second), output.EndTimestamp)

		assert.Equal(t, strconv.FormatInt(int64(i+1), 10), output.Id)
		assert.Equal(t, strconv.FormatInt(span.ParentId, 10), output.ParentId)
		assert.Equal(t, strconv.FormatInt(span.TraceId, 10), output.TraceId)
		assert.Equal(t, "test-span", output.Name)
		assert.Equal(t, map[string]string{"farts": "mandatory"}, output.Tags)
		assert.Equal(t, true, output.Indicator)
		assert.Equal(t, true, output.Error)
	}
}

func BenchmarkBatchFlushing(b *testing.B) {
	const flushSpans = 1000 // number of spans to accumulate before flushing
	logger := logrus.StandardLogger()

	// set up a null responder that we can flush to:
	ts := httptest.NewServer(jsonEndpoint(b, nil))
	defer ts.Close()
	sink, err := splunk.NewSplunkSpanSink(ts.URL, "00000000-0000-0000-0000-000000000000",
		"test-host", "", logger, time.Duration(0))
	require.NoError(b, err)
	err = sink.Start(nil)
	require.NoError(b, err)

	start := time.Unix(100000, 1000000)
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		ParentId:       4,
		TraceId:        6,
		Id:             0,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Service:        "test-srv",
		Name:           "test-span",
		Indicator:      true,
		Error:          true,
		Tags: map[string]string{
			"farts": "mandatory",
		},
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
		},
	}

	b.ResetTimer()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		span.Id = int64(i + 1)
		err = sink.Ingest(span)
		require.NoError(b, err)
		if i != 0 && i%flushSpans == 0 {
			b.StartTimer()
			sink.Flush()
			b.StopTimer()
		}
	}
}

func BenchmarkBatchIngest(b *testing.B) {
	const flushSpans = 400000 // number of spans to accumulate before flushing
	logger := logrus.StandardLogger()

	// set up a null responder that we can flush to:
	ts := httptest.NewServer(jsonEndpoint(b, nil))
	defer ts.Close()
	sink, err := splunk.NewSplunkSpanSink(ts.URL, "00000000-0000-0000-0000-000000000000",
		"test-host", "", logger, time.Duration(0))
	require.NoError(b, err)
	err = sink.Start(nil)
	require.NoError(b, err)

	start := time.Unix(100000, 1000000)
	end := start.Add(5 * time.Second)
	span := &ssf.SSFSpan{
		ParentId:       4,
		TraceId:        6,
		Id:             0,
		StartTimestamp: start.UnixNano(),
		EndTimestamp:   end.UnixNano(),
		Service:        "test-srv",
		Name:           "test-span",
		Indicator:      true,
		Error:          true,
		Tags: map[string]string{
			"farts": "mandatory",
		},
		Metrics: []*ssf.SSFSample{
			ssf.Count("some.counter", 1, map[string]string{"purpose": "testing"}),
			ssf.Gauge("some.gauge", 20, map[string]string{"purpose": "testing"}),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		span.Id = int64(i + 1)
		err = sink.Ingest(span)
		require.NoError(b, err)
		if i != 0 && i%flushSpans == 0 {
			b.StopTimer()
			sink.Flush()
			b.StartTimer()
		}
	}
}
