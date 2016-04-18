package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounterEmpty(t *testing.T) {

	c := NewCounter("a.b.c", []string{"a:b"})
	c.Sample(1, 1.0)

	assert.Equal(t, "a.b.c", c.name, "Name")
	assert.Len(t, c.tags, 1, "Tag length")
	assert.Equal(t, c.tags[0], "a:b", "Tag contents")

	metrics := c.Flush()
	assert.Len(t, metrics, 1, "Flushes 1 metric")

	m1 := metrics[0]
	assert.Equal(t, int32(10), m1.Interval, "Interval")
	assert.Equal(t, "rate", m1.MetricType, "Type")
	assert.Len(t, c.tags, 1, "Tag length")
	assert.Equal(t, c.tags[0], "a:b", "Tag contents")
	// The counter returns an array with a single tuple of timestamp,value
	assert.Equal(t, 0.1, m1.Value[0][1], "Metric value")
}

func TestCounterRate(t *testing.T) {

	c := NewCounter("a.b.c", []string{"a:b"})

	c.Sample(5, 1.0)

	// The counter returns an array with a single tuple of timestamp,value
	metrics := c.Flush()
	assert.Equal(t, 0.5, metrics[0].Value[0][1], "Metric value")
}

func TestCounterSampleRate(t *testing.T) {

	c := NewCounter("a.b.c", []string{"a:b"})

	c.Sample(5, 0.5)

	// The counter returns an array with a single tuple of timestamp,value
	metrics := c.Flush()
	assert.Equal(t, float64(1), metrics[0].Value[0][1], "Metric value")
}

func TestGauge(t *testing.T) {

	g := NewGauge("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", g.name, "Name")
	assert.Len(t, g.tags, 1, "Tag length")
	assert.Equal(t, g.tags[0], "a:b", "Tag contents")

	g.Sample(5, 1.0)

	metrics := g.Flush()
	assert.Len(t, metrics, 1, "Flushed metric count")

	m1 := metrics[0]
	// Interval is not meaningful for this
	assert.Equal(t, int32(0), m1.Interval, "Interval")
	assert.Equal(t, "gauge", m1.MetricType, "Type")
	tags := m1.Tags
	assert.Len(t, tags, 1, "Tag length")
	assert.Equal(t, tags[0], "a:b", "Tag contents")

	// The counter returns an array with a single tuple of timestamp,value
	assert.Equal(t, float64(5), m1.Value[0][1], "Value")
}

func TestSet(t *testing.T) {

	s := NewSet("a.b.c", []string{"a:b"}, 1000, 0.99)

	assert.Equal(t, "a.b.c", s.name, "Name")
	assert.Len(t, s.tags, 1, "Tag count")
	assert.Equal(t, "a:b", s.tags[0], "First tag")

	s.Sample(5, 1.0)

	s.Sample(5, 1.0)

	s.Sample(123, 1.0)

	metrics := s.Flush()
	assert.Len(t, metrics, 1, "Flush")

	m1 := metrics[0]
	// Interval is not meaningful for this
	assert.Equal(t, int32(0), m1.Interval, "Interval")
	assert.Equal(t, "set", m1.MetricType, "Type")
	assert.Len(t, m1.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m1.Tags[0], "First tag")
	assert.Equal(t, float64(2), m1.Value[0][1], "Value")
}

func TestHisto(t *testing.T) {

	h := NewHist("a.b.c", []string{"a:b"}, []float64{0.50})

	assert.Equal(t, "a.b.c", h.name, "Name")
	assert.Len(t, h.tags, 1, "Tag count")
	assert.Equal(t, "a:b", h.tags[0], "First tag")

	h.Sample(5, 1.0)
	h.Sample(10, 1.0)
	h.Sample(15, 1.0)
	h.Sample(20, 1.0)
	h.Sample(25, 1.0)

	metrics := h.Flush()
	// We get lots of metrics back for histograms!
	assert.Len(t, metrics, 4, "Flushed metrics length")

	// First the count
	m1 := metrics[0]
	assert.Equal(t, "a.b.c.count", m1.Name, "Name")
	assert.Equal(t, int32(10), m1.Interval, "Interval")
	assert.Equal(t, "rate", m1.MetricType, "Type")
	tags := m1.Tags
	assert.Len(t, tags, 1, "Tag count")
	assert.Equal(t, "a:b", tags[0], "First tag")

	// The counter returns an array with a single tuple of timestamp,value
	assert.Equal(t, float64(0.5), m1.Value[0][1], "Value")

	// Now the max
	m2 := metrics[1]
	if m2.Name != "a.b.c.max" {
		t.Errorf("Expected interval, wanted (a.b.c.max) got (%s)", m2.Name)
	}
	if m2.Interval != 0 {
		t.Errorf("Expected interval, wanted (0) got (%d)", m2.Interval)
	}
	if m2.MetricType != "gauge" {
		t.Errorf("Expected metric type, wanted (gauge) got (%s)", m2.MetricType)
	}
	if len(m2.Tags) != 1 && m2.Tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m2.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m2.Value[0][1] != 25 {
		t.Errorf("Expected value, wanted (1) got (%f)", m2.Value[0][1])
	}

	// Now the min
	m3 := metrics[2]
	if m3.Name != "a.b.c.min" {
		t.Errorf("Expected interval, wanted (a.b.c.min) got (%s)", m3.Name)
	}
	if m3.Interval != 0 {
		t.Errorf("Expected interval, wanted (0) got (%d)", m3.Interval)
	}
	if m3.MetricType != "gauge" {
		t.Errorf("Expected metric type, wanted (gauge) got (%s)", m3.MetricType)
	}
	if len(m3.Tags) != 1 && m3.Tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m3.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m3.Value[0][1] != 5 {
		t.Errorf("Expected value, wanted (2) got (%f)", m3.Value[0][1])
	}

	// And the percentile
	m4 := metrics[3]
	if m4.Name != "a.b.c.50percentile" {
		t.Errorf("Expected interval, wanted (a.b.c.50percentile) got (%s)", m4.Name)
	}
	if m4.Interval != 0 {
		t.Errorf("Expected interval, wanted (0) got (%d)", m4.Interval)
	}
	if m4.MetricType != "gauge" {
		t.Errorf("Expected metric type, wanted (gauge) got (%s)", m4.MetricType)
	}
	if len(m4.Tags) != 1 && m4.Tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m4.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m4.Value[0][1] != 15 {
		t.Errorf("Expected value, wanted (15) got (%f)", m4.Value[0][1])
	}
}

func TestHistoSampleRate(t *testing.T) {

	h := NewHist("a.b.c", []string{"a:b"}, []float64{0.50})

	assert.Equal(t, "a.b.c", h.name, "Name")
	assert.Len(t, h.tags, 1, "Tag length")
	assert.Equal(t, h.tags[0], "a:b", "Tag contents")

	h.Sample(5, 0.5)
	h.Sample(10, 0.5)
	h.Sample(15, 0.5)
	h.Sample(20, 0.5)
	h.Sample(25, 0.5)

	metrics := h.Flush()
	assert.Len(t, metrics, 4, "Metrics flush length")

	// First the count
	m1 := metrics[0]
	assert.Equal(t, "a.b.c.count", m1.Name, "Count name")
	assert.Equal(t, float64(1), m1.Value[0][1], "Sampled count as rate")
}
