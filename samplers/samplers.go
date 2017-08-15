package samplers

import (
	"bytes"
	"encoding/binary"
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
	Hostname   string        `json:"host,omitempty"`
	DeviceName string        `json:"device_name,omitempty"`
	Interval   int32         `json:"interval,omitempty"`
}

type Aggregate int

const (
	AggregateMin Aggregate = 1 << iota
	AggregateMax
	AggregateMedian
	AggregateAverage
	AggregateCount
	AggregateSum
	AggregateHarmonicMean
)

var AggregatesLookup = map[string]Aggregate{
	"min":    AggregateMin,
	"max":    AggregateMax,
	"median": AggregateMedian,
	"avg":    AggregateAverage,
	"count":  AggregateCount,
	"sum":    AggregateSum,
	"hmean":  AggregateHarmonicMean,
}

type HistogramAggregates struct {
	Value Aggregate
	Count int
}

var aggregates = [...]string{
	AggregateMin:          "min",
	AggregateMax:          "max",
	AggregateMedian:       "median",
	AggregateAverage:      "avg",
	AggregateCount:        "count",
	AggregateSum:          "sum",
	AggregateHarmonicMean: "hmean",
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
	Name  string
	Tags  []string
	value int64
}

// Sample adds a sample to the counter.
func (c *Counter) Sample(sample float64, sampleRate float32) {
	c.value += int64(sample) * int64(1/sampleRate)
}

// Flush generates a DDMetric from the current state of this Counter.
func (c *Counter) Flush(interval time.Duration) []DDMetric {
	tags := make([]string, len(c.Tags))
	copy(tags, c.Tags)
	return []DDMetric{{
		Name:       c.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(c.value) / interval.Seconds()}},
		Tags:       tags,
		MetricType: "rate",
		Interval:   int32(interval.Seconds()),
	}}
}

// Export converts a Counter into a JSONMetric which reports the rate.
func (c *Counter) Export() (JSONMetric, error) {
	buf := new(bytes.Buffer)

	err := binary.Write(buf, binary.LittleEndian, c.value)
	if err != nil {
		return JSONMetric{}, err
	}

	return JSONMetric{
		MetricKey: MetricKey{
			Name:       c.Name,
			Type:       "counter",
			JoinedTags: strings.Join(c.Tags, ","),
		},
		Tags:  c.Tags,
		Value: buf.Bytes(),
	}, nil
}

// Combine merges the values seen with another set (marshalled as a byte slice)
func (c *Counter) Combine(other []byte) error {
	var otherCounts int64
	buf := bytes.NewReader(other)
	err := binary.Read(buf, binary.LittleEndian, &otherCounts)

	if err != nil {
		return err
	}

	c.value += otherCounts

	return nil
}

// NewCounter generates and returns a new Counter.
func NewCounter(Name string, Tags []string) *Counter {
	return &Counter{Name: Name, Tags: Tags}
}

// Gauge retains whatever the last value was.
type Gauge struct {
	Name  string
	Tags  []string
	value float64
}

// Sample takes on whatever value is passed in as a sample.
func (g *Gauge) Sample(sample float64, sampleRate float32) {
	g.value = sample
}

// Flush generates a DDMetric from the current state of this gauge.
func (g *Gauge) Flush() []DDMetric {
	tags := make([]string, len(g.Tags))
	copy(tags, g.Tags)
	return []DDMetric{{
		Name:       g.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(g.value)}},
		Tags:       tags,
		MetricType: "gauge",
	}}
}

// NewGauge genearaaaa who am I kidding just getting rid of the warning.
func NewGauge(Name string, Tags []string) *Gauge {
	return &Gauge{Name: Name, Tags: Tags}
}

// Set is a list of unique values seen.
type Set struct {
	Name string
	Tags []string
	Hll  *hyperloglog.HyperLogLogPlus
}

// Sample checks if the supplied value has is already in the filter. If not, it increments
// the counter!
func (s *Set) Sample(sample string, sampleRate float32) {
	hasher := fnv.New64a()
	hasher.Write([]byte(sample))
	s.Hll.Add(hasher)
}

// NewSet generates a new Set and returns it
func NewSet(Name string, Tags []string) *Set {
	// error is only returned if precision is outside the 4-18 range
	// TODO: this is the maximum precision, should it be configurable?
	Hll, _ := hyperloglog.NewPlus(18)
	return &Set{
		Name: Name,
		Tags: Tags,
		Hll:  Hll,
	}
}

// Flush generates a DDMetric for the state of this Set.
func (s *Set) Flush() []DDMetric {
	tags := make([]string, len(s.Tags))
	copy(tags, s.Tags)
	return []DDMetric{{
		Name:       s.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(s.Hll.Count())}},
		Tags:       tags,
		MetricType: "gauge",
	}}
}

// Export converts a Set into a JSONMetric which reports the Tags in the set.
func (s *Set) Export() (JSONMetric, error) {
	val, err := s.Hll.GobEncode()
	if err != nil {
		return JSONMetric{}, err
	}
	return JSONMetric{
		MetricKey: MetricKey{
			Name:       s.Name,
			Type:       "set",
			JoinedTags: strings.Join(s.Tags, ","),
		},
		Tags:  s.Tags,
		Value: val,
	}, nil
}

// Combine merges the values seen with another set (marshalled as a byte slice)
func (s *Set) Combine(other []byte) error {
	otherHLL, _ := hyperloglog.NewPlus(18)
	if err := otherHLL.GobDecode(other); err != nil {
		return err
	}
	if err := s.Hll.Merge(otherHLL); err != nil {
		// does not error unless compressions are different
		// however, decoding the other Hll causes us to use its compression
		// parameter, which might be different from ours
		return err
	}
	return nil
}

// Histo is a collection of values that generates max, min, count, and
// percentiles over time.
type Histo struct {
	Name  string
	Tags  []string
	Value *tdigest.MergingDigest
	// these values are computed from only the samples that came through this
	// veneur instance, ignoring any histograms merged from elsewhere
	// we separate them because they're easy to aggregate on the backend without
	// loss of granularity, and having host-local information on them might be
	// useful
	LocalWeight        float64
	LocalMin           float64
	LocalMax           float64
	LocalSum           float64
	LocalReciprocalSum float64
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample float64, sampleRate float32) {
	weight := float64(1 / sampleRate)
	h.Value.Add(sample, weight)

	h.LocalWeight += weight
	h.LocalMin = math.Min(h.LocalMin, sample)
	h.LocalMax = math.Max(h.LocalMax, sample)
	h.LocalSum += sample * weight

	h.LocalReciprocalSum += (1 / sample) * weight
}

// NewHist generates a new Histo and returns it.
func NewHist(Name string, Tags []string) *Histo {
	return &Histo{
		Name: Name,
		Tags: Tags,
		// we're going to allocate a lot of these, so we don't want them to be huge
		Value:    tdigest.NewMerging(100, false),
		LocalMin: math.Inf(+1),
		LocalMax: math.Inf(-1),
		LocalSum: 0,
	}
}

// Flush generates DDMetrics for the current state of the Histo. percentiles
// indicates what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64, aggregates HistogramAggregates) []DDMetric {
	now := float64(time.Now().Unix())
	// we only want to flush the number of samples we received locally, since
	// any other samples have already been flushed by a local veneur instance
	// before this was forwarded to us
	rate := h.LocalWeight / interval.Seconds()
	metrics := make([]DDMetric, 0, aggregates.Count+len(percentiles))

	if (aggregates.Value&AggregateMax) == AggregateMax && !math.IsInf(h.LocalMax, 0) {
		// Defensively recopy tags to avoid aliasing bugs in case multiple DDMetrics share the same
		// tag array in the future
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.max", h.Name),
			Value:      [1][2]float64{{now, h.LocalMax}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}
	if (aggregates.Value&AggregateMin) == AggregateMin && !math.IsInf(h.LocalMin, 0) {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.min", h.Name),
			Value:      [1][2]float64{{now, h.LocalMin}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}

	if (aggregates.Value&AggregateSum) == AggregateSum && h.LocalSum != 0 {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.sum", h.Name),
			Value:      [1][2]float64{{now, h.LocalSum}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}

	if (aggregates.Value&AggregateAverage) == AggregateAverage && h.LocalSum != 0 && h.LocalWeight != 0 {
		// we need both a rate and a non-zero sum before it will make sense
		// to submit an average
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.avg", h.Name),
			Value:      [1][2]float64{{now, h.LocalSum / h.LocalWeight}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}

	if (aggregates.Value&AggregateCount) == AggregateCount && rate != 0 {
		// if we haven't received any local samples, then leave this sparse,
		// otherwise it can lead to some misleading zeroes in between the
		// flushes of downstream instances
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.count", h.Name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       tags,
			MetricType: "rate",
			Interval:   int32(interval.Seconds()),
		})
	}

	if (aggregates.Value & AggregateMedian) == AggregateMedian {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(
			metrics,
			DDMetric{
				Name:       fmt.Sprintf("%s.median", h.Name),
				Value:      [1][2]float64{{now, h.Value.Quantile(0.5)}},
				Tags:       tags,
				MetricType: "gauge",
			},
		)
	}

	if (aggregates.Value&AggregateHarmonicMean) == AggregateHarmonicMean && h.LocalReciprocalSum != 0 && h.LocalWeight != 0 {
		// we need both a rate and a non-zero sum before it will make sense
		// to submit an average
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.hmean", h.Name),
			Value:      [1][2]float64{{now, h.LocalWeight / h.LocalReciprocalSum}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}

	for _, p := range percentiles {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.Name, int(p*100)),
				Value:      [1][2]float64{{now, h.Value.Quantile(p)}},
				Tags:       tags,
				MetricType: "gauge",
			},
		)
	}

	return metrics
}

// Export converts a Histogram into a JSONMetric
func (h *Histo) Export() (JSONMetric, error) {
	val, err := h.Value.GobEncode()
	if err != nil {
		return JSONMetric{}, err
	}
	return JSONMetric{
		MetricKey: MetricKey{
			Name:       h.Name,
			Type:       "histogram",
			JoinedTags: strings.Join(h.Tags, ","),
		},
		Tags:  h.Tags,
		Value: val,
	}, nil
}

// Combine merges the values of a histogram with another histogram
// (marshalled as a byte slice)
func (h *Histo) Combine(other []byte) error {
	otherHistogram := tdigest.NewMerging(100, false)
	if err := otherHistogram.GobDecode(other); err != nil {
		return err
	}
	h.Value.Merge(otherHistogram)
	return nil
}
