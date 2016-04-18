package veneur

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWorker(t *testing.T) {
	ReadConfig("example.yaml")
	w := NewWorker(1)

	m := Metric{Name: "a.b.c", Value: 1, Digest: 12345, Type: "counter", SampleRate: 1.0}
	w.ProcessMetric(&m)

	start := time.Now()
	ddmetrics := w.Flush(start)
	assert.Len(t, ddmetrics, 1, "Number of flushed metrics")

	elevenSeconds := 11 * time.Second
	expired := start.Add(elevenSeconds)
	nometrics := w.Flush(expired)
	assert.Len(t, nometrics, 0, "Should flush no metrics")
}
