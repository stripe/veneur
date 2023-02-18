package veneur

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/testbackend"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})

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
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})

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
	w := NewWorker(1, WorkerOpts{IsLocal: false, Logger: logrus.New()})

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

func TestWorkerStatusMetric(t *testing.T) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})

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

	logger := logrus.NewEntry(logrus.New())
	go NewSpanWorker(
		[]sinks.SpanSink{fake}, cl, nil, spanChanNone, logger).Work()
	go NewSpanWorker(
		[]sinks.SpanSink{fake}, cl, nil, spanChanFoo, logger).Work()

	sendAndWait := func(spanChan chan<- *ssf.SSFSpan, span *ssf.SSFSpan) {
		fake.wg.Add(1)
		spanChan <- span
		fake.wg.Wait()
	}

	// Don't allocate a map if there's no common tags and not tag map on the
	// span already
	sendAndWait(spanChanNone, testSpan(nil))
	require.Nil(t, fake.latestSpan().Tags)

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
func (s *fakeSpanSink) Flush()                    {}
func (s *fakeSpanSink) latestSpan() *ssf.SSFSpan  { return s.spans[len(s.spans)-1] }
func (s *fakeSpanSink) Ingest(span *ssf.SSFSpan) error {
	s.spans = append(s.spans, span)
	s.wg.Done()
	return nil
}

func newTestClient(t *testing.T, num int) (*trace.Client, chan *ssf.SSFSpan) {
	ch := make(chan *ssf.SSFSpan, num)
	cl, err := trace.NewBackendClient(testbackend.NewBackend(ch))
	require.NoError(t, err)
	return cl, ch
}

type testMetricExporter interface {
	GetName() string
	Metric() (*metricpb.Metric, error)
}

func exportMetricAndFlush(t testing.TB, exp testMetricExporter) WorkerMetrics {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})
	m, err := exp.Metric()
	assert.NoErrorf(t, err, "exporting the metric '%s' shouldn't have failed",
		exp.GetName())

	assert.NoError(t, w.ImportMetric(m), "importing a metric shouldn't "+
		"have failed")
	return w.Flush()
}

func TestWorkerImportMetricGRPC(t *testing.T) {
	t.Run("histogram", func(t *testing.T) {
		t.Parallel()
		h := samplers.NewHist("test.histo", nil)
		h.Sample(1.0, 1.0)

		assert.Len(t, exportMetricAndFlush(t, h).histograms, 1,
			"The number of flushed histograms is not correct")
	})
	t.Run("gauge", func(t *testing.T) {
		t.Parallel()
		g := samplers.NewGauge("test.gauge", nil)
		g.Sample(2.0, 1.0)

		assert.Len(t, exportMetricAndFlush(t, g).globalGauges, 1,
			"The number of flushed gauges is not correct")
	})
	t.Run("counter", func(t *testing.T) {
		t.Parallel()
		c := samplers.NewCounter("test.counter", nil)
		c.Sample(2.0, 1.0)

		assert.Len(t, exportMetricAndFlush(t, c).globalCounters, 1,
			"The number of flushed counters is not correct")
	})
	t.Run("timer", func(t *testing.T) {
		t.Parallel()
		w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})
		h := samplers.NewHist("test.timer", nil)
		h.Sample(1.0, 1.0)

		m, err := h.Metric()
		assert.NoErrorf(t, err, "exporting the histogram shouldn't have failed")
		m.Type = metricpb.Type_Timer

		assert.NoError(t, w.ImportMetric(m), "importing a timer shouldn't "+
			"have failed")
		assert.Len(t, w.Flush().timers, 1, "The number of flushed "+
			"timers is not correct")
	})
	t.Run("set", func(t *testing.T) {
		t.Parallel()
		s := samplers.NewSet("test.set", nil)
		s.Sample("value")

		assert.Len(t, exportMetricAndFlush(t, s).sets, 1,
			"The number of flushed sets is not correct")
	})
}

func TestWorkerImportMetricGRPCNilValue(t *testing.T) {
	t.Parallel()

	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})
	metric := &metricpb.Metric{
		Name:  "test",
		Type:  metricpb.Type_Histogram,
		Value: nil,
	}

	assert.Error(t, w.ImportMetric(metric), "Importing a metric with "+
		"a nil value should have failed")
}

// Test that (WorkerMetrics).ForwardableMetrics produces the right output with
// a variety of inputs.
func TestWorkerMetricsForwardableMetrics(t *testing.T) {
	t.Parallel()

	type testMetric struct {
		name  string
		scope samplers.MetricScope
		mType string
	}

	testCases := []struct {
		name     string
		inputs   []testMetric
		expected []testMetric
	}{
		{
			name: "no global metrics",
			inputs: []testMetric{
				testMetric{
					name:  "test.gauge",
					scope: samplers.MixedScope,
					mType: GaugeTypeName,
				},
				testMetric{
					name:  "test.counter",
					scope: samplers.LocalOnly,
					mType: CounterTypeName,
				},
			},
			expected: []testMetric{},
		},
		{
			name: "some global metrics",
			inputs: []testMetric{
				testMetric{
					name:  "test.gauge",
					scope: samplers.MixedScope,
					mType: GaugeTypeName,
				},
				testMetric{
					name:  "test.mixed.histo",
					scope: samplers.MixedScope,
					mType: HistogramTypeName,
				},
			},
			expected: []testMetric{
				testMetric{
					name:  "test.mixed.histo",
					scope: samplers.MixedScope,
					mType: HistogramTypeName,
				},
			},
		},
		{
			name:     "no metrics",
			inputs:   []testMetric{},
			expected: []testMetric{},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			wm := NewWorkerMetrics()

			// Add all of the test metrics
			for _, m := range tc.inputs {
				mk := samplers.MetricKey{Name: m.name, Type: m.mType}
				wm.Upsert(mk, m.scope, []string{})
			}

			logger := logrus.NewEntry(logrus.New())
			ms := wm.ForwardableMetrics(nil, logger)

			// Convert all of the forwardable metrics into testMetric's
			// and then compare them
			actual := make([]testMetric, len(ms))
			for i, m := range ms {
				actual[i] = testMetric{
					name:  m.GetName(),
					mType: strings.ToLower(m.GetType().String()),
				}
			}

			assert.ElementsMatch(t, tc.expected, actual,
				"The output of ForwardableMetrics doesn't have the right metrics")
		})
	}
}

func TestLocalWorkerSampleTimeseries(t *testing.T) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, CountUniqueTimeseries: true, Logger: logrus.New()})

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Digest: 1,
		Scope:  samplers.LocalOnly,
	}
	m2 := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Digest: 2,
		Scope:  samplers.MixedScope,
	}
	w.SampleTimeseries(&m)
	assert.Equal(t, uint64(1), w.uniqueMTS.Estimate())
	w.SampleTimeseries(&m)
	assert.Equal(t, uint64(1), w.uniqueMTS.Estimate())
	w.SampleTimeseries(&m2)
	assert.Equal(t, uint64(2), w.uniqueMTS.Estimate())
}

func TestLocalWorkerSampleForwardedTimeseries(t *testing.T) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, CountUniqueTimeseries: true, Logger: logrus.New()})

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Digest: 1,
		Scope:  samplers.MixedScope,
	}
	m2 := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Digest: 2,
		Scope:  samplers.GlobalOnly,
	}
	w.SampleTimeseries(&m)
	w.SampleTimeseries(&m2)
	assert.Equal(t, uint64(0), w.uniqueMTS.Estimate())
}

func TestGlobalWorkerSampleTimeseries(t *testing.T) {
	w := NewWorker(1, WorkerOpts{IsLocal: false, CountUniqueTimeseries: true, Logger: logrus.New()})

	m := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "histogram",
		},
		Digest: 1,
		Scope:  samplers.LocalOnly,
	}
	m2 := samplers.UDPMetric{
		MetricKey: samplers.MetricKey{
			Name: "a.b.c",
			Type: "counter",
		},
		Digest: 2,
		Scope:  samplers.GlobalOnly,
	}
	w.SampleTimeseries(&m)
	w.SampleTimeseries(&m2)
	assert.Equal(t, uint64(2), w.uniqueMTS.Estimate())
}

func BenchmarkWork(b *testing.B) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})

	const Len = 1000
	input := make([]*samplers.UDPMetric, Len)
	for i, _ := range input {
		m := samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "counter",
				Type: CounterTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		}

		switch r := i % 5; r {
		case 1:
			m.MetricKey.Type = GaugeTypeName
		case 2:
			m.MetricKey.Type = HistogramTypeName
		case 3:
			m.MetricKey.Type = SetTypeName
			m.Value = "a value here!"
		case 4:
			m.MetricKey.Type = TimerTypeName
		default:
			// do nothing
		}

		input[i] = &m
	}

	go w.Work()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.PacketChan <- *input[i%Len]
	}
	w.Stop()
}

func BenchmarkWorkWithCountUniqueTimeseries(b *testing.B) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})

	const Len = 1000
	input := make([]*samplers.UDPMetric, Len)
	for i, _ := range input {
		m := samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "counter",
				Type: CounterTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		}

		switch r := i % 5; r {
		case 1:
			m.MetricKey.Type = GaugeTypeName
		case 2:
			m.MetricKey.Type = HistogramTypeName
		case 3:
			m.MetricKey.Type = SetTypeName
			m.Value = "a value here!"
		case 4:
			m.MetricKey.Type = TimerTypeName
		default:
			// do nothing
		}

		input[i] = &m
	}

	go w.Work()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.PacketChan <- *input[i%Len]
	}
	w.Stop()
}

func BenchmarkSampleTimeseries(b *testing.B) {
	w := NewWorker(1, WorkerOpts{IsLocal: true, Logger: logrus.New()})
	const Len = 1000
	input := make([]*samplers.UDPMetric, Len)
	for i, _ := range input {
		m := samplers.UDPMetric{
			MetricKey: samplers.MetricKey{
				Name: "counter",
				Type: CounterTypeName,
			},
			Value:      20.0,
			Digest:     12345,
			SampleRate: 1.0,
			Scope:      samplers.MixedScope,
		}

		switch r := i % 5; r {
		case 1:
			m.MetricKey.Type = GaugeTypeName
		case 2:
			m.MetricKey.Type = HistogramTypeName
		case 3:
			m.MetricKey.Type = SetTypeName
			m.Value = "a value here!"
		case 4:
			m.MetricKey.Type = TimerTypeName
		default:
			// do nothing
		}

		input[i] = &m
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.SampleTimeseries(input[i%Len])
	}
}
