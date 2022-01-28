package samplers

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/tdigest"
)

const ε = .01

func TestRouting(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		sinks     RouteInformation
		sinkNames []string
	}{{
		"none specified",
		[]string{"foo:bar", "veneurlocalonly"},
		nil,
		[]string{"foosink", "barsink"},
	}, {
		"none specified",
		[]string{"foo:bar", "veneurlocalonly:"},
		nil,
		[]string{"foosink", "barsink"},
	}, {
		"one sink",
		[]string{"veneursinkonly:foobar"},
		map[string]struct{}{"foobar": {}},
		[]string{"foobar"},
	}, {
		"multiple sinks",
		[]string{"veneursinkonly:foobar", "veneursinkonly:baz"},
		map[string]struct{}{"foobar": {}, "baz": {}},
		[]string{"foobar", "baz"},
	}}

	for _, elt := range tests {
		test := elt
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			info := routeInfo(test.tags)
			assert.Equal(t, test.sinks, info)
			for _, sink := range test.sinkNames {
				assert.True(t, info.RouteTo(sink), "Should route to %q", sink)
			}
			if test.sinks != nil {
				assert.False(t, info.RouteTo("never_to_this_sink"))
			}
		})
	}
}

func TestCounterEmpty(t *testing.T) {
	c := NewCounter("a.b.c", []string{"a:b"})
	c.Sample(1, 1.0)

	assert.Equal(t, "a.b.c", c.Name, "Name")
	assert.Len(t, c.Tags, 1, "Tag length")
	assert.Equal(t, c.Tags[0], "a:b", "Tag contents")

	metrics := c.Flush(10 * time.Second)
	assert.Len(t, metrics, 1, "Flushes 1 metric")

	m1 := metrics[0]
	assert.Equal(t, CounterMetric, m1.Type, "Type")
	assert.Len(t, c.Tags, 1, "Tag length")
	assert.Equal(t, c.Tags[0], "a:b", "Tag contents")
	assert.Equal(t, float64(1), m1.Value, "Metric value")
}

func TestCounterRate(t *testing.T) {
	c := NewCounter("a.b.c", []string{"a:b"})

	c.Sample(5, 1.0)

	metrics := c.Flush(10 * time.Second)
	assert.Equal(t, float64(5), metrics[0].Value, "Metric value")
}

func TestCounterSampleRate(t *testing.T) {
	c := NewCounter("a.b.c", []string{"a:b"})

	c.Sample(5, 0.5)

	metrics := c.Flush(10 * time.Second)
	assert.Equal(t, float64(10), metrics[0].Value, "Metric value")
}

func TestCounterMerge(t *testing.T) {
	c := NewCounter("a.b.c", []string{"tag:val"})

	c.Sample(5, 0.5)
	jm, err := c.Export()
	assert.NoError(t, err, "should have exported counter successfully")

	c2 := NewCounter("a.b.c", []string{"tag:val"})

	c2.Sample(14, 0.5)
	// test with a different interval, and ensure the correct rate is passed on
	jm2, err2 := c2.Export()
	assert.NoError(t, err2, "should have exported counter successfully")

	cGlobal := NewCounter("a.b.c", []string{"tag2: val2"})
	assert.NoError(t, cGlobal.Combine(jm.Value), "should have combined counters successfully")

	metrics := cGlobal.Flush(10 * time.Second)
	assert.Equal(t, float64(10), metrics[0].Value)

	assert.NoError(t, cGlobal.Combine(jm2.Value), "should have combined counters successfully")

	metrics = cGlobal.Flush(10 * time.Second)
	assert.Equal(t, float64(38), metrics[0].Value)
}

// Test the Metric and Merge methods on Counter
func TestCounterMergeMetric(t *testing.T) {
	c := NewCounter("a.b.c", []string{"tag:val"})
	c.Sample(5, 0.5)
	m, err := c.Metric()
	assert.NoError(t, err, "should have created a metric from a counter successfully")

	// test with a different interval, and ensure the correct rate is passed on
	c2 := NewCounter("a.b.c", []string{"tag:val"})
	c2.Sample(14, 0.5)
	m2, err := c2.Metric()
	assert.NoError(t, err, "should have created a metric from a counter")

	cGlobal := NewCounter("a.b.c", []string{"tag2: val2"})

	cGlobal.Merge(m.GetCounter())
	metrics := cGlobal.Flush(10 * time.Second)
	assert.Equal(t, float64(10), metrics[0].Value)

	cGlobal.Merge(m2.GetCounter())
	metrics = cGlobal.Flush(10 * time.Second)
	assert.Equal(t, float64(38), metrics[0].Value)
}

func TestGaugeMerge(t *testing.T) {
	g := NewGauge("a.b.c", []string{"tag:val"})

	g.Sample(5, 1.0)
	jm, err := g.Export()
	assert.NoError(t, err, "should have exported gauge succcesfully")

	gGlobal := NewGauge("a.b.c", []string{"tag2: val2"})
	gGlobal.value = 1 // So we can overwrite it
	assert.NoError(t, gGlobal.Combine(jm.Value), "should have combined gauges successfully")

	metrics := gGlobal.Flush()
	assert.Equal(t, float64(5), metrics[0].Value)
}

func TestGauge(t *testing.T) {
	g := NewGauge("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", g.Name, "Name")
	assert.Len(t, g.Tags, 1, "Tag length")
	assert.Equal(t, g.Tags[0], "a:b", "Tag contents")

	g.Sample(5, 1.0)

	metrics := g.Flush()
	assert.Len(t, metrics, 1, "Flushed metric count")

	m1 := metrics[0]
	assert.Equal(t, GaugeMetric, m1.Type, "Type")
	tags := m1.Tags
	assert.Len(t, tags, 1, "Tag length")
	assert.Equal(t, tags[0], "a:b", "Tag contents")

	assert.Equal(t, float64(5), m1.Value, "Value")
}

// Test the Metric and Merge function on Gauge
func TestGaugeMergeMetric(t *testing.T) {
	g := NewGauge("a.b.c", []string{"tag:val"})

	g.Sample(5, 1.0)
	m, err := g.Metric()
	assert.NoError(t, err, "a gauge should be able to create a metric")

	gGlobal := NewGauge("a.b.c", []string{"tag2: val2"})
	gGlobal.value = 1 // So we can overwrite it
	gGlobal.Merge(m.GetGauge())

	metrics := gGlobal.Flush()
	assert.Equal(t, float64(5), metrics[0].Value)
}

func TestSet(t *testing.T) {
	s := NewSet("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", s.Name, "Name")
	assert.Len(t, s.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", s.Tags[0], "First tag")

	s.Sample("5")

	s.Sample("5")

	s.Sample("123")

	s.Sample("2147483647")
	s.Sample("-2147483648")

	metrics := s.Flush()
	assert.Len(t, metrics, 1, "Flush")

	m1 := metrics[0]
	assert.Equal(t, GaugeMetric, m1.Type, "Type")
	assert.Len(t, m1.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m1.Tags[0], "First tag")
	assert.Equal(t, float64(4), m1.Value, "Value")
}

func TestSetMerge(t *testing.T) {
	rand.Seed(time.Now().Unix())

	s := NewSet("a.b.c", []string{"a:b"})
	for i := 0; i < 100; i++ {
		s.Sample(strconv.Itoa(rand.Int()))
	}
	assert.Equal(t, uint64(100), s.Hll.Estimate(), "counts did not match")

	jm, err := s.Export()
	assert.NoError(t, err, "should have exported successfully")

	s2 := NewSet("a.b.c", []string{"a:b"})
	assert.NoError(t, s2.Combine(jm.Value), "should have combined successfully")
	// HLLs are approximate, and we've seen error of +-1 here in the past, so
	// we're giving the test some room for error to reduce flakes
	count1 := int(s.Hll.Estimate())
	count2 := int(s2.Hll.Estimate())
	countDifference := count1 - count2
	assert.True(t, -1 <= countDifference && countDifference <= 1, "counts did not match after merging (%d and %d)", count1, count2)
}

// Test the Metric and Merge function on Set
func TestSetMergeMetric(t *testing.T) {
	rand.Seed(time.Now().Unix())

	s := NewSet("a.b.c", []string{"a:b"})
	for i := 0; i < 100; i++ {
		s.Sample(strconv.Itoa(rand.Int()))
	}
	assert.Equal(t, uint64(100), s.Hll.Estimate(), "counts did not match")

	m, err := s.Metric()
	assert.NoError(t, err, "should have made a metric from a set")

	s2 := NewSet("a.b.c", []string{"a:b"})
	assert.NoError(t, s2.Merge(m.GetSet()), "should have combined successfully")
	// HLLs are approximate, and we've seen error of +-1 here in the past, so
	// we're giving the test some room for error to reduce flakes
	count1 := int(s.Hll.Estimate())
	count2 := int(s2.Hll.Estimate())
	countDifference := count1 - count2
	assert.True(t, -1 <= countDifference && countDifference <= 1, "counts did not match after merging (%d and %d)", count1, count2)
}

func digest(values []float64) *tdigest.MergingDigest {
	td := tdigest.NewMerging(100, false)
	for _, v := range values {
		td.Add(v, 1.0)
	}
	return td
}

// ensure histogram with no local samples flush aggregates for global flushes
// but no aggregates for non-global (mixed scope) flushes.
func TestGlobalHistoFlushBehavior(t *testing.T) {
	aggregates := HistogramAggregates{
		Value: AggregateMin,
		Count: 1,
	}
	var percentiles []float64

	h := NewHist("test", []string{})
	h.Value = digest([]float64{1.0})

	m := h.Flush(10*time.Second, percentiles, aggregates, true)
	assert.Len(t, m, 1, "global histogram didn't return aggregates for global flush")
	assert.Equal(t, float64(1), m[0].Value, "global histogram returned invalid value for global flush")

	m = h.Flush(10*time.Second, percentiles, aggregates, false)
	assert.Empty(t, m, "global histogram returned aggregates for mixed scope flush")
}

// ensure histogram with local samples flush global aggregates for global flushes
// but local aggregates for non-global (mixed scope) flushes.
func TestLocalHistoFlushedBehavior(t *testing.T) {
	aggregates := HistogramAggregates{
		Value: AggregateCount,
		Count: 1,
	}
	var percentiles []float64

	// the local count for this histogram should be 1. the global count should be 0.
	h := NewHist("test", []string{})
	h.Sample(1.0, 1.0)
	h.Value = tdigest.NewMerging(100, false)

	m := h.Flush(10*time.Second, percentiles, aggregates, true)
	assert.Len(t, m, 1, "histogram didn't return aggregates for global flush")
	assert.Equal(t, float64(0), m[0].Value, "histogram returned value for local count during global flush")

	m = h.Flush(10*time.Second, percentiles, aggregates, false)
	assert.Len(t, m, 1, "histogram returned no aggregates for mixed scope flush")
	assert.Equal(t, float64(1), m[0].Value, "histogram returned global value for mixed scope flush.")
}

func TestHisto(t *testing.T) {
	h := NewHist("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", h.Name, "Name")
	assert.Len(t, h.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", h.Tags[0], "First tag")

	h.Sample(5, 1.0)
	h.Sample(10, 1.0)
	h.Sample(15, 1.0)
	h.Sample(20, 1.0)
	h.Sample(25, 1.0)

	var aggregates HistogramAggregates
	aggregates.Value = AggregateMin | AggregateMax | AggregateMedian |
		AggregateAverage | AggregateCount | AggregateSum |
		AggregateHarmonicMean
	aggregates.Count = 7

	percentiles := []float64{0.90}

	metrics := h.Flush(10*time.Second, percentiles, aggregates, true)
	// We get lots of metrics back for histograms!
	// One for each of the aggregates specified, plus
	// one for the explicit percentile we are asking for
	assert.Len(t, metrics, aggregates.Count+len(percentiles), "Flushed metrics length")

	// the max
	m2 := metrics[0]
	assert.Equal(t, "a.b.c.max", m2.Name, "Name")
	assert.Equal(t, GaugeMetric, m2.Type, "Type")
	assert.Len(t, m2.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m2.Tags[0], "First tag")
	assert.Equal(t, float64(25), m2.Value, "Value")

	// the min
	m3 := metrics[1]
	assert.Equal(t, "a.b.c.min", m3.Name, "Name")
	assert.Equal(t, GaugeMetric, m3.Type, "Type")
	assert.Len(t, m3.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m3.Tags[0], "First tag")
	assert.Equal(t, float64(5), m3.Value, "Value")

	// the sum
	m4 := metrics[2]
	assert.Equal(t, "a.b.c.sum", m4.Name, "Name")
	assert.Equal(t, GaugeMetric, m4.Type, "Type")
	assert.Len(t, m4.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m4.Tags[0], "First tag")
	assert.Equal(t, float64(75), m4.Value, "Value")

	// the average
	m5 := metrics[3]
	assert.Equal(t, "a.b.c.avg", m5.Name, "Name")
	assert.Equal(t, GaugeMetric, m5.Type, "Type")
	assert.Len(t, m5.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m5.Tags[0], "First tag")
	assert.Equal(t, float64(15), m5.Value, "Value")

	// the count
	m1 := metrics[4]
	assert.Equal(t, "a.b.c.count", m1.Name, "Name")
	assert.Equal(t, CounterMetric, m1.Type, "Type")
	assert.Len(t, m1.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m1.Tags[0], "First tag")
	assert.Equal(t, float64(5), m1.Value, "Value")

	// the median
	m6 := metrics[5]
	assert.Equal(t, "a.b.c.median", m6.Name, "Name")
	assert.Equal(t, GaugeMetric, m6.Type, "Type")
	assert.Len(t, m6.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m6.Tags[0], "First tag")
	assert.Equal(t, float64(15), m6.Value, "Value")

	// the average
	m8 := metrics[6]
	assert.Equal(t, "a.b.c.hmean", m8.Name, "Name")
	assert.Equal(t, GaugeMetric, m8.Type, "Type")
	assert.Len(t, m8.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m8.Tags[0], "First tag")
	expected := float64(5.0 / ((1.0 / 5) + (1.0 / 10) + (1.0 / 15) + (1.0 / 20) + (1.0 / 25)))
	assert.Equal(t, expected, m8.Value, "Value")

	// And the percentile
	m7 := metrics[7]
	assert.Equal(t, "a.b.c.90percentile", m7.Name, "Name")
	assert.Equal(t, GaugeMetric, m7.Type, "Type")
	assert.Len(t, m7.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m7.Tags[0], "First tag")
	assert.Equal(t, float64(23.75), m7.Value, "Value")
}

func TestHistoAvgOnly(t *testing.T) {
	h := NewHist("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", h.Name, "Name")
	assert.Len(t, h.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", h.Tags[0], "First tag")

	h.Sample(5, 1.0)
	h.Sample(10, 1.0)
	h.Sample(15, 1.0)
	h.Sample(20, 1.0)
	h.Sample(25, 1.0)

	var aggregates HistogramAggregates
	aggregates.Value = AggregateAverage
	aggregates.Count = 1

	percentiles := []float64{}

	metrics := h.Flush(10*time.Second, percentiles, aggregates, true)
	// We get lots of metrics back for histograms!
	// One for each of the aggregates specified, plus
	// one for the explicit percentile we are asking for
	assert.Len(t, metrics, aggregates.Count, "Flushed metrics length")

	// the average
	m5 := metrics[0]
	assert.Equal(t, "a.b.c.avg", m5.Name, "Name")
	assert.Equal(t, GaugeMetric, m5.Type, "Type")
	assert.Len(t, m5.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m5.Tags[0], "First tag")
	assert.Equal(t, float64(15), m5.Value, "Value")
}

func TestHistoHMeanOnly(t *testing.T) {
	h := NewHist("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", h.Name, "Name")
	assert.Len(t, h.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", h.Tags[0], "First tag")

	h.Sample(5, 1.0)
	h.Sample(10, 1.0)
	h.Sample(15, 1.0)
	h.Sample(20, 1.0)
	h.Sample(25, 1.0)

	var aggregates HistogramAggregates
	aggregates.Value = AggregateHarmonicMean
	aggregates.Count = 1

	percentiles := []float64{}

	metrics := h.Flush(10*time.Second, percentiles, aggregates, true)
	// We get lots of metrics back for histograms!
	// One for each of the aggregates specified, plus
	// one for the explicit percentile we are asking for
	assert.Len(t, metrics, aggregates.Count, "Flushed metrics length")

	// the average
	m5 := metrics[0]
	assert.Equal(t, "a.b.c.hmean", m5.Name, "Name")
	assert.Equal(t, GaugeMetric, m5.Type, "Type")
	assert.Len(t, m5.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", m5.Tags[0], "First tag")
	expected := float64(5.0 / ((1.0 / 5) + (1.0 / 10) + (1.0 / 15) + (1.0 / 20) + (1.0 / 25)))
	assert.Equal(t, expected, m5.Value, "Value")
}

func TestHistoSampleRate(t *testing.T) {
	h := NewHist("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", h.Name, "Name")
	assert.Len(t, h.Tags, 1, "Tag length")
	assert.Equal(t, h.Tags[0], "a:b", "Tag contents")

	h.Sample(5, 0.5)
	h.Sample(10, 0.5)
	h.Sample(15, 0.5)
	h.Sample(20, 0.5)
	h.Sample(25, 0.5)

	var aggregates HistogramAggregates
	aggregates.Value = AggregateMin | AggregateMax | AggregateCount
	aggregates.Count = 3

	metrics := h.Flush(10*time.Second, []float64{0.50}, aggregates, true)
	assert.Len(t, metrics, 4, "Metrics flush length")

	// First the max
	m1 := metrics[0]
	assert.Equal(t, "a.b.c.max", m1.Name, "Max name")
	assert.Equal(t, float64(25), m1.Value, "Sampled max as rate")

	count := metrics[2]
	assert.Equal(t, "a.b.c.count", count.Name, "count name")
	assert.Equal(t, float64(10), count.Value, "count value")
}

func TestHistoMerge(t *testing.T) {
	rand.Seed(time.Now().Unix())

	h := NewHist("a.b.c", []string{"a:b"})
	for i := 0; i < 100; i++ {
		h.Sample(rand.NormFloat64(), 1.0)
	}

	jm, err := h.Export()
	assert.NoError(t, err, "should have exported successfully")

	h2 := NewHist("a.b.c", []string{"a:b"})
	assert.NoError(t, h2.Combine(jm.Value), "should have combined successfully")
	assert.InEpsilon(t, h.Value.Quantile(0.5), h2.Value.Quantile(0.5), 0.02, "50th percentiles did not match after merging")
	assert.InDelta(t, 0, h2.LocalWeight, 0.02, "merged histogram should have count of zero")
	assert.True(t, math.IsInf(h2.LocalMin, +1), "merged histogram should have local minimum of +inf")
	assert.True(t, math.IsInf(h2.LocalMax, -1), "merged histogram should have local minimum of -inf")

	h2.Sample(1.0, 1.0)
	assert.InDelta(t, 1.0, h2.LocalWeight, 0.02, "merged histogram should have count of 1 after adding a value")
	assert.InDelta(t, 1.0, h2.LocalMin, 0.02, "merged histogram should have min of 1 after adding a value")
	assert.InDelta(t, 1.0, h2.LocalMax, 0.02, "merged histogram should have max of 1 after adding a value")
}

// Test the Metric and Merge function on Set
func TestHistoMergeMetric(t *testing.T) {
	rand.Seed(time.Now().Unix())

	h := NewHist("a.b.c", []string{"a:b"})
	for i := 0; i < 100; i++ {
		h.Sample(rand.NormFloat64(), 1.0)
	}

	m, err := h.Metric()
	assert.NoError(t, err, "should have created a metricpb.Metric from a Histo")

	h2 := NewHist("a.b.c", []string{"a:b"})
	h2.Merge(m.GetHistogram())
	assert.InEpsilon(t, h.Value.Quantile(0.5), h2.Value.Quantile(0.5), 0.02, "50th percentiles did not match after merging")
	assert.InDelta(t, 0, h2.LocalWeight, 0.02, "merged histogram should have count of zero")
	assert.True(t, math.IsInf(h2.LocalMin, +1), "merged histogram should have local minimum of +inf")
	assert.True(t, math.IsInf(h2.LocalMax, -1), "merged histogram should have local minimum of -inf")

	h2.Sample(1.0, 1.0)
	assert.InDelta(t, 1.0, h2.LocalWeight, 0.02, "merged histogram should have count of 1 after adding a value")
	assert.InDelta(t, 1.0, h2.LocalMin, 0.02, "merged histogram should have min of 1 after adding a value")
	assert.InDelta(t, 1.0, h2.LocalMax, 0.02, "merged histogram should have max of 1 after adding a value")
}

func TestMetricKeyEquality(t *testing.T) {
	c1 := NewCounter("a.b.c", []string{"a:b", "c:d"})
	ce1, _ := c1.Export()

	c2 := NewCounter("a.b.c", []string{"a:b", "c:d"})
	ce2, _ := c2.Export()

	c3 := NewCounter("a.b.c", []string{"c:d", "fart:poot"})
	ce3, _ := c3.Export()
	assert.Equal(t, ce1.JoinedTags, ce2.JoinedTags)
	// Make sure we can stringify that key and get equal!
	assert.Equal(t, ce1.MetricKey.String(), ce2.MetricKey.String())
	assert.NotEqual(t, ce1.MetricKey.String(), ce3.MetricKey.String())
}

func TestParseMetricSSF(t *testing.T) {
	val := rand.Float32()
	now := time.Now().Unix()
	sampleRate := rand.Float32()

	tests := []struct {
		sample   ssf.SSFSample
		expected UDPMetric
	}{
		{
			sample: ssf.SSFSample{
				Metric: ssf.SSFSample_GAUGE,

				Name:       "my.test.metric",
				Value:      val,
				Timestamp:  now,
				Message:    "arbitrary test message",
				Status:     ssf.SSFSample_WARNING,
				SampleRate: sampleRate,
				Tags: map[string]string{
					"keats":            "false",
					"yeats":            "false",
					"wilde":            "true",
					"veneurglobalonly": "true",
				},
				Unit: "frobs per second",
			},

			expected: UDPMetric{
				MetricKey: MetricKey{
					Name:       "my.test.metric",
					Type:       "gauge",
					JoinedTags: "keats:false,wilde:true,yeats:false",
				},
				Digest:     0x7ae783ad,
				Value:      val,
				SampleRate: sampleRate,
				Tags: []string{
					"keats:false",
					"wilde:true",
					"yeats:false",
				},
				Scope: GlobalOnly,
			},
		},
		{
			sample: ssf.SSFSample{
				Metric: ssf.SSFSample_GAUGE,

				Name:       "my.test.metric",
				Value:      val,
				Timestamp:  now,
				Message:    "arbitrary test message",
				Status:     ssf.SSFSample_WARNING,
				SampleRate: sampleRate,
				Tags: map[string]string{
					"keats":           "false",
					"yeats":           "false",
					"wilde":           "true",
					"veneurlocalonly": "true",
				},
				Unit: "frobs per second",
			},

			expected: UDPMetric{
				MetricKey: MetricKey{
					Name:       "my.test.metric",
					Type:       "gauge",
					JoinedTags: "keats:false,wilde:true,yeats:false",
				},
				Digest:     0x7ae783ad,
				Value:      val,
				SampleRate: sampleRate,
				Tags: []string{
					"keats:false",
					"wilde:true",
					"yeats:false",
				},
				Scope: LocalOnly,
			},
		},
		{
			sample: ssf.SSFSample{
				Metric: ssf.SSFSample_GAUGE,

				Name:       "my.test.metric",
				Value:      val,
				Timestamp:  now,
				Message:    "arbitrary test message",
				Status:     ssf.SSFSample_WARNING,
				SampleRate: sampleRate,
				Scope:      ssf.SSFSample_GLOBAL,
				Tags: map[string]string{
					"keats": "false",
					"yeats": "false",
					"wilde": "true",
				},
				Unit: "frobs per second",
			},

			expected: UDPMetric{
				MetricKey: MetricKey{
					Name:       "my.test.metric",
					Type:       "gauge",
					JoinedTags: "keats:false,wilde:true,yeats:false",
				},
				Digest:     0x7ae783ad,
				Value:      val,
				SampleRate: sampleRate,
				Tags: []string{
					"keats:false",
					"wilde:true",
					"yeats:false",
				},
				Scope: GlobalOnly,
			},
		},
		{
			sample: ssf.SSFSample{
				Metric: ssf.SSFSample_GAUGE,

				Name:       "my.test.metric",
				Value:      val,
				Timestamp:  now,
				Message:    "arbitrary test message",
				Status:     ssf.SSFSample_WARNING,
				SampleRate: sampleRate,
				Scope:      ssf.SSFSample_LOCAL,
				Tags: map[string]string{
					"keats": "false",
					"yeats": "false",
					"wilde": "true",
				},
				Unit: "frobs per second",
			},

			expected: UDPMetric{
				MetricKey: MetricKey{
					Name:       "my.test.metric",
					Type:       "gauge",
					JoinedTags: "keats:false,wilde:true,yeats:false",
				},
				Digest:     0x7ae783ad,
				Value:      val,
				SampleRate: sampleRate,
				Tags: []string{
					"keats:false",
					"wilde:true",
					"yeats:false",
				},
				Scope: LocalOnly,
			},
		},
	}

	for i, elt := range tests {
		sample := elt.sample
		expected := elt.expected
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			udpMetric, err := (&Parser{}).ParseMetricSSF(&sample)
			assert.NoError(t, err)
			assert.Equal(t, udpMetric.MetricKey, expected.MetricKey)
			assert.Equal(t, udpMetric.Type, expected.Type)
			assert.Equal(t, udpMetric.Digest, expected.Digest)
			assert.InEpsilon(t, udpMetric.Value, expected.Value, ε)
			assert.InEpsilon(t, udpMetric.SampleRate, expected.SampleRate, ε)
			assert.Equal(t, udpMetric.JoinedTags, expected.JoinedTags)
			assert.Equal(t, udpMetric.Tags, expected.Tags)
			assert.Equal(t, udpMetric.Scope, expected.Scope)
		})
	}
}

func BenchmarkParseMetricSSF(b *testing.B) {
	const LEN = 10000

	samples := make([]*ssf.SSFSample, LEN)

	for i := range samples {
		p := make([]byte, 10)
		_, err := rand.Read(p)
		if err != nil {
			b.Fatalf("Error generating data: %s", err)
		}
		sample := ssf.SSFSample{
			Name:       "my.test.metric",
			Value:      rand.Float32(),
			Timestamp:  time.Now().Unix(),
			SampleRate: rand.Float32(),
			Tags: map[string]string{
				"keats":       "false",
				"yeats":       "false",
				"wilde":       "true",
				string(p[:5]): string(p[5:]),
			},
		}
		samples[i] = &sample
	}
	p := Parser{}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ParseMetricSSF(samples[i%LEN])
	}
}

var benchResultInt int

func BenchmarkConvertIndicatorMetrics(b *testing.B) {
	explicitTags := []struct {
		name       string
		metricTags string
	}{
		{"ZeroTags", ""},
		{"OneTag", "baz:gorch"},
		{"TwoTags", "foo:bar,baz:gorch"},
	}
	implicitTags := []struct {
		name       string
		metricTags []string
	}{
		{"NoImplicit", []string{}},
		{"OneImplicit", []string{"six:6"}},
		{"SixImplicit", []string{"three:three", "six:6", "nine:9", "ten:10", "eleven:11", "twelve:12"}},
	}

	total := 0

	for _, it := range implicitTags {
		for _, et := range explicitTags {
			duration := 5 * time.Second
			start := time.Now()
			end := start.Add(duration)
			span := &ssf.SSFSpan{}
			span.Id = 1
			span.TraceId = 5
			span.Name = "foo"
			span.StartTimestamp = start.UnixNano()
			span.EndTimestamp = end.UnixNano()
			span.Indicator = true
			span.Service = "bar-srv"
			span.Metrics = make([]*ssf.SSFSample, 0)
			buff, err := proto.Marshal(span)
			assert.Nil(b, err)
			inSpan, err := protocol.ParseSSF(buff)
			if err != nil {
				b.Fatal(err)
			}

			benchName := fmt.Sprintf("%s%s", it.name, et.name)
			parser := NewParser(it.metricTags)
			b.Run(benchName, func(b *testing.B) {
				for n := 0; n < b.N; n++ {
					ums, err := parser.ConvertIndicatorMetrics(inSpan, "", "timer_name")
					if err != nil {
						b.Fatal(err)
					}
					total += len(ums)
				}
			})
		}
	}
	// Attempt to avoid compiler optimizations? Is this relevant?
	benchResultInt = total
}
