package debug

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/samplers"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

type debugMetricSink struct {
	log *logrus.Logger
	mtx *sync.Mutex
}

var _ sinks.MetricSink = &debugMetricSink{}

func NewDebugMetricSink(mtx *sync.Mutex, log *logrus.Logger) sinks.MetricSink {
	return &debugMetricSink{log, mtx}
}

func (b *debugMetricSink) Name() string {
	return "blackhole"
}

func (b *debugMetricSink) Start(*trace.Client) error {
	return nil
}

func (b *debugMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	if len(metrics) == 0 || b.log.Level < logrus.DebugLevel {
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
	if len(samples) == 0 || b.log.Level < logrus.DebugLevel {
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
	return
}

type debugSpanSink struct {
	log *logrus.Logger
	mtx *sync.Mutex
}

var _ sinks.SpanSink = &debugSpanSink{}

func NewDebugSpanSink(mtx *sync.Mutex, log *logrus.Logger) sinks.SpanSink {
	return &debugSpanSink{log, mtx}
}

func (b *debugSpanSink) Name() string {
	return "debug"
}

// Start performs final adjustments on the sink.
func (b *debugSpanSink) Start(*trace.Client) error {
	return nil
}

func (b *debugSpanSink) Ingest(span *ssf.SSFSpan) error {
	if b.log.Level < logrus.DebugLevel {
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
	b.log.Debugf("Span %s:%s(%v) %x/%x/%x %v+%v%s (m:%d)",
		span.Service, span.Name, span.Tags,
		span.TraceId, span.ParentId, span.Id,
		span.StartTimestamp, duration,
		info, len(span.Metrics),
	)
	return nil
}

func (b *debugSpanSink) Flush() {
	return
}
