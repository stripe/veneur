package samplers

import (
	"testing"

	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/tdigest"

	"github.com/stretchr/testify/assert"
)

type sampleCase struct {
	host string
	val  float64
}

var sampleCases = []struct {
	aggregates  Aggregate
	percentiles []float64
	in          []sampleCase
	out         []TestMetric
	msg         string
}{
	{
		AggregateMin,
		[]float64{},
		[]sampleCase{{"", -2}, {"", 4}, {"", 2}},
		TMetrics(
			TGauge("test.min", -2),
		),
		"wrong min reported",
	},
	{
		AggregateSum,
		[]float64{},
		[]sampleCase{{"", 1}, {"", 2}, {"", 3}},
		TMetrics(TGauge("test.sum", 6)),
		"wrong sum reported",
	},
	{
		AggregateMax,
		[]float64{},
		[]sampleCase{{"", -2}, {"", -4}, {"", -1}},
		TMetrics(TGauge("test.max", -1)),
		"wrong max reported",
	},
	{
		AggregateMin | AggregateMax | AggregateAverage | AggregateCount | AggregateSum,
		[]float64{},
		[]sampleCase{{"", 1}, {"", 2}, {"", 3}},
		TMetrics(
			TGauge("test.min", 1),
			TGauge("test.max", 3),
			TGauge("test.avg", 2),
			TGauge("test.count", 3),
			TGauge("test.sum", 6),
		),
		"an aggregate value is incorrect",
	},
	{
		AggregateHarmonicMean,
		[]float64{},
		[]sampleCase{{"", 2}, {"", 4}, {"", 2}},
		TMetrics(
			TGauge("test.hmean", 2.4),
		),
		"harmonic mean is incorrect",
	},
	{
		0,
		[]float64{.5, .99},
		[]sampleCase{{"", 1}, {"", 2}, {"", 3}, {"", 4}, {"", 5}},
		TMetrics(
			TGauge("test.50percentile", 3),
			TGauge("test.99percentile", 4.975),
		),
		"percentiles are incorrect",
	},
	{
		AggregateMin,
		[]float64{.5},
		[]sampleCase{{"a", 1}, {"b", 2}, {"c", 3}},
		TMetrics(
			TGauge("test.50percentile", 2),
			TGauge("test.min", 1, OptHostname("a")),
			TGauge("test.min", 2, OptHostname("b")),
			TGauge("test.min", 3, OptHostname("c")),
		),
		"aggregates reported separately",
	},
}

func TestMixedHistoSample(t *testing.T) {
	for _, c := range sampleCases {
		t.Run(c.msg, testSample(c.percentiles, c.aggregates, c.in, c.out))
	}
}

func testSample(ps []float64, aggs Aggregate, inms []sampleCase, ts []TestMetric) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		mh := NewMixedHisto("test", nil)
		for _, inm := range inms {
			mh.Sample(inm.val, 1, inm.host)
		}

		results := ToTestMetrics(mh.Flush(ps, HistogramAggregates{aggs, 0}))
		assert.ElementsMatch(
			t,
			results,
			ts,
			"EXPECTED: %v\nACTUAL:%v", ts, results,
		)
	}
}

type mergeCase struct {
	samples []float64
	host    string
}

var mergeCases = []struct {
	aggregates  Aggregate
	percentiles []float64
	cases       []mergeCase
	out         []TestMetric
	msg         string
}{
	{
		AggregateMin,
		nil,
		[]mergeCase{
			{
				[]float64{2, 2, 2},
				"",
			},
			{
				[]float64{1, 2, 3},
				"",
			},
		},
		TMetrics(TGauge("test.min", 1)),
		"positive min is incorrect",
	},
	{
		AggregateMin,
		nil,
		[]mergeCase{
			{
				[]float64{2, 2, 2},
				"",
			},
			{
				[]float64{1, 2, -3},
				"",
			},
		},
		TMetrics(TGauge("test.min", -3)),
		"negative min is incorrect",
	},
	{
		AggregateMax,
		nil,
		[]mergeCase{
			{
				[]float64{2, 2, 2},
				"",
			},
			{
				[]float64{1, 2, -3},
				"",
			},
		},
		TMetrics(TGauge("test.max", 2)),
		"positive max is incorrect",
	},
	{
		AggregateMax,
		nil,
		[]mergeCase{
			{
				[]float64{-2, -2, -2},
				"",
			},
			{
				[]float64{-1, -2, -3},
				"",
			},
		},
		TMetrics(TGauge("test.max", -1)),
		"negative max is incorrect",
	},
	{
		AggregateCount | AggregateAverage,
		nil,
		[]mergeCase{
			{
				[]float64{1, 2, 3},
				"",
			},
			{
				[]float64{4, 5},
				"",
			},
		},
		TMetrics(
			TGauge("test.avg", 3),
			TGauge("test.count", 5),
		),
		"count or average is incorrect",
	},
	{
		AggregateHarmonicMean,
		nil,
		[]mergeCase{
			{
				[]float64{2},
				"",
			},
			{
				[]float64{4, 2},
				"",
			},
		},
		TMetrics(
			TGauge("test.hmean", 2.4),
		),
		"harmonic mean is incorrect",
	},
	{
		0,
		[]float64{.5, .99},
		[]mergeCase{
			{
				[]float64{3, 5},
				"",
			},
			{
				[]float64{1, 2, 4},
				"",
			},
		},
		TMetrics(
			TGauge("test.50percentile", 3),
			TGauge("test.99percentile", 4.975),
		),
		"percentiles are incorrect",
	},
	{
		AggregateMin,
		[]float64{.5},
		[]mergeCase{
			{
				[]float64{3, 5},
				"",
			},
			{
				[]float64{1, 2},
				"a",
			},
			{
				[]float64{4},
				"b",
			},
		},
		TMetrics(
			TGauge("test.50percentile", 3),
			TGauge("test.min", 3),
			TGauge("test.min", 1, OptHostname("a")),
			TGauge("test.min", 4, OptHostname("b")),
		),
		"aggregates not aggregated separately for each hostname",
	},
}

func TestMixedHistoMerge(t *testing.T) {
	for _, c := range mergeCases {
		t.Run(c.msg, testMerge(c.percentiles, c.aggregates, c.cases, c.out))
	}
}

func testMerge(ps []float64, aggs Aggregate, mergeCase []mergeCase, ts []TestMetric) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		mh := NewMixedHisto("test", nil)
		for _, c := range mergeCase {
			mh.Merge(c.host, histvalue(c.samples))
		}

		results := ToTestMetrics(mh.Flush(ps, HistogramAggregates{aggs, 0}))
		assert.ElementsMatch(
			t,
			results,
			ts,
			"EXPECTED: %v\nACTUAL:%v", ts, results,
		)
	}
}

func histvalue(samples []float64) *metricpb.HistogramValue {
	td := tdigest.NewMerging(1000, true)
	for _, s := range samples {
		td.Add(s, 1.0)
	}
	return &metricpb.HistogramValue{td.Data()}
}
