package main

import (
	"fmt"
	"time"

	"github.com/rcrowley/go-metrics"
)

type DDMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float64 `json:"points"`
	Tags       []string      `json:"tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host"`
	// devicename
	Interval int32 `json:"interval,omitempty"`
}

type Sampler interface {
	Sample(int32)
	Flush() []DDMetric
}

type Counter struct {
	name           string
	tags           []string
	value          int32
	lastSampleTime time.Time
}

func (c *Counter) Sample(sample int32) {
	c.value += sample
	c.lastSampleTime = time.Now()
}

func (c *Counter) Flush(interval int32) []DDMetric {
	rate := float64(c.value) / float64(interval)
	c.value = 0
	return []DDMetric{DDMetric{
		Name:       c.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), rate}},
		Tags:       c.tags,
		MetricType: "rate",
		Hostname:   "testhost", // TODO Stop hardcoding this
		Interval:   interval,
	}}
}

func NewCounter(name string, tags []string) *Counter {
	return &Counter{name: name, tags: tags, lastSampleTime: time.Now()}
}

type Gauge struct {
	name           string
	tags           []string
	value          int32
	lastSampleTime time.Time
}

func (g *Gauge) Sample(sample int32) {
	g.value = sample
	g.lastSampleTime = time.Now()
}

func (g *Gauge) Flush(interval int32) []DDMetric {
	v := g.value
	g.value = 0
	return []DDMetric{DDMetric{
		Name:       g.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(v)}},
		Tags:       g.tags,
		MetricType: "gauge",
		Hostname:   "testhost", // TODO Stop hardcoding this
	}}
}

func NewGauge(name string, tags []string) *Gauge {
	return &Gauge{name: name, tags: tags, lastSampleTime: time.Now()}
}

type Histo struct {
	name           string
	tags           []string
	value          metrics.Histogram
	percentiles    []float64
	lastSampleTime time.Time
}

func (h *Histo) Sample(sample int32) {
	h.value.Update(int64(sample))
	h.lastSampleTime = time.Now()
}

func NewHist(name string, tags []string, percentiles []float64) *Histo {
	return &Histo{
		name:           name,
		tags:           tags,
		value:          metrics.NewHistogram(metrics.NewExpDecaySample(1028, 0.015)),
		percentiles:    percentiles,
		lastSampleTime: time.Now(),
	}
}

func (h *Histo) Flush(interval int32) []DDMetric {
	now := float64(time.Now().Unix())
	rate := float64(h.value.Count()) / float64(interval)
	metrics := []DDMetric{
		DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       h.tags,
			MetricType: "rate",
			Hostname:   "testhost", // TODO Stop hardcoding this
			Interval:   interval,
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Max())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   "testhost", // TODO Stop hardcoding this
		},
		DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, float64(h.value.Min())}},
			Tags:       h.tags,
			MetricType: "gauge",
			Hostname:   "testhost", // TODO Stop hardcoding this
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
				Hostname:   "testhost", // TODO Stop hardcoding this
			},
		)
	}

	return metrics
}
