package veneur

import (
	"fmt"
	"time"

	"github.com/VividCortex/gohistogram"
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
	Interval   int32         `json:"interval,omitempty"`
}

// Counter is an accumulator
type Counter struct {
	name  string
	tags  []string
	value int64
}

// Sample adds a sample to the counter.
func (c *Counter) Sample(sample float64, sampleRate float32) {
	c.value += int64(sample) * int64(1/sampleRate)
}

// Flush generates a DDMetric from the current state of this Counter.
func (c *Counter) Flush(interval time.Duration) []DDMetric {
	return []DDMetric{{
		Name:       c.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(c.value) / interval.Seconds()}},
		Tags:       c.tags,
		MetricType: "rate",
		Interval:   int32(interval.Seconds()),
	}}
}

// NewCounter generates and returns a new Counter.
func NewCounter(name string, tags []string) *Counter {
	return &Counter{name: name, tags: tags}
}

// Gauge retains whatever the last value was.
type Gauge struct {
	name  string
	tags  []string
	value float64
}

// Sample takes on whatever value is passed in as a sample.
func (g *Gauge) Sample(sample float64, sampleRate float32) {
	g.value = sample
}

// Flush generates a DDMetric from the current state of this gauge.
func (g *Gauge) Flush() []DDMetric {
	return []DDMetric{{
		Name:       g.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(g.value)}},
		Tags:       g.tags,
		MetricType: "gauge",
	}}
}

// NewGauge genearaaaa who am I kidding just getting rid of the warning.
func NewGauge(name string, tags []string) *Gauge {
	return &Gauge{name: name, tags: tags}
}

// Set is a list of unique values seen.
type Set struct {
	name   string
	tags   []string
	value  float64
	filter *bloom.BloomFilter
}

// Sample checks if the supplied value has is already in the filter. If not, it increments
// the counter!
func (s *Set) Sample(sample string, sampleRate float32) {
	if !s.filter.TestAndAddString(sample) {
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
		filter: bloom.NewWithEstimates(setSize, accuracy),
	}
}

// Flush generates a DDMetric for the state of this Set.
func (s *Set) Flush() []DDMetric {
	return []DDMetric{{
		Name:       s.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(s.value)}},
		Tags:       s.tags,
		MetricType: "gauge",
	}}
}

// Histo is a collection of values that generates max, min, count, and
// percentiles over time.
type Histo struct {
	name        string
	tags        []string
	count       int64
	max         float64
	min         float64
	value       gohistogram.NumericHistogram
	percentiles []float64
	counter     bool
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample float64, sampleRate float32) {
	if h.count == 0 {
		h.max = sample
		h.min = sample
	} else {
		if h.max < sample {
			h.max = sample
		}
		if h.min > sample {
			h.min = sample
		}
	}
	h.count += 1 * int64(1/sampleRate)
	h.value.Add(sample)
}

// NewHist generates a new Histo and returns it.
func NewHist(name string, tags []string, percentiles []float64, counter bool) *Histo {
	// For gohistogram, the following is from the docs:
	// There is no "optimal" bin count, but somewhere between 20 and 80 bins should be sufficient. (50!)
	return &Histo{
		name:        name,
		tags:        tags,
		count:       0,
		value:       *gohistogram.NewHistogram(50),
		percentiles: percentiles,
		counter:     counter,
	}
}

// Flush generates DDMetrics for the current state of the Histo.
func (h *Histo) Flush(interval time.Duration) []DDMetric {
	now := float64(time.Now().Unix())
	rate := float64(h.count) / interval.Seconds()
	metrics := make([]DDMetric, 0, 3+len(h.percentiles))

	metrics = append(metrics,
		DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, h.max}},
			Tags:       h.tags,
			MetricType: "gauge",
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, h.min}},
			Tags:       h.tags,
			MetricType: "gauge",
		},
	)
	if h.counter {
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       h.tags,
			MetricType: "rate",
			Interval:   int32(interval.Seconds()),
		})
	}

	for i, p := range h.percentiles {
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.name, int(h.percentiles[i]*100)),
				Value:      [1][2]float64{{now, h.value.Quantile(p)}},
				Tags:       h.tags,
				MetricType: "gauge",
			},
		)
	}

	return metrics
}
