package debug

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

func MigrateConfig(conf *veneur.Config) {
	if conf.DebugFlushedMetrics {
		conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
			Kind: "debug",
			Name: "debug",
		})
	}
	if conf.DebugIngestedSpans {
		conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
			Kind: "debug",
			Name: "debug",
		})
	}
}

type debugMetricSink struct {
	log  *logrus.Entry
	mtx  *sync.Mutex
	name string
}

// Prevents debug sinks from logging at the same time.
var mtx = sync.Mutex{}

func ParseMetricConfig(
	_ string, _ interface{},
) (veneur.MetricSinkConfig, error) {
	return nil, nil
}

// CreateMetricSink creates a new debug sink for metrics. This function
// should match the signature of a value in veneur.MetricSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	return &debugMetricSink{
		log:  logger,
		mtx:  &mtx,
		name: name,
	}, nil
}

func (b *debugMetricSink) Name() string {
	return b.name
}

func (b *debugMetricSink) Kind() string {
	return "debug"
}

func (b *debugMetricSink) Start(*trace.Client) error {
	return nil
}

func (b *debugMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	if len(metrics) == 0 || b.log.Logger.Level < logrus.DebugLevel {
		return nil
	}
	b.mtx.Lock()
	defer b.mtx.Unlock()

	b.log.Debugf("Flushing %d metrics:", len(metrics))
	for _, m := range metrics {
		var msg = ""
		if m.Message != "" {
			msg = fmt.Sprintf("m:%q", m.Message)
		}
		b.log.Debugf("  %s: %s(%v) = %f%s", m.Type, m.Name, m.Tags, m.Value, msg)
	}
	return nil
}

func (b *debugMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
	if len(samples) == 0 || b.log.Logger.Level < logrus.DebugLevel {
		return
	}
	b.mtx.Lock()
	defer b.mtx.Unlock()

	b.log.Debugf("Flushing %d other samples:", len(samples))
	for _, m := range samples {
		var msg = ""
		if m.Message != "" {
			msg = fmt.Sprintf("m:%q", m.Message)
		}
		// TODO: more information about events
		b.log.Debugf("  %s: %s(%v) = %f%s", m.Metric.String(), m.Name, m.Tags, m.Value, msg)
	}
}

type debugSpanSink struct {
	log  *logrus.Entry
	mtx  *sync.Mutex
	name string
}

func ParseSpanConfig(_ string, _ interface{}) (veneur.SpanSinkConfig, error) {
	return nil, nil
}

// CreateSpanSink creates a new debug sink for spans. This function
// should match the signature of a value in veneur.SpanSinkTypes, and is
// intended to be passed into veneur.NewFromConfig to be called based on the
// provided configuration.
func CreateSpanSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	return &debugSpanSink{
		log:  logger,
		mtx:  &mtx,
		name: name,
	}, nil
}

func (b *debugSpanSink) Name() string {
	return b.name
}

// Start performs final adjustments on the sink.
func (b *debugSpanSink) Start(*trace.Client) error {
	return nil
}

func (b *debugSpanSink) Ingest(span *ssf.SSFSpan) error {
	if b.log.Logger.Level < logrus.DebugLevel {
		return nil
	}
	b.mtx.Lock()
	defer b.mtx.Unlock()

	info := ""
	if span.Indicator {
		info = info + "I"
	}
	if span.Error {
		info = info + "E"
	}
	if len(info) > 0 {
		info = " (" + info + ")"
	}

	duration := time.Duration(span.EndTimestamp - span.StartTimestamp)
	b.log.WithFields(logrus.Fields{
		"service":         span.Service,
		"validationError": protocol.ValidateTrace(span),
		"name":            span.Name,
		"traceId":         span.TraceId,
		"parentId":        span.ParentId,
		"id":              span.Id,
		"tags":            span.Tags,
		"start":           span.StartTimestamp,
		"duration":        duration,
		"info":            info,
		"numMetrics":      len(span.Metrics),
	}).Debug("Span")
	return nil
}

func (b *debugSpanSink) Flush() {
	// Do nothing.
}
