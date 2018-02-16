package samplers

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/tdigest"
)

func TestRouting(t *testing.T) {
	tests := []struct {
		name      string
		tags      []string
		sinks     RouteInformation
		sinkNames []string
	}{
		{"none specified", []string{"foo:bar", "veneurlocalonly"}, nil, []string{"foosink", "barsink"}},
		{"one sink", []string{"veneursinkonly:foobar"}, map[string]struct{}{"foobar": struct{}{}}, []string{"foobar"}},
		{
			"multiple sinks",
			[]string{"veneursinkonly:foobar", "veneursinkonly:baz"},
			map[string]struct{}{"foobar": struct{}{}, "baz": struct{}{}},
			[]string{"foobar", "baz"},
		},
	}
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

func TestSet(t *testing.T) {
	s := NewSet("a.b.c", []string{"a:b"})

	assert.Equal(t, "a.b.c", s.Name, "Name")
	assert.Len(t, s.Tags, 1, "Tag count")
	assert.Equal(t, "a:b", s.Tags[0], "First tag")

	s.Sample("5", 1.0)

	s.Sample("5", 1.0)

	s.Sample("123", 1.0)

	s.Sample("2147483647", 1.0)
	s.Sample("-2147483648", 1.0)

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
		s.Sample(strconv.Itoa(rand.Int()), 1.0)
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

	metrics := h.Flush(10*time.Second, percentiles, aggregates)
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

	metrics := h.Flush(10*time.Second, percentiles, aggregates)
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

	metrics := h.Flush(10*time.Second, percentiles, aggregates)
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

	metrics := h.Flush(10*time.Second, []float64{0.50}, aggregates)
	assert.Len(t, metrics, 4, "Metrics flush length")

	// First the max
	m1 := metrics[0]
	assert.Equal(t, "a.b.c.max", m1.Name, "Max name")
	assert.Equal(t, float64(25), m1.Value, "Sampled max as rate")

	count := metrics[2]
	assert.Equal(t, "a.b.c.count", count.Name, "count name")
	assert.Equal(t, float64(10), count.Value, "count value")
}

func TestHistoMergeOneSideOnly(t *testing.T) {
	rand.Seed(time.Now().Unix())

	h := histoWithSamples("a.b.c", []string{"a:b"}, 100)
	jm, err := h.Export()
	assert.NoError(t, err, "should have exported successfully")

	h2 := NewHist("a.b.c", []string{"a:b"})
	assert.NoError(t, h2.Combine(jm.Value), "should have combined successfully")
	assert.InEpsilon(t, h.tDigest.Quantile(0.5), h2.tDigest.Quantile(0.5), 0.02, "50th percentiles did not match after merging")
	assert.Equal(t, h.weight, h2.weight, "merged histogram should have the weight of the other")
	assert.Equal(t, h.min, h2.min, "merged histogram should have the min of the other")
	assert.Equal(t, h.max, h2.max, "merged histogram should have the max of the other")
	assert.Equal(t, h.sum, h2.sum, "merged histogram should have the sum of the other")
	assert.Equal(t, h.reciprocalSum, h2.reciprocalSum, "merged histogram should have the reciprocal sum of the other")
}

func TestHistoMergeBoth(t *testing.T) {
	rand.Seed(time.Now().Unix())

	h := &Histo{
		Name:          "a.b",
		tDigest:       tdigest.NewMerging(100, false),
		min:           1,
		max:           10,
		sum:           1,
		weight:        1,
		reciprocalSum: 1,
	}

	type histValues struct {
		weight        float64
		min           float64
		max           float64
		sum           float64
		reciprocalSum float64
	}

	testCases := []struct {
		description string
		toCombine   *Histo
		expected    histValues
	}{
		{
			description: "values on both sides",
			toCombine: &Histo{
				tDigest:       tdigest.NewMerging(100, false),
				min:           0,
				max:           9,
				sum:           2,
				weight:        2,
				reciprocalSum: 2,
			},
			expected: histValues{
				weight:        3,
				min:           0,
				max:           10,
				sum:           3,
				reciprocalSum: 3,
			},
		},
		{
			description: "one side uninitialized",
			toCombine:   NewHist("b.c", []string{}),
			expected: histValues{
				weight:        h.weight,
				min:           h.min,
				max:           h.max,
				sum:           h.sum,
				reciprocalSum: h.reciprocalSum,
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.description, func(t *testing.T) {
			jm, err := h.Export()
			assert.NoError(t, err, "the histo should have exported successfully")
			assert.NoError(t, tc.toCombine.Combine(jm.Value), "combining the two should have succeeded")

			assert.Equal(t, tc.expected.min, tc.toCombine.min,
				"the merged min should be correct")
			assert.Equal(t, tc.expected.weight, tc.toCombine.weight,
				"merged histogram should have the weight of the other")
			assert.Equal(t, tc.expected.min, tc.toCombine.min,
				"merged histogram should have the min of the other")
			assert.Equal(t, tc.expected.max, tc.toCombine.max,
				"merged histogram should have the max of the other")
			assert.Equal(t, tc.expected.sum, tc.toCombine.sum,
				"merged histogram should have the sum of the other")
			assert.Equal(t, tc.expected.reciprocalSum, tc.toCombine.reciprocalSum,
				"merged histogram should have the reciprocal sum of the other")
		})
	}
}

// This test can be removed once the code handling the old binary format in
// Histo.Combine is removed.
func TestHistoCombineOldFormat(t *testing.T) {
	rand.Seed(time.Now().Unix())

	h := histoWithSamples("a.b", []string{}, 0)
	other := histoWithSamples("a.b", []string{}, 100)

	// Set these parameters to different values between the two
	for i, hist := range []*Histo{h, other} {
		val := float64(i + 1)
		hist.weight = val
		hist.min = val
		hist.max = val
		hist.sum = val
		hist.reciprocalSum = val
	}

	encoded, err := other.tDigest.GobEncode()
	assert.NoError(t, err, "the other histogram's tdigest should have been encoded")

	err = h.Combine(encoded)
	assert.NoError(t, err, "combining with the binary tdigest should have succeeded")

	// The tdigests should have been merged
	assert.Equal(t, other.tDigest.Quantile(0.5), h.tDigest.Quantile(0.5),
		"the combined histogram should have the median of the merged tdigest")

	// All the other values should've been unchanged
	expected := float64(1)
	assert.Equal(t, expected, h.weight, "the weight shouldn't have changed")
	assert.Equal(t, expected, h.min, "the min shouldn't have changed")
	assert.Equal(t, expected, h.max, "the max shouldn't have changed")
	assert.Equal(t, expected, h.sum, "the sum shouldn't have changed")
	assert.Equal(t, expected, h.reciprocalSum, "the reciprocal sum shouldn't have changed")
}

func TestHistoCombineInvalidData(t *testing.T) {
	h := NewHist("a.b", []string{})
	err := h.Combine([]byte("invalid data"))
	assert.Error(t, err, "combining with invalid data should've failed")
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

func BenchmarkHistoExport(b *testing.B) {
	rand.Seed(time.Now().Unix())

	for _, samples := range []int{10, 100, 1000, 10000} {
		h := histoWithSamples("a.b.c", []string{"a:b"}, samples)
		b.Run(fmt.Sprintf("Samples=%d", samples), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if _, err := h.Export(); err != nil {
					b.Fatalf("Failed to export a histogram: %v", err)
				}
			}
		})
	}
}

func BenchmarkHistoCombine(b *testing.B) {
	rand.Seed(time.Now().Unix())

	for _, samples := range []int{10, 100, 1000, 10000} {
		h := histoWithSamples("a.b.c", []string{"a:b"}, samples)

		other := histoWithSamples("c.d.e", []string{"c:d"}, samples)
		marshalled, err := other.Export()
		assert.NoError(b, err, "should have exported successfully")

		b.Run(fmt.Sprintf("Samples=%d", samples), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := h.Combine(marshalled.Value); err != nil {
					b.Fatalf("Failed to combine with the other histogram: %v",
						err)
				}
			}
		})
	}
}

// This can be removed once the code handling the old binary format in
// Histo.Combine is removed.
func BenchmarkHistoCombineOldFormat(b *testing.B) {
	rand.Seed(time.Now().Unix())

	for _, samples := range []int{10, 100, 1000, 10000} {
		h := histoWithSamples("a.b.c", []string{"a:b"}, samples)

		other := histoWithSamples("c.d.e", []string{"c:d"}, samples)
		encoded, err := other.tDigest.GobEncode()
		assert.NoError(b, err, "the other tdigest should have been encoded successfully")

		b.Run(fmt.Sprintf("Samples=%d", samples), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if err := h.Combine(encoded); err != nil {
					b.Fatalf("Failed to combine with a previous-format "+
						"(gob-encoded) histogram: %v", err)
				}
			}
		})
	}
}

func histoWithSamples(name string, tags []string, samples int) *Histo {
	h := NewHist(name, tags)
	for i := 0; i < samples; i++ {
		h.Sample(rand.NormFloat64(), 1.0)
	}
	return h
}
