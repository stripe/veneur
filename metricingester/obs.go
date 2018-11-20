package metricingester

import (
	"context"

	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func traceLogger(log *logrus.Logger, ctx context.Context) *logrus.Entry {
	if span, ok := opentracing.SpanFromContext(ctx).(*trace.Span); ok {
		return log.
			WithField("trace_id", span.TraceID).
			WithField("span_id", span.SpanID)
	}
	return log.WithField("trace_id", "<UNKNOWN>")
}

type mclient interface {
	Add(...*ssf.SSFSample)
}

func traceMetrics(ctx context.Context) mclient {
	if span, ok := opentracing.SpanFromContext(ctx).(*trace.Span); ok {
		return span
	}
	// this is essentially a noop client right now--we must make sure the context has a span object
	// to report metrics.
	return &ssf.Samples{}
}
