package veneur

import (
	"encoding/binary"
	"fmt"
	"time"

	"github.com/rcrowley/go-metrics"
	"github.com/willf/bloom"
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
	Sample(int32, float32)
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
func (c *Counter) Sample(sample int32, sampleRate float32) {
	c.value += sample * int32(1/sampleRate)
	c.lastSampleTime = time.Now()
}

// Flush takes the current state of the counter, generates a
// DDMetric then clears it.
func (c *Counter) Flush() []DDMetric {
	rate := float64(c.value) / Config.Interval.Seconds()
	c.value = 0
	return []DDMetric{DDMetric{
		Name:       c.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), rate}},
		Tags:       c.tags,
		MetricType: "rate",
		Hostname:   Config.Hostname,
		Interval:   int32(Config.Interval.Seconds()),
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
func (g *Gauge) Sample(sample int32, sampleRate float32) {
	g.value = sample
	g.lastSampleTime = time.Now()
}

// Flush takes the current state of the gauge, generates a
// DDMetric then clears it.
func (g *Gauge) Flush() []DDMetric {
	v := g.value
	g.value = 0
	return []DDMetric{DDMetric{
		Name:       g.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(v)}},
		Tags:       g.tags,
		MetricType: "gauge",
		Hostname:   Config.Hostname,
	}}
}

// NewGauge genearaaaa who am I kidding just getting rid of the warning.
func NewGauge(name string, tags []string) *Gauge {
	return &Gauge{name: name, tags: tags, lastSampleTime: time.Now()}
}

// Set is a list of unique values seen.
type Set struct {
	name           string
	tags           []string
	value          int64
	filter         *bloom.BloomFilter
	lastSampleTime time.Time
}

// Sample checks if the supplied value has is already in the filter. If not, it increments
// the counter!
func (s *Set) Sample(sample int32, sampleRate float32) {
	byteSample := make([]byte, 4)
	binary.PutVarint(byteSample, int64(sample))
	if !s.filter.Test(byteSample) {
		s.filter.Add(byteSample)
		s.value++
	}
}

// NewSet generates a new Set and returns it
func NewSet(name string, tags []string, setSize uint, accuracy float64) *Set {
	return &Set{
		name:  name,
		tags:  tags,
		value: 0,
		// TODO We could likely set this based on the set size at last flush to dynamically adjust the storage?
		filter:         bloom.NewWithEstimates(setSize, accuracy),
		lastSampleTime: time.Now(),
	}
}

// Flush takes the current state of the set, generates a
// DDMetric then clears it.
func (s *Set) Flush() []DDMetric {
	v := s.value
	s.value = 0
	return []DDMetric{DDMetric{
		Name:       s.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(v)}},
		Tags:       s.tags,
		MetricType: "set",
		Hostname:   Config.Hostname,
	}}
}

// Histo is a collection of values that generates max, min, count, and
// percentiles over time.
type Histo struct {
	name           string
	tags           []string
	count          int32
	value          metrics.Histogram
	percentiles    []float64
	lastSampleTime time.Time
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample int32, sampleRate float32) {
	h.count += 1 * int32(1/sampleRate)
	h.value.Update(int64(sample))
	h.lastSampleTime = time.Now()
}

// NewHist generates a new Histo and returns it.
func NewHist(name string, tags []string, percentiles []float64) *Histo {
	return &Histo{
		name:           name,
		tags:           tags,
		count:          0,
		value:          metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015)),
		percentiles:    percentiles,
		lastSampleTime: time.Now(),
	}
}

// Flush generates DDMetrics for the current state of the
// Histo.
func (h *Histo) Flush() []DDMetric {
	now := float64(time.Now().Unix())
	rate := float64(h.count) / Config.Interval.Seconds()
	metrics := []DDMetric{
		DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       h.tags,
			MetricType: "rate",
			Hostname:   Config.Hostname,
			Interval:   int32(Config.Interval.Seconds()),
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Max())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   Config.Hostname,
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Min())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   Config.Hostname,
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
				Hostname:   Config.Hostname,
			},
		)
	}

	return metrics
}
