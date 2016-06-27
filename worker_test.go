package veneur

import (
	"testing"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, nil, logrus.New(), []float64{0.5, 0.99}, true)

	m := Metric{Name: "a.b.c", Value: 1.0, Digest: 12345, Type: "counter", SampleRate: 1.0}
	w.ProcessMetric(&m)

	ddmetrics := w.Flush(10 * time.Second)
	assert.Len(t, ddmetrics, 1, "Number of flushed metrics")

	nometrics := w.Flush(10 * time.Second)
	assert.Len(t, nometrics, 0, "Should flush no metrics")
}
