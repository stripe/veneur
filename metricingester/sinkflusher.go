package metricingester

import (
	"context"
	"fmt"
	"time"

	"github.com/stripe/veneur/trace/metrics"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type sinkFlusher struct {
	aggregates  samplers.HistogramAggregates
	percentiles []float64
	sinks       []Sink
	log         *logrus.Logger
	tc          *trace.Client
}

type Sink interface {
	Name() string
	Flush(context.Context, []samplers.InterMetric) error
}

func (s sinkFlusher) Flush(ctx context.Context, envelope samplerEnvelope) {
	logger := traceLogger(s.log, ctx)
	ms := make([]samplers.InterMetric, 0, countMetrics(envelope))
	// get ms from envelope
	for _, sampler := range envelope.counters {
		ms = append(ms, sampler.Flush(time.Second)...)
	}
	for _, sampler := range envelope.sets {
		ms = append(ms, sampler.Flush()...)
	}
	for _, sampler := range envelope.gauges {
		ms = append(ms, sampler.Flush()...)
	}
	for _, sampler := range envelope.statusChecks {
		ms = append(ms, sampler.Flush()...)
	}
	for _, sampler := range envelope.histograms {
		ms = append(ms, sampler.Flush(time.Second, s.percentiles, s.aggregates, true)...)
	}
	for _, sampler := range envelope.mixedHistograms {
		ms = append(ms, sampler.Flush(s.percentiles, s.aggregates, envelope.mixedHosts)...)
	}

	if len(ms) == 0 {
		return
	}

	tags := map[string]string{"part": "post"}
	for _, sinkInstance := range s.sinks {
		go func(sink Sink) {
			samples := &ssf.Samples{}
			defer metrics.Report(s.tc, samples)
			start := time.Now()
			err := sink.Flush(ctx, ms)
			if err != nil {
				samples.Add(
					ssf.Count(fmt.Sprintf("flush.plugins.%s.error_total", sink.Name()), 1, nil),
				)
				logger.WithError(err).WithField("Sink", sink.Name()).Warn("Error flushing Sink")
			}
			samples.Add(
				ssf.Timing(
					fmt.Sprintf("flush.plugins.%s.total_duration_ns", sink.Name()),
					time.Since(start),
					time.Nanosecond,
					tags,
				),
				ssf.Gauge(fmt.Sprintf("flush.plugins.%s.post_metrics_total", sink.Name()), float32(len(ms)), nil),
			)
		}(sinkInstance)
	}
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
