package veneur

import (
	"testing"

	"github.com/stripe/veneur/ssf"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
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
