package veneur

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/VividCortex/gohistogram"
	"github.com/clarkduvall/hyperloglog"
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

// JSONMetric is used to represent a metric that can be remarshaled with its
// internal state intact. It is used to send metrics from one Veneur to another.
type JSONMetric struct {
	MetricKey
	Tags []string `json:"tags"`
	// the Value is an internal representation of the metric's contents, eg a
	// gob-encoded histogram or hyperloglog.
	Value []byte `json:"value"`
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
	name string
	tags []string
	hll  *hyperloglog.HyperLogLogPlus
}

// Sample checks if the supplied value has is already in the filter. If not, it increments
// the counter!
func (s *Set) Sample(sample string, sampleRate float32) {
	hasher := fnv.New64a()
	hasher.Write([]byte(sample))
	s.hll.Add(hasher)
}

// NewSet generates a new Set and returns it
func NewSet(name string, tags []string) *Set {
	// error is only returned if precision is outside the 4-18 range
	// TODO: this is the maximum precision, should it be configurable?
	hll, _ := hyperloglog.NewPlus(18)
	return &Set{
		name: name,
		tags: tags,
		hll:  hll,
	}
}

// Flush generates a DDMetric for the state of this Set.
func (s *Set) Flush() []DDMetric {
	return []DDMetric{{
		Name:       s.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(s.hll.Count())}},
		Tags:       s.tags,
		MetricType: "gauge",
	}}
}

func (s *Set) Export() (JSONMetric, error) {
	val, err := s.hll.GobEncode()
	if err != nil {
		return JSONMetric{}, err
	}
	return JSONMetric{
		MetricKey: MetricKey{
			Name:       s.name,
			Type:       "set",
			JoinedTags: strings.Join(s.tags, ","),
		},
		Tags:  s.tags,
		Value: val,
	}, nil
}

func (s *Set) Combine(other []byte) error {
	otherHLL, _ := hyperloglog.NewPlus(18)
	if err := otherHLL.GobDecode(other); err != nil {
		return err
	}
	if err := s.hll.Merge(otherHLL); err != nil {
		// does not error unless compressions are different
		// however, decoding the other hll causes us to use its compression
		// parameter, which might be different from ours
		return err
	}
	return nil
}

// Histo is a collection of values that generates max, min, count, and
// percentiles over time.
type Histo struct {
	name  string
	tags  []string
	count int64
	max   float64
	min   float64
	value gohistogram.NumericHistogram
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
func NewHist(name string, tags []string) *Histo {
	// For gohistogram, the following is from the docs:
	// There is no "optimal" bin count, but somewhere between 20 and 80 bins should be sufficient. (50!)
	return &Histo{
		name:  name,
		tags:  tags,
		value: *gohistogram.NewHistogram(50),
	}
}

// Flush generates DDMetrics for the current state of the Histo. counter indicates
// whether to include the histogram's built-in counter, and percentiles indicates
// what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64, counter bool) []DDMetric {
	now := float64(time.Now().Unix())
	rate := float64(h.count) / interval.Seconds()
	metrics := make([]DDMetric, 0, 3+len(percentiles))

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
	if counter {
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       h.tags,
			MetricType: "rate",
			Interval:   int32(interval.Seconds()),
		})
	}

	for _, p := range percentiles {
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.name, int(p*100)),
				Value:      [1][2]float64{{now, h.value.Quantile(p)}},
				Tags:       h.tags,
				MetricType: "gauge",
			},
		)
	}

	return metrics
}
