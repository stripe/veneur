package metricingester

import (
	"context"
	"sync"
	"time"

	"github.com/stripe/veneur/samplers"
)

type sinkFlusher struct {
	aggregates  samplers.HistogramAggregates
	percentiles []float64
	sinks       []Sink
}

type sinkFlusherOpt func(sinkFlusher) sinkFlusher

func optPercentiles(ps []float64) sinkFlusherOpt {
	return func(f sinkFlusher) sinkFlusher {
		f.percentiles = ps
		return f
	}
}

func optAggregates(as samplers.HistogramAggregates) sinkFlusherOpt {
	return func(f sinkFlusher) sinkFlusher {
		f.aggregates = as
		return f
	}
}

func newSinkFlusher(sinks []Sink, opts ...sinkFlusherOpt) sinkFlusher {
	sf := sinkFlusher{sinks: sinks}
	for _, opt := range opts {
		sf = opt(sf)
	}
	return sf
}

type Sink interface {
	Name() string
	Flush(context.Context, []samplers.InterMetric) error
}

func (s sinkFlusher) Flush(ctx context.Context, envelope samplerEnvelope) {
	metrics := make([]samplers.InterMetric, 0, countMetrics(envelope))
	// get metrics from envelope
	for _, sampler := range envelope.counters {
		metrics = append(metrics, sampler.Flush(time.Second)...)
	}
	for _, sampler := range envelope.sets {
		metrics = append(metrics, sampler.Flush()...)
	}
	for _, sampler := range envelope.gauges {
		metrics = append(metrics, sampler.Flush()...)
	}
	for _, sampler := range envelope.histograms {
		metrics = append(metrics, sampler.Flush(time.Second, s.percentiles, s.aggregates, true)...)
	}
	for _, sampler := range envelope.mixedHistograms {
		metrics = append(metrics, sampler.Flush(time.Second, s.percentiles, s.aggregates, true)...)
	}

	if len(metrics) == 0 {
		return
	}

	// TODO(clin): Add back metrics once we finalize the metrics client pull request.
	wg := sync.WaitGroup{}
	for _, sinkInstance := range s.sinks {
		wg.Add(1)
		go func(ms Sink) {
			err := ms.Flush(ctx, metrics)
			if err != nil {
				//s.log.WithError(err).WithField("Sink", ms.Name()).Warn("Error flushing Sink")
			}
			wg.Done()
		}(sinkInstance)
	}
	wg.Wait()
	return
}

func countMetrics(samplers samplerEnvelope) (count int) {
	// This is a minor optimization to reduce allocations produced by append statements.
	// We just need to get order of magnitude right, so this isn't super precise, and that's probably ok.
	//
	// If we need to be close to zeroalloc for some reason we can come back and make this perfect.
	count += len(samplers.counters)
	count += len(samplers.gauges)
	count += len(samplers.sets)
	count += len(samplers.histograms) * 5
	// probably way off
	count += len(samplers.mixedHistograms) * 5 * 10
	return count
}
