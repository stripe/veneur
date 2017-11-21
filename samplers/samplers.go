package samplers

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/axiomhq/hyperloglog"
	"github.com/stripe/veneur/tdigest"
)

// MetricType defines what kind of metric this is, so that we or our upstream
// sinks can do the right thing with it.
type MetricType int

const (
	// CounterMetric is a counter
	CounterMetric MetricType = iota
	// GaugeMetric is a gauge
	GaugeMetric
)

// InterMetric represents a metric that has been completed and is ready for
// flushing by sinks.
type InterMetric struct {
	Name      string
	Timestamp int64
	Value     float64
	Tags      []string
	Type      MetricType
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

// Flush generates an InterMetric from the current state of this Counter.
func (c *Counter) Flush(interval time.Duration) []InterMetric {
	tags := make([]string, len(c.Tags))
	copy(tags, c.Tags)
	return []InterMetric{{
		Name:      c.Name,
		Timestamp: time.Now().Unix(),
		Value:     float64(c.value),
		Tags:      tags,
		Type:      CounterMetric,
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

// Combine overwrites gauge (marshalled as a byte slice)
func (g *Gauge) Combine(other []byte) error {
	bits := binary.LittleEndian.Uint64(other)
	otherGauge := math.Float64frombits(bits)

	g.value += otherGauge

	return nil
}

// Flush generates an InterMetric from the current state of this gauge.
func (g *Gauge) Flush() []InterMetric {
	tags := make([]string, len(g.Tags))
	copy(tags, g.Tags)
	return []InterMetric{{
		Name:      g.Name,
		Timestamp: time.Now().Unix(),
		Value:     float64(g.value),
		Tags:      tags,
		Type:      GaugeMetric,
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
	Hll  *hyperloglog.Sketch
}

// Sample checks if the supplied value has is already in the filter. If not, it increments
// the counter!
func (s *Set) Sample(sample string, sampleRate float32) {
	s.Hll.Insert([]byte(sample))
}

// NewSet generates a new Set and returns it
func NewSet(Name string, Tags []string) *Set {
	// error is only returned if precision is outside the 4-18 range
	// TODO: this is the maximum precision, should it be configurable?
	Hll := hyperloglog.New()
	return &Set{
		Name: Name,
		Tags: Tags,
		Hll:  Hll,
	}
}

// Flush generates an InterMetric for the state of this Set.
func (s *Set) Flush() []InterMetric {
	tags := make([]string, len(s.Tags))
	copy(tags, s.Tags)
	return []InterMetric{{
		Name:      s.Name,
		Timestamp: time.Now().Unix(),
		Value:     float64(s.Hll.Estimate()),
		Tags:      tags,
		Type:      GaugeMetric,
	}}
}

// Export converts a Set into a JSONMetric which reports the Tags in the set.
func (s *Set) Export() (JSONMetric, error) {
	val, err := s.Hll.MarshalBinary()
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
	otherHLL := hyperloglog.New()
	if err := otherHLL.UnmarshalBinary(other); err != nil {
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

// Flush generates InterMetrics for the current state of the Histo. percentiles
// indicates what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64, aggregates HistogramAggregates) []InterMetric {
	now := time.Now().Unix()
	metrics := make([]InterMetric, 0, aggregates.Count+len(percentiles))

	if (aggregates.Value&AggregateMax) == AggregateMax && !math.IsInf(h.LocalMax, 0) {
		// Defensively recopy tags to avoid aliasing bugs in case multiple InterMetrics share the same
		// tag array in the future
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.max", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalMax),
			Tags:      tags,
			Type:      GaugeMetric,
		})
	}
	if (aggregates.Value&AggregateMin) == AggregateMin && !math.IsInf(h.LocalMin, 0) {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.min", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalMin),
			Tags:      tags,
			Type:      GaugeMetric,
		})
	}

	if (aggregates.Value&AggregateSum) == AggregateSum && h.LocalSum != 0 {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.sum", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalSum),
			Tags:      tags,
			Type:      GaugeMetric,
		})
	}

	if (aggregates.Value&AggregateAverage) == AggregateAverage && h.LocalSum != 0 && h.LocalWeight != 0 {
		// we need both a rate and a non-zero sum before it will make sense
		// to submit an average
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.avg", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalSum / h.LocalWeight),
			Tags:      tags,
			Type:      GaugeMetric,
		})
	}

	if (aggregates.Value&AggregateCount) == AggregateCount && h.LocalWeight != 0 {
		// if we haven't received any local samples, then leave this sparse,
		// otherwise it can lead to some misleading zeroes in between the
		// flushes of downstream instances
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.count", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalWeight),
			Tags:      tags,
			Type:      CounterMetric,
		})
	}

	if (aggregates.Value & AggregateMedian) == AggregateMedian {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(
			metrics,
			InterMetric{
				Name:      fmt.Sprintf("%s.median", h.Name),
				Timestamp: now,
				Value:     float64(h.Value.Quantile(0.5)),
				Tags:      tags,
				Type:      GaugeMetric,
			},
		)
	}

	if (aggregates.Value&AggregateHarmonicMean) == AggregateHarmonicMean && h.LocalReciprocalSum != 0 && h.LocalWeight != 0 {
		// we need both a rate and a non-zero sum before it will make sense
		// to submit an average
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(metrics, InterMetric{
			Name:      fmt.Sprintf("%s.hmean", h.Name),
			Timestamp: now,
			Value:     float64(h.LocalWeight / h.LocalReciprocalSum),
			Tags:      tags,
			Type:      GaugeMetric,
		})
	}

	for _, p := range percentiles {
		tags := make([]string, len(h.Tags))
		copy(tags, h.Tags)
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			InterMetric{
				Name:      fmt.Sprintf("%s.%dpercentile", h.Name, int(p*100)),
				Timestamp: now,
				Value:     float64(h.Value.Quantile(p)),
				Tags:      tags,
				Type:      GaugeMetric,
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
