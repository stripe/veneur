package main

import (
	"testing"
	"time"
)

func TestWorker(t *testing.T) {
	w := NewWorker(1, 10, 10)

	m := Metric{Name: "a.b.c", Digest: 12345, Type: "counter"}
	w.ProcessMetric(&m)

	start := time.Now()
	ddmetrics := w.Flush(start)
	if len(ddmetrics) != 1 {
		t.Errorf("Expected (1) flushed metric, got (%d)", len(ddmetrics))
	}

	elevenSeconds, _ := time.ParseDuration("11s")
	expired := start.Add(elevenSeconds)
	nometrics := w.Flush(expired)
	if len(nometrics) != 0 {
		t.Errorf("Expected (0) flushed metric, got (%d)", len(nometrics))
	}
}
