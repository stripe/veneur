package metricingester

import (
	"context"
	"strconv"

	"github.com/opentracing/opentracing-go"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/trace"
)

func traceLogger(log *logrus.Logger, ctx context.Context) *logrus.Entry {
	if span, ok := opentracing.SpanFromContext(ctx).(*trace.Span); ok {
		return log.
			WithField("trace_id", strconv.FormatInt(span.TraceID, 16)).
			WithField("span_id", strconv.FormatInt(span.SpanID, 16))
	}
	return log.WithField("trace_id", "<UNKNOWN>")
}
