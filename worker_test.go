package veneur

import (
	"math/rand"
	"strconv"
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/samplers"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, log)

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

func TestWorkerImportSet(t *testing.T) {
	w := NewWorker(1, nil, log)
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

func TestWorkerImportCardinality(t *testing.T) {
	w := NewWorker(1, nil, log)

	cc := samplers.NewCardinalityCount()
	for i := 0; i < 100; i++ {
		cc.Sample("abc", strconv.Itoa(rand.Int()))
	}
	for i := 0; i < 150; i++ {
		cc.Sample("cde", strconv.Itoa(rand.Int()))
	}

	jsonMetrics, err := cc.ExportSets()
	assert.NoError(t, err, "should have exported successfully")

	for _, jm := range jsonMetrics {
		w.ImportMetric(jm)
	}

	wm := w.Flush()
	assert.Len(t, wm.cardinality.MetricCardinality, 2, "number of flushed outputs")
}
