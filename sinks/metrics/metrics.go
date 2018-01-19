// Package metrics provides sinks that are used by veneur internally.
package metrics

import (
	"fmt"
	"sync/atomic"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type metricExtractionSink struct {
	workers                []Processor
	indicatorSpanTimerName string
	log                    *logrus.Logger
	statsd                 *statsd.Client
	spansProcessed         int64
	metricsGenerated       int64
}

var _ sinks.SpanSink = &metricExtractionSink{}

// Processor represents a thing that can process UDPMetrics.
type Processor interface {
	// IngestUDP takes a single UDPMetric and processes it in the worker.
	IngestUDP(samplers.UDPMetric)
}

// NewMetricExtractionSink sets up and creates a span sink that
// extracts metrics ("samples") from SSF spans and reports them to a
// veneur's metrics workers.
func NewMetricExtractionSink(mw []Processor, timerName string, statsd *statsd.Client, log *logrus.Logger) (sinks.SpanSink, error) {
	return &metricExtractionSink{
		workers:                mw,
		indicatorSpanTimerName: timerName,
		statsd:                 statsd,
		log:                    log,
	}, nil
}

// Name returns "metric_extraction".
func (m metricExtractionSink) Name() string {
	return "metric_extraction"
}

// Start is a no-op.
func (m metricExtractionSink) Start(*trace.Client) error {
	return nil
}

func (m *metricExtractionSink) sendMetrics(metrics []samplers.UDPMetric) {
	for _, metric := range metrics {
		m.workers[metric.Digest%uint32(len(m.workers))].IngestUDP(metric)
	}
}

func (m *metricExtractionSink) sendSample(sample *ssf.SSFSample) error {
	metric, err := samplers.ParseMetricSSF(sample)
	if err != nil {
		return err
	}
	m.sendMetrics([]samplers.UDPMetric{metric})
	return nil
}

// Ingest extracts metrics from an SSF span, and feeds them into the
// appropriate metric sinks.
func (m metricExtractionSink) Ingest(span *ssf.SSFSpan) error {
	metrics, err := samplers.ConvertMetrics(span)
	if err != nil {
		if _, ok := err.(samplers.InvalidMetrics); ok {
			m.log.WithError(err).
				Warn("Could not parse metrics from SSF Message")
			m.sendSample(ssf.Count("veneur.ssf.error_total", 1, map[string]string{
				"packet_type": "ssf_metric",
				"step":        "extract_metrics",
				"reason":      "invalid_metrics",
			}))
		} else {
			m.log.WithError(err).Error("Unexpected error extracting metrics from SSF Message")
			m.sendSample(ssf.Count("veneur.ssf.error_total", 1, map[string]string{
				"packet_type": "ssf_metric",
				"step":        "extract_metrics",
				"reason":      "unexpected_error",
				"error":       err.Error(),
			}))
			return err
		}
	}
	atomic.AddInt64(&m.spansProcessed, 1)
	atomic.AddInt64(&m.metricsGenerated, int64(len(metrics)))
	m.sendMetrics(metrics)

	if err := protocol.ValidateTrace(span); err != nil {
		return err
	}

	indicatorMetrics, err := samplers.ConvertIndicatorMetrics(span, m.indicatorSpanTimerName)
	if err != nil {
		m.log.WithError(err).
			WithField("span_name", span.Name).
			Warn("Couldn't extract indicator metrics for span")
		return err
	}
	m.sendMetrics(indicatorMetrics)
	return nil
}

func (m metricExtractionSink) Flush() {
	m.statsd.Count(sinks.MetricKeyTotalSpansFlushed, atomic.LoadInt64(&m.spansProcessed), []string{fmt.Sprintf("sink:%s", m.Name())}, 1.0)
	m.statsd.Count(sinks.MetricKeyTotalMetricsFlushed, atomic.LoadInt64(&m.metricsGenerated), []string{fmt.Sprintf("sink:%s", m.Name())}, 1.0)

	atomic.SwapInt64(&m.spansProcessed, 0)
	atomic.SwapInt64(&m.metricsGenerated, 0)
	return
}
