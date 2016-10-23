package veneur

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/clarkduvall/hyperloglog"
	"github.com/stripe/veneur/tdigest"
)

const PartitionDateFormat = "20060102"
const RedshiftDateFormat = "2006-01-02 03:04:05"

// DDMetric is a data structure that represents the JSON that Datadog
// wants when posting to the API
type DDMetric struct {
	Name       string        `json:"metric"`
	Value      [1][2]float64 `json:"points"`
	Tags       []string      `json:"tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host"`
	DeviceName string        `json:"device_name"`
	Interval   int32         `json:"interval,omitempty"`
}

type tsvField int

const (
	// the order in which these appear determines the
	// order of the fields in the resultant TSV
	tsvName tsvField = iota
	tsvTags
	tsvMetricType

	// The hostname attached to the metric
	tsvHostname

	// The hostname of the server flushing the data
	tsvVeneurHostname

	tsvDeviceName
	tsvInterval

	tsvTimestamp
	tsvValue

	// This is the _partition field
	// required by the Redshift IncrementalLoader.
	// For our purposes, the current date is a good partition.
	tsvPartition
)

var tsvSchema = [...]string{
	tsvName:           "tsvName",
	tsvTags:           "tsvTags",
	tsvMetricType:     "tsvMetricType",
	tsvHostname:       "tsvHostname",
	tsvDeviceName:     "tsvDeviceName",
	tsvInterval:       "tsvInterval",
	tsvVeneurHostname: "tsvVeneurHostname",
	tsvTimestamp:      "tsvTimestamp",
	tsvValue:          "tsvValue",
	tsvPartition:      "tsvPartition",
}

// String returns the field name.
// eg tsvName.String() returns "Name"
func (f tsvField) String() string {
	return fmt.Sprintf(strings.Replace(tsvSchema[f], "tsv", "", 1))
}

// each key in tsvMapping is guaranteed to have a unique value
var tsvMapping = map[string]int{}

func init() {
	for i, field := range tsvSchema {
		tsvMapping[field] = i
	}
}

// encodeCSV generates a newline-terminated CSV row that describes
// the data represented by the DDMetric.
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func (d DDMetric) encodeCSV(w *csv.Writer, partitionDate *time.Time, hostname string) error {

	timestamp := d.Value[0][0]
	value := strconv.FormatFloat(d.Value[0][1], 'f', -1, 64)
	interval := strconv.Itoa(int(d.Interval))

	// TODO(aditya) some better error handling for this
	// to guarantee that the result is proper JSON
	tags := "{" + strings.Join(d.Tags, ",") + "}"

	fields := [...]string{
		// the order here doesn't actually matter
		// as long as the keys are right
		tsvName:           d.Name,
		tsvTags:           tags,
		tsvMetricType:     d.MetricType,
		tsvHostname:       d.Hostname,
		tsvDeviceName:     d.DeviceName,
		tsvInterval:       interval,
		tsvVeneurHostname: hostname,
		tsvValue:          value,

		tsvTimestamp: time.Unix(int64(timestamp), 0).UTC().Format(RedshiftDateFormat),

		// TODO avoid edge case at midnight
		tsvPartition: partitionDate.UTC().Format(PartitionDateFormat),
	}

	w.Write(fields[:])
	return w.Error()
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
	tags := make([]string, len(c.tags))
	copy(tags, c.tags)
	return []DDMetric{{
		Name:       c.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(c.value) / interval.Seconds()}},
		Tags:       tags,
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
	tags := make([]string, len(g.tags))
	copy(tags, g.tags)
	return []DDMetric{{
		Name:       g.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(g.value)}},
		Tags:       tags,
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
	tags := make([]string, len(s.tags))
	copy(tags, s.tags)
	return []DDMetric{{
		Name:       s.name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(s.hll.Count())}},
		Tags:       tags,
		MetricType: "gauge",
	}}
}

// Export converts a Set into a JSONMetric which reports the tags in the set.
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

// Combine merges the values seen with another set (marshalled as a byte slice)
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
	localSum    float64
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample float64, sampleRate float32) {
	weight := float64(1 / sampleRate)
	h.value.Add(sample, weight)

	h.localWeight += weight
	h.localMin = math.Min(h.localMin, sample)
	h.localMax = math.Max(h.localMax, sample)
	h.localSum += sample * weight
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
		localSum: 0,
	}
}

// Flush generates DDMetrics for the current state of the Histo. percentiles
// indicates what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64, aggregates HistogramAggregates) []DDMetric {
	now := float64(time.Now().Unix())
	// we only want to flush the number of samples we received locally, since
	// any other samples have already been flushed by a local veneur instance
	// before this was forwarded to us
	rate := h.localWeight / interval.Seconds()
	metrics := make([]DDMetric, 0, aggregates.Count+len(percentiles))

	if (aggregates.Value&AggregateMax) == AggregateMax && !math.IsInf(h.localMax, 0) {
		// XXX: why is this recreating tags mutliple times, granted they won't always be used, but it seems preferable to
		// generate them once anyway
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.max", h.name),
			Value:      [1][2]float64{{now, h.localMax}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}
	if (aggregates.Value&AggregateMin) == AggregateMin && !math.IsInf(h.localMin, 0) {
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.min", h.name),
			Value:      [1][2]float64{{now, h.localMin}},
			Tags:       tags,
			MetricType: "gauge",
		})
	}

	if (aggregates.Value&AggregateSum) == AggregateSum && h.localSum != 0 {
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.sum", h.name),
			Value:      [1][2]float64{{now, h.localSum}},
			Tags:       tags,
			MetricType: "gauge",
		})
		if (aggregates.Value&AggregateAverage) == AggregateAverage && h.localWeight != 0 {
			// we need both a rate and a non-zero sum before it will make sense
			// to submit an average
			metrics = append(metrics, DDMetric{
				Name:       fmt.Sprintf("%s.avg", h.name),
				Value:      [1][2]float64{{now, h.localSum / h.localWeight}},
				Tags:       tags,
				MetricType: "gauge",
			})
		}
	}

	if (aggregates.Value&AggregateCount) == AggregateCount && rate != 0 {
		// if we haven't received any local samples, then leave this sparse,
		// otherwise it can lead to some misleading zeroes in between the
		// flushes of downstream instances
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.count", h.name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       tags,
			MetricType: "rate",
			Interval:   int32(interval.Seconds()),
		})
	}

	if (aggregates.Value & AggregateMedian) == AggregateMedian {
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(
			metrics,
			DDMetric{
				Name:       fmt.Sprintf("%s.median", h.name),
				Value:      [1][2]float64{{now, h.value.Quantile(0.5)}},
				Tags:       tags,
				MetricType: "gauge",
			},
		)
	}

	for _, p := range percentiles {
		tags := make([]string, len(h.tags))
		copy(tags, h.tags)
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.name, int(p*100)),
				Value:      [1][2]float64{{now, h.value.Quantile(p)}},
				Tags:       tags,
				MetricType: "gauge",
			},
		)
	}

	return metrics
}

// Export converts a Histogram into a JSONMetric
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

// Combine merges the values of a histogram with another histogram
// (marshalled as a byte slice)
func (h *Histo) Combine(other []byte) error {
	otherHistogram := tdigest.NewMerging(100, false)
	if err := otherHistogram.GobDecode(other); err != nil {
		return err
	}
	h.value.Merge(otherHistogram)
	return nil
}
