package veneur

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/samplers"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Value:      1.0,
		Digest:     12345,
		SampleRate: 1.0,
	}
	w.ProcessMetric(&m)

	wm := w.Flush()
	assert.Len(t, wm.counters, 1, "Number of flushed metrics")

	nometrics := w.Flush()
	assert.Len(t, nometrics.counters, 0, "Should flush no metrics")
}

func TestWorkerLocal(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Value:      1.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.LocalOnly,
	}
	w.ProcessMetric(&m)

	wm := w.Flush()
	assert.Len(t, wm.localHistograms, 1, "number of local histograms")
	assert.Len(t, wm.histograms, 0, "number of global histograms")
}

func TestWorkerGlobal(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)

	gc := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Value:      1.0,
		Digest:     12345,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}
	w.ProcessMetric(&gc)

	gg := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "b.c.a",
			Type: "gauge",
		},
		Value:      1.0,
		Digest:     12346,
		SampleRate: 1.0,
		Scope:      samplers.GlobalOnly,
	}
	w.ProcessMetric(&gg)

	assert.Equal(t, 1, len(w.wm.globalGauges), "should have 1 global gauge")
	assert.Equal(t, 0, len(w.wm.gauges), "should have no normal gauges")
	assert.Equal(t, 1, len(w.wm.globalCounters), "should have 1 global counter")
	assert.Equal(t, 0, len(w.wm.counters), "should have no local counters")
}

func TestWorkerImportSet(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)
	testset := samplers.NewSet("a.b.c", nil)
	testset.Sample("foo", 1.0)
	testset.Sample("bar", 1.0)

	jsonMetric, err := testset.Export()
	assert.NoError(t, err, "should have exported successfully")

	w.ImportMetric(jsonMetric)

	wm := w.Flush()
	assert.Len(t, wm.sets, 1, "number of flushed sets")
}

func TestWorkerImportHistogram(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)
	testhisto := samplers.NewHist("a.b.c", nil)
	testhisto.Sample(1.0, 1.0)
	testhisto.Sample(2.0, 1.0)

	jsonMetric, err := testhisto.Export()
	assert.NoError(t, err, "should have exported successfully")

	w.ImportMetric(jsonMetric)

	wm := w.Flush()
	assert.Len(t, wm.histograms, 1, "number of flushed histograms")
}

func TestWorkerStatusMetric(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), nil)

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "status",
		},
		Value:   ssf.SSFSample_CRITICAL,
		Digest:  12345,
		Message: "you've got mail!",
	}
	w.ProcessMetric(&m)

	wm := w.Flush()
	assert.Len(t, wm.localStatusChecks, 1, "Number of flushed metrics")
	var datapoint *samplers.StatusCheck
	for _, v := range wm.localStatusChecks {
		datapoint = v
		break
	}
	assert.NotNil(t, datapoint, "Expected a service check to be in the worker metrics map, but none found")

	assert.Equal(t, float64(m.Value.(ssf.SSFSample_Status)), float64(datapoint.Value), "The value of the status check should be the same value as the UDPMetric input")
	assert.Equal(t, m.Message, datapoint.Message, "The message of the status check should be the same message as the UDPMetric input")
	assert.Equal(t, m.Name, datapoint.Name, "The name of the status check should be the same name as the UDPMetric input")
	nometrics := w.Flush()
	assert.Len(t, nometrics.localStatusChecks, 0, "Should flush no metrics")
}

func TestSpanWorkerTagApplication(t *testing.T) {
	tags := map[string]func() map[string]string{
		"foo": func() map[string]string {
			return map[string]string{
				"foo": "bar",
			}
		},
		"foo2": func() map[string]string {
			return map[string]string{
				"foo": "other",
			}
		},
		"baz": func() map[string]string {
			return map[string]string{
				"baz": "qux",
			}
		},
		"both": func() map[string]string {
			return map[string]string{
				"foo": "bar",
				"baz": "qux",
			}
		},
	}

	testSpan := func(tags map[string]string) *ssf.SSFSpan {
		return &ssf.SSFSpan{
			TraceId:        1,
			ParentId:       1,
			Id:             2,
			StartTimestamp: int64(time.Now().UnixNano()),
			EndTimestamp:   int64(time.Now().UnixNano()),
			Tags:           tags,
			Error:          false,
			Service:        "farts-srv",
			Indicator:      false,
			Name:           "farting farty farts",
		}
	}

	cl, clch := newTestClient(t, 1)
	quitch := make(chan struct{})
	go func() {
		for range clch {
		}
	}()

	fake := &fakeSpanSink{wg: &sync.WaitGroup{}}
	spanChanNone := make(chan *ssf.SSFSpan)
	spanChanFoo := make(chan *ssf.SSFSpan)

	go NewSpanWorker([]sinks.SpanSink{fake}, cl, nil, spanChanNone, nil).Work()
	go NewSpanWorker([]sinks.SpanSink{fake}, cl, nil, spanChanFoo, tags["foo"]()).Work()

	sendAndWait := func(spanChan chan<- *ssf.SSFSpan, span *ssf.SSFSpan) {
		fake.wg.Add(1)
		spanChan <- span
		fake.wg.Wait()
	}

	// Don't allocate a map if there's no common tags and not tag map on the
	// span already
	sendAndWait(spanChanNone, testSpan(nil))
	require.Nil(t, fake.latestSpan().Tags)

	// Change nothing when commonTags is nil
	sendAndWait(spanChanNone, testSpan(tags["foo"]()))
	require.Equal(t, tags["foo"](), fake.latestSpan().Tags)

	// Allocate map and add tags if no map on span and there are commonTags
	sendAndWait(spanChanFoo, testSpan(nil))
	require.Equal(t, tags["foo"](), fake.latestSpan().Tags)

	// Do not override existing tags if keys match
	sendAndWait(spanChanFoo, testSpan(tags["foo2"]()))
	require.Equal(t, tags["foo2"](), fake.latestSpan().Tags)

	// Combine keys when no match
	sendAndWait(spanChanFoo, testSpan(tags["baz"]()))
	require.Equal(t, tags["both"](), fake.latestSpan().Tags)

	close(quitch)
}

type fakeSpanSink struct {
	wg    *sync.WaitGroup
	spans []*ssf.SSFSpan
}

func (s *fakeSpanSink) Start(*trace.Client) error { return nil }
func (s *fakeSpanSink) Name() string              { return "fake" }
func (s *fakeSpanSink) Flush(_ context.Context)   {}
func (s *fakeSpanSink) latestSpan() *ssf.SSFSpan  { return s.spans[len(s.spans)-1] }
func (s *fakeSpanSink) Ingest(span *ssf.SSFSpan) error {
	s.spans = append(s.spans, span)
	s.wg.Done()
	return nil
}

type testBackend struct {
	spans chan *ssf.SSFSpan
}

func (be *testBackend) Close() error {
	return nil
}

func (be *testBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	be.spans <- span
	return nil
}

func (be *testBackend) FlushSync(ctx context.Context) error {
	return nil
}

func newTestClient(t *testing.T, num int) (*trace.Client, chan *ssf.SSFSpan) {
	ch := make(chan *ssf.SSFSpan, num)
	cl, err := trace.NewBackendClient(&testBackend{ch})
	require.NoError(t, err)
	return cl, ch
}
