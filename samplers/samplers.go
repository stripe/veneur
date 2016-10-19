package samplers

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
	Tags       []string      `json:"Tags,omitempty"`
	MetricType string        `json:"type"`
	Hostname   string        `json:"host"`
	DeviceName string        `json:"device_Name"`
	Interval   int32         `json:"interval,omitempty"`
}

type tsvField int

const (
	// the order in which these appear determines the
	// order of the fields in the resultant TSV
	TsvName tsvField = iota
	TsvTags
	TsvMetricType

	// The hostName attached to the metric
	TsvHostname

	// The hostName of the server flushing the data
	TsvVeneurHostname

	TsvDeviceName
	TsvInterval

	TsvTimestamp
	TsvValue

	// This is the _partition field
	// required by the Redshift IncrementalLoader.
	// For our purposes, the current date is a good partition.
	TsvPartition
)

var tsvSchema = [...]string{
	TsvName:           "TsvName",
	TsvTags:           "TsvTags",
	TsvMetricType:     "TsvMetricType",
	TsvHostname:       "TsvHostname",
	TsvDeviceName:     "TsvDeviceName",
	TsvInterval:       "TsvInterval",
	TsvVeneurHostname: "TsvVeneurHostname",
	TsvTimestamp:      "TsvTimestamp",
	TsvValue:          "TsvValue",
	TsvPartition:      "TsvPartition",
}

// String returns the field Name.
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

// EncodeCSV generates a newline-terminated CSV row that describes
// the data represented by the DDMetric.
// The caller is responsible for setting w.Comma as the appropriate delimiter.
// For performance, encodeCSV does not flush after every call; the caller is
// expected to flush at the end of the operation cycle
func (d DDMetric) EncodeCSV(w *csv.Writer, partitionDate *time.Time, hostName string) error {

	timestamp := d.Value[0][0]
	value := strconv.FormatFloat(d.Value[0][1], 'f', -1, 64)
	interval := strconv.Itoa(int(d.Interval))

	// TODO(aditya) some better error handling for this
	// to guarantee that the result is proper JSON
	Tags := "{" + strings.Join(d.Tags, ",") + "}"

	fields := [...]string{
		// the order here doesn't actually matter
		// as long as the keys are right
		TsvName:           d.Name,
		TsvTags:           Tags,
		TsvMetricType:     d.MetricType,
		TsvHostname:       d.Hostname,
		TsvDeviceName:     d.DeviceName,
		TsvInterval:       interval,
		TsvVeneurHostname: hostName,
		TsvValue:          value,

		TsvTimestamp: time.Unix(int64(timestamp), 0).UTC().Format(RedshiftDateFormat),

		// TODO avoid edge case at midnight
		TsvPartition: partitionDate.UTC().Format(PartitionDateFormat),
	}

	w.Write(fields[:])
	return w.Error()
}

// JSONMetric is used to represent a metric that can be remarshaled with its
// internal state intact. It is used to send metrics from one Veneur to another.
type JSONMetric struct {
	MetricKey
	Tags []string `json:"Tags"`
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
	Tags := make([]string, len(c.Tags))
	copy(Tags, c.Tags)
	return []DDMetric{{
		Name:       c.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(c.value) / interval.Seconds()}},
		Tags:       Tags,
		MetricType: "rate",
		Interval:   int32(interval.Seconds()),
	}}
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
	Tags := make([]string, len(g.Tags))
	copy(Tags, g.Tags)
	return []DDMetric{{
		Name:       g.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(g.value)}},
		Tags:       Tags,
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
	Tags := make([]string, len(s.Tags))
	copy(Tags, s.Tags)
	return []DDMetric{{
		Name:       s.Name,
		Value:      [1][2]float64{{float64(time.Now().Unix()), float64(s.Hll.Count())}},
		Tags:       Tags,
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
	LocalWeight float64
	LocalMin    float64
	LocalMax    float64
}

// Sample adds the supplied value to the histogram.
func (h *Histo) Sample(sample float64, sampleRate float32) {
	weight := float64(1 / sampleRate)
	h.Value.Add(sample, weight)

	h.LocalWeight += weight
	h.LocalMin = math.Min(h.LocalMin, sample)
	h.LocalMax = math.Max(h.LocalMax, sample)
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
	}
}

// HistogramLocalLength is the maximum number of DDMetrics that a histogram can flush if
// len(percentiles)==0
// specifically the count, min and max
const HistogramLocalLength = 3

// Flush generates DDMetrics for the current state of the Histo. percentiles
// indicates what percentiles should be exported from the histogram.
func (h *Histo) Flush(interval time.Duration, percentiles []float64) []DDMetric {
	now := float64(time.Now().Unix())
	// we only want to flush the number of samples we received locally, since
	// any other samples have already been flushed by a local veneur instance
	// before this was forwarded to us
	rate := h.LocalWeight / interval.Seconds()
	metrics := make([]DDMetric, 0, 3+len(percentiles))

	if !math.IsInf(h.LocalMax, 0) {
		Tags := make([]string, len(h.Tags))
		copy(Tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.max", h.Name),
			Value:      [1][2]float64{{now, h.LocalMax}},
			Tags:       Tags,
			MetricType: "gauge",
		})
	}
	if !math.IsInf(h.LocalMin, 0) {
		Tags := make([]string, len(h.Tags))
		copy(Tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.min", h.Name),
			Value:      [1][2]float64{{now, h.LocalMin}},
			Tags:       Tags,
			MetricType: "gauge",
		})
	}
	if rate != 0 {
		// if we haven't received any local samples, then leave this sparse,
		// otherwise it can lead to some misleading zeroes in between the
		// flushes of downstream instances
		Tags := make([]string, len(h.Tags))
		copy(Tags, h.Tags)
		metrics = append(metrics, DDMetric{
			Name:       fmt.Sprintf("%s.count", h.Name),
			Value:      [1][2]float64{{now, rate}},
			Tags:       Tags,
			MetricType: "rate",
			Interval:   int32(interval.Seconds()),
		})
	}

	for _, p := range percentiles {
		Tags := make([]string, len(h.Tags))
		copy(Tags, h.Tags)
		metrics = append(
			metrics,
			// TODO Fix to allow for p999, etc
			DDMetric{
				Name:       fmt.Sprintf("%s.%dpercentile", h.Name, int(p*100)),
				Value:      [1][2]float64{{now, h.Value.Quantile(p)}},
				Tags:       Tags,
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
