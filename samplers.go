package veneur

import (
	"fmt"
	"time"

	"github.com/rcrowley/go-metrics"
)

// DDMetric is a data structure that represents the JSON that Datadog
// wants when posting to the API
type DDMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float64 `json:"points"`
	Tags       []string      `json:"tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host"`
	// devicename
	Interval int32 `json:"interval,omitempty"`
}

// Sampler is a thing that takes samples and does something with them
// to be flushed later.
type Sampler interface {
	Sample(int32)
	Flush() []DDMetric
}

// Counter is an accumulator
type Counter struct {
	name           string
	tags           []string
	value          int32
	lastSampleTime time.Time
}

// Sample adds a sample to the counter.
func (c *Counter) Sample(sample int32) {
	c.value += sample
	c.lastSampleTime = time.Now()
}

// Flush takes the current state of the counter, generates a
// DDMetric then clears it.
func (c *Counter) Flush(interval time.Duration, hostname string) []DDMetric {
	rate := float64(c.value) / interval.Seconds()
	c.value = 0
	return []DDMetric{DDMetric{
		Name:       c.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), rate}},
		Tags:       c.tags,
		MetricType: "rate",
		Hostname:   hostname,
		Interval:   int32(interval.Seconds()),
	}}
}

// NewCounter generates and returns a new Counter.
func NewCounter(name string, tags []string) *Counter {
	return &Counter{name: name, tags: tags, lastSampleTime: time.Now()}
}

// Gauge retains whatever the last value was.
type Gauge struct {
	name           string
	tags           []string
	value          int32
	lastSampleTime time.Time
}

// Sample takes on whatever value is passed in as a sample.
func (g *Gauge) Sample(sample int32) {
	g.value = sample
	g.lastSampleTime = time.Now()
}

// Flush takes the current state of the gauge, generates a
// DDMetric then clears it.
func (g *Gauge) Flush(interval time.Duration, hostname string) []DDMetric {
	v := g.value
	g.value = 0
	return []DDMetric{DDMetric{
		Name:       g.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(v)}},
		Tags:       g.tags,
		MetricType: "gauge",
		Hostname:   hostname,
	}}
}

// NewGauge genearaaaa who am I kidding just getting rid of the warning.
func NewGauge(name string, tags []string) *Gauge {
	return &Gauge{name: name, tags: tags, lastSampleTime: time.Now()}
}

// Histo is a collection of values that generates max, min, count, and
// percentiles over time.
type Histo struct {
	name           string
	tags           []string
	value          metrics.Histogram
	percentiles    []float64
	lastSampleTime time.Time
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample int32) {
	h.value.Update(int64(sample))
	h.lastSampleTime = time.Now()
}

// NewHist generates a new Histo and returns it.
func NewHist(name string, tags []string, percentiles []float64) *Histo {
	return &Histo{
		name:           name,
		tags:           tags,
		value:          metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015)),
		percentiles:    percentiles,
		lastSampleTime: time.Now(),
	}
}

// Flush generates DDMetrics for the current state of the
// Histo.
func (h *Histo) Flush(interval time.Duration, hostname string) []DDMetric {
	now := float64(time.Now().Unix())
	rate := float64(h.value.Count()) / interval.Seconds()
	metrics := []DDMetric{
		DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       h.tags,
			MetricType: "rate",
			Hostname:   hostname,
			Interval:   int32(interval.Seconds()),
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Max())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   hostname,
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Min())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   hostname,
		},
	}

	percentiles := h.value.Percentiles(h.percentiles)
	for i, p := range percentiles {
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.name, int(h.percentiles[i]*100)),
				Value:      [1][2]float64{{now, float64(p)}},
				Tags:       h.tags,
				MetricType: "gauge",
				Hostname:   hostname,
			},
		)
	}

	return metrics
}
