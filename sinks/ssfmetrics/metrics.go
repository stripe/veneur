// Package ssfmetrics provides sinks that are used by veneur internally.
package ssfmetrics

import (
	"context"
	"sync/atomic"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
)

// metricExtractionSink enqueues ssf spans or udp metrics for processing in the next pipeline iteration.
type metricExtractionSink struct {
	workers                []Processor
	indicatorSpanTimerName string
	log                    *logrus.Logger
	traceClient            *trace.Client
	spansProcessed         int64
	metricsGenerated       int64
}

var _ sinks.SpanSink = &metricExtractionSink{}

// Processor represents a thing that can process UDPMetrics.
type Processor interface {
	// IngestUDP takes a single UDPMetric and processes it in the worker.
	IngestUDP(samplers.UDPMetric)
}

// DerivedMetricsSink composes the functionality of a SpanSink and DerivedMetricsProcessor
type DerivedMetricsSink interface {
	sinks.SpanSink
	samplers.DerivedMetricsProcessor
}

// NewMetricExtractionSink sets up and creates a span sink that
// extracts metrics ("samples") from SSF spans and reports them to a
// veneur's metrics workers.
func NewMetricExtractionSink(mw []Processor, timerName string, cl *trace.Client, log *logrus.Logger) (DerivedMetricsSink, error) {
	return &metricExtractionSink{
		workers:                mw,
		indicatorSpanTimerName: timerName,
		traceClient:            cl,
		log:                    log,
	}, nil
}

// Name returns "metric_extraction".
func (m *metricExtractionSink) Name() string {
	return "metric_extraction"
}

// Start is a no-op.
func (m *metricExtractionSink) Start(*trace.Client) error {
	return nil
}

// sendMetrics enqueues the metrics into the worker channels
func (m *metricExtractionSink) sendMetrics(metrics []samplers.UDPMetric) {
	for _, metric := range metrics {
		m.workers[metric.Digest%uint32(len(m.workers))].IngestUDP(metric)
	}
}

func (m *metricExtractionSink) SendSample(sample *ssf.SSFSample) error {
	metric, err := samplers.ParseMetricSSF(sample)
	if err != nil {
		return err
	}
	m.sendMetrics([]samplers.UDPMetric{metric})
	return nil
}

// Ingest extracts metrics from an SSF span, and feeds them into the
// appropriate metric sinks.
func (m *metricExtractionSink) Ingest(span *ssf.SSFSpan) error {
	var metricsCount int
	defer func() {
		atomic.AddInt64(&m.metricsGenerated, int64(metricsCount))
		atomic.AddInt64(&m.spansProcessed, 1)
	}()
	metrics, err := samplers.ConvertMetrics(span)
	if err != nil {
		if _, ok := err.(samplers.InvalidMetrics); ok {
			m.log.WithError(err).
				Warn("Could not parse metrics from SSF Message")
			m.SendSample(ssf.Count("ssf.error_total", 1, map[string]string{
				"packet_type": "ssf_metric",
				"step":        "extract_metrics",
				"reason":      "invalid_metrics",
			}))
		} else {
			m.log.WithError(err).Error("Unexpected error extracting metrics from SSF Message")
			m.SendSample(ssf.Count("ssf.error_total", 1, map[string]string{
				"packet_type": "ssf_metric",
				"step":        "extract_metrics",
				"reason":      "unexpected_error",
				"error":       err.Error(),
			}))
			return err
		}
	}
	metricsCount += len(metrics)
	m.sendMetrics(metrics)

	if err := protocol.ValidateTrace(span); err != nil {
		return err
	}
	// If we made it here, we are dealing with a fully-fledged
	// trace span, not just a mere carrier for Samples:

	indicatorMetrics, err := samplers.ConvertIndicatorMetrics(span, m.indicatorSpanTimerName)
	if err != nil {
		m.log.WithError(err).
			WithField("span_name", span.Name).
			Warn("Couldn't extract indicator metrics for span")
		return err
	}
	metricsCount += len(indicatorMetrics)

	spanMetrics, err := samplers.ConvertSpanUniquenessMetrics(span, 0.01)
	if err != nil {
		m.log.WithError(err).
			WithField("span_name", span.Name).
			Warn("Couldn't extract uniqueness metrics for span")
		return err
	}
	metricsCount += len(spanMetrics)

	m.sendMetrics(append(indicatorMetrics, spanMetrics...))
	return nil
}

func (m *metricExtractionSink) Flush(context.Context) {
	tags := map[string]string{"sink": m.Name()}
	metrics.ReportBatch(m.traceClient, []*ssf.SSFSample{
		ssf.Count(sinks.MetricKeyTotalSpansFlushed, float32(atomic.SwapInt64(&m.spansProcessed, 0)), tags),
		ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(atomic.SwapInt64(&m.metricsGenerated, 0)), tags),
	})
}
