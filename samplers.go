package veneur

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"github.com/clarkduvall/hyperloglog"
	"github.com/stripe/veneur/tdigest"
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
	value *tdigest.MergingDigest
	// these values are computed from only the samples that came through this
	// veneur instance, ignoring any histograms merged from elsewhere
	// we separate them because they're easy to aggregate on the backend without
	// loss of granularity, and having host-local information on them might be
	// useful
	localWeight float64
	localMin    float64
	localMax    float64
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample float64, sampleRate float32) {
	weight := float64(1 / sampleRate)
	h.value.Add(sample, weight)

	h.localWeight += weight
	h.localMin = math.Min(h.localMin, sample)
	h.localMax = math.Max(h.localMax, sample)
}

// NewHist generates a new Histo and returns it.
func NewHist(name string, tags []string) *Histo {
	return &Histo{
		name: name,
		tags: tags,
		// we're going to allocate a lot of these, so we don't want them to be huge
		value:    tdigest.NewMerging(100, false),
		localMin: math.Inf(+1),
		localMax: math.Inf(-1),
	}
}

// Flush generates DDMetrics for the current state of the Histo. percentiles
// indicates what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64) []DDMetric {
	now := float64(time.Now().Unix())
	// we only want to flush the number of samples we received locally, since
	// any other samples have already been flushed by a local veneur instance
	// before this was forwarded to us
	rate := h.localWeight / interval.Seconds()
	metrics := make([]DDMetric, 0, 3+len(percentiles))

	if !math.IsInf(h.localMax, 0) {
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, h.localMax}},
			Tags:       h.tags,
			MetricType: "gauge",
		})
	}
	if !math.IsInf(h.localMin, 0) {
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, h.localMin}},
			Tags:       h.tags,
			MetricType: "gauge",
		})
	}
	if rate != 0 {
		// if we haven't received any local samples, then leave this sparse,
		// otherwise it can lead to some misleading zeroes in between the
		// flushes of downstream instances
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

func (h *Histo) Export() (JSONMetric, error) {
	val, err := h.value.GobEncode()
	if err != nil {
		return JSONMetric{}, err
	}
	return JSONMetric{
		MetricKey: MetricKey{
			Name:       h.name,
			Type:       "histogram",
			JoinedTags: strings.Join(h.tags, ","),
		},
		Tags:  h.tags,
		Value: val,
	}, nil
}

func (h *Histo) Combine(other []byte) error {
	otherHistogram := tdigest.NewMerging(100, false)
	if err := otherHistogram.GobDecode(other); err != nil {
		return err
	}
	h.value.Merge(otherHistogram)
	return nil
}
