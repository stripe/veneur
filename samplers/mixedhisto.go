package samplers

import (
	"fmt"
	"math"
	"time"

	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/tdigest"
)

// NewMixedHisto creates a new mixed histogram.
func NewMixedHisto(name string, tags []string) MixedHisto {
	return MixedHisto{
		histo:         NewHist(name, tags),
		min:           make(map[string]float64),
		max:           make(map[string]float64),
		weight:        make(map[string]float64),
		sum:           make(map[string]float64),
		reciprocalSum: make(map[string]float64),
	}
}

// MixedHisto is a sampler for the MixedScope histogram case.
//
// In other words, it tracks min, max, count, sum, and harmonic mean for each host
// separately but aggregates all percentiles globally.
//
// In the mixed scope case, histogram behavior is to emit summary statistics that
// can be aggregated without bias at the host level, but emit all other metrics
// (namely, percentiles) at the global level.
//
// Note that we don't support the "median" aggregate for mixed histograms.
type MixedHisto struct {
	histo         *Histo
	min           map[string]float64
	max           map[string]float64
	weight        map[string]float64
	sum           map[string]float64
	reciprocalSum map[string]float64
}

// Sample accepts a new data point for the histogram.
func (m MixedHisto) Sample(sample float64, sampleRate float32, host string) {
	m.histo.Sample(sample, sampleRate)

	weight := float64(1 / sampleRate)
	m.min[host] = math.Min(sample, getDefault(m.min, host, math.Inf(+1)))
	m.max[host] = math.Max(sample, getDefault(m.max, host, math.Inf(-1)))
	m.weight[host] += weight
	m.sum[host] += sample * weight
	m.reciprocalSum[host] += (1 / sample) * weight
}

// Flush returns metrics for the specified percentiles and aggregates.
func (m MixedHisto) Flush(percentiles []float64, aggregates HistogramAggregates) []InterMetric {
	ms := m.histo.Flush(0, percentiles, HistogramAggregates{}, false)

	// doesn't support median! Would require implementing separated digests.
	now := time.Now().Unix()
	metric := func(suffix string, val float64, host string) InterMetric {
		return InterMetric{
			Name:      fmt.Sprintf("%s.%s", m.histo.Name, suffix),
			Timestamp: now,
			Value:     val,
			Tags:      m.histo.Tags,
			HostName:  host,
			Type:      GaugeMetric,
			Sinks:     routeInfo(m.histo.Tags),
		}
	}
	for host, _ := range m.max {
		if (aggregates.Value & AggregateMax) != 0 {
			ms = append(ms, metric("max", m.max[host], host))
		}
		if (aggregates.Value & AggregateMin) != 0 {
			ms = append(ms, metric("min", m.min[host], host))
		}
		if (aggregates.Value & AggregateSum) != 0 {
			ms = append(ms, metric("sum", m.sum[host], host))
		}
		if (aggregates.Value & AggregateAverage) != 0 {
			ms = append(ms, metric("avg", m.sum[host]/m.weight[host], host))
		}
		if (aggregates.Value & AggregateCount) != 0 {
			ms = append(ms, metric("count", m.weight[host], host))
		}
		if (aggregates.Value & AggregateHarmonicMean) != 0 {
			ms = append(ms, metric("hmean", m.weight[host]/m.reciprocalSum[host], host))
		}
	}
	return ms
}

func getDefault(m map[string]float64, key string, def float64) float64 {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// Merge merges in a histogram.
func (m MixedHisto) Merge(host string, v *metricpb.HistogramValue) {
	merging := tdigest.NewMergingFromData(v.GetTDigest())
	m.histo.Value.Merge(merging)

	m.min[host] = math.Min(merging.Min(), getDefault(m.min, host, math.Inf(+1)))
	m.max[host] = math.Max(merging.Max(), getDefault(m.max, host, math.Inf(-1)))
	m.weight[host] += merging.Count()
	m.sum[host] += merging.Sum()
	m.reciprocalSum[host] += merging.ReciprocalSum()
}
