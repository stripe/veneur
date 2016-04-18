package veneur

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCounterEmpty(t *testing.T) {

	c := NewCounter("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", c.name, "Name")
	if len(c.tags) != 1 && c.tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", c.tags)
	}

	metrics := c.Flush()
	if len(metrics) != 1 {
		t.Errorf("Expected 1 DDMetric, got (%d)", len(metrics))
	}

	m1 := metrics[0]
	if m1.Interval != 10 {
		t.Errorf("Expected interval, wanted (10) got (%d)", m1.Interval)
	}
	if m1.MetricType != "rate" {
		t.Errorf("Expected metric type, wanted (rate) got (%s)", m1.MetricType)
	}
	tags := m1.Tags
	if len(tags) != 1 && tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m1.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m1.Value[0][1] != 0 {
		t.Errorf("Expected value, wanted (0) got (%f)", m1.Value[0][1])
	}
}

func TestCounterRate(t *testing.T) {

	c := NewCounter("a.b.c", []string{"a:b"})

	c.Sample(5)

	// The counter returns an array with a single tuple of timestamp,value
	metrics := c.Flush()
	if metrics[0].Value[0][1] != 0.5 {
		t.Errorf("Expected value, wanted (0.5) got (%f)", metrics[0].Value[0][1])
	}
}

func TestGauge(t *testing.T) {

	g := NewGauge("a.b.c", []string{"a:b"})

	if g.name != "a.b.c" {
		t.Errorf("Expected name, wanted (a.b.c) got (%s)", g.name)
	}
	if len(g.tags) != 1 && g.tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", g.tags)
	}

	g.Sample(5)

	metrics := g.Flush()
	if len(metrics) != 1 {
		t.Errorf("Expected 1 DDMetric, got (%d)", len(metrics))
	}

	m1 := metrics[0]
	// Interval is not meaningful for this
	if m1.Interval != 0 {
		t.Errorf("Expected interval, wanted (0) got (%d)", m1.Interval)
	}
	if m1.MetricType != "gauge" {
		t.Errorf("Expected metric type, wanted (gauge) got (%s)", m1.MetricType)
	}
	tags := m1.Tags
	if len(tags) != 1 && tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m1.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m1.Value[0][1] != 5 {
		t.Errorf("Expected value, wanted (5) got (%f)", m1.Value[0][1])
	}
}

func TestSet(t *testing.T) {

	s := NewSet("a.b.c", []string{"a:b"}, 1000, 0.99)

	assert.Equal(t, "a.b.c", s.name, "Name")
	assert.Len(t, s.tags, 1, "Tag count")
	assert.Equal(t, "a:b", s.tags[0], "First tag")

	s.Sample(5)

	s.Sample(5)

	s.Sample(123)

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

	if h.name != "a.b.c" {
		t.Errorf("Expected name, wanted (a.b.c) got (%s)", h.name)
	}
	if len(h.tags) != 1 && h.tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", h.tags)
	}

	h.Sample(5)
	h.Sample(10)
	h.Sample(15)
	h.Sample(20)
	h.Sample(25)

	metrics := h.Flush()
	// We get lots of metrics back for histograms!
	if len(metrics) != 4 {
		t.Errorf("Expected 4 DDMetrics, got (%d)", len(metrics))
	}

	// First the count
	m1 := metrics[0]
	if m1.Name != "a.b.c.count" {
		t.Errorf("Expected interval, wanted (a.b.c.count) got (%s)", m1.Name)
	}
	if m1.Interval != 10 {
		t.Errorf("Expected interval, wanted (10) got (%d)", m1.Interval)
	}
	if m1.MetricType != "rate" {
		t.Errorf("Expected metric type, wanted (rate) got (%s)", m1.MetricType)
	}
	tags := m1.Tags
	if len(tags) != 1 && tags[0] != "a:b" {
		t.Errorf("Expected tags, wanted ([\"a:b\"]) got (%v)", m1.Tags)
	}
	// The counter returns an array with a single tuple of timestamp,value
	if m1.Value[0][1] != 0.5 {
		t.Errorf("Expected value, wanted (0.5) got (%f)", m1.Value[0][1])
	}

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
