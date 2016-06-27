package veneur

import (
	"testing"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), []float64{0.5, 0.99}, true)

	m := Metric{Name: "a.b.c", Value: 1.0, Digest: 12345, Type: "counter", SampleRate: 1.0}
	w.ProcessMetric(&m)

	wm := w.Flush()
	assert.Len(t, wm.counters, 1, "Number of flushed metrics")

	nometrics := w.Flush()
	assert.Len(t, nometrics.counters, 0, "Should flush no metrics")
}
