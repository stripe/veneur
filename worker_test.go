package veneur

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/samplers/metricpb"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, logrus.New())

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
	w := NewWorker(1, nil, logrus.New())

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
	w := NewWorker(1, nil, logrus.New())

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
	w := NewWorker(1, nil, logrus.New())
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
	w := NewWorker(1, nil, logrus.New())
	testhisto := samplers.NewHist("a.b.c", nil)
	testhisto.Sample(1.0, 1.0)
	testhisto.Sample(2.0, 1.0)

	jsonMetric, err := testhisto.Export()
	assert.NoError(t, err, "should have exported successfully")

	w.ImportMetric(jsonMetric)

	wm := w.Flush()
	assert.Len(t, wm.histograms, 1, "number of flushed histograms")
}

func exportMetricAndFlush(t testing.TB, exp metricExporter) WorkerMetrics {
	w := NewWorker(1, nil, logrus.New())
	m, err := exp.Metric()
	assert.NoErrorf(t, err, "exporting the metric '%s' shouldn't have failed",
		exp.GetName())

	assert.NoError(t, w.ImportMetricGRPC(m), "importing a metric shouldn't "+
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
		w := NewWorker(1, nil, logrus.New())
		h := samplers.NewHist("test.timer", nil)
		h.Sample(1.0, 1.0)

		m, err := h.Metric()
		assert.NoErrorf(t, err, "exporting the histogram shouldn't have failed")
		m.Type = metricpb.Type_Timer

		assert.NoError(t, w.ImportMetricGRPC(m), "importing a timer shouldn't "+
			"have failed")
		assert.Len(t, w.Flush().timers, 1, "The number of flushed "+
			"timers is not correct")
	})
	t.Run("set", func(t *testing.T) {
		t.Parallel()
		s := samplers.NewSet("test.set", nil)
		s.Sample("value", 1.0)

		assert.Len(t, exportMetricAndFlush(t, s).sets, 1,
			"The number of flushed sets is not correct")
	})
}

func TestWorkerImportMetricGRPCNilValue(t *testing.T) {
	t.Parallel()

	w := NewWorker(1, nil, logrus.New())
	metric := &metricpb.Metric{
		Name:  "test",
		Type:  metricpb.Type_Histogram,
		Value: nil,
	}

	assert.Error(t, w.ImportMetricGRPC(metric), "Importing a metric with "+
		"a nil value should have failed")
}
