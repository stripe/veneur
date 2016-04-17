package veneur

import (
	"testing"
	"time"
)

func TestWorker(t *testing.T) {
	ReadConfig("example.yaml")
	w := NewWorker(1)

	m := Metric{Name: "a.b.c", Digest: 12345, Type: "counter"}
	w.ProcessMetric(&m)

	start := time.Now()
	ddmetrics := w.Flush(start)
	if len(ddmetrics) != 1 {
		t.Errorf("Expected (1) flushed metric, got (%d)", len(ddmetrics))
	}

	elevenSeconds := 11 * time.Second
	expired := start.Add(elevenSeconds)
	nometrics := w.Flush(expired)
	if len(nometrics) != 0 {
		t.Errorf("Expected (0) flushed metric, got (%d)", len(nometrics))
	}
}
