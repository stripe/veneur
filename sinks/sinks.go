package sinks

import (
	"context"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

// MetricKeyMetricFlushDuration is emitted as a timer for each sink by the
// flusher, tagged with the name for the sink.
const MetricKeyMetricFlushDuration = "sink.metric_flush_total_duration_ns"

// MetricKeyTotalMetricsFlushed should be emitted as a counter by a MetricSink
// if possible. Tagged with `sink:sink.Name()`. The `Flush` function is a great
// place to do this.
const MetricKeyTotalMetricsFlushed = "sink.metrics_flushed_total"

// MetricKeyTotalMetricsSkipped should be emitted as a counter by a MetricSink
// if possible. Tagged with `sink:sink.Name()`. Track the number of metrics
// skipped, not applicable to this MetricSink.
const MetricKeyTotalMetricsSkipped = "sink.metrics_skipped_total"

// MetricKeyTotalMetricsDropped should be emitted as a counter by a MetricSink
// if possible. Tagged with `sink:sink.Name()`. Track the number of metrics
// dropped, not applicable to this MetricSink.
const MetricKeyTotalMetricsDropped = "sink.metrics_dropped_total"

// EventReportedCount number of events processed by a sink. Tagged with
// `sink:sink.Name()`.
const EventReportedCount = "sink.events_reported_total"

// MetricSink is a receiver of `InterMetric`s when Veneur periodically flushes
// it's aggregated metrics.
type MetricSink interface {
	Name() string
	// Start finishes setting up the sink and starts any
	// background processing tasks that the sink might have to run
	// in the background. It's invoked when the server starts.
	Start(traceClient *trace.Client) error
	// Flush receives `InterMetric`s from Veneur and is
	// responsible for "sinking" these metrics to whatever it's
	// backend wants. Note that the sink must **not** mutate the
	// incoming metrics as they are shared with other sinks. Sinks
	// must also check each metric with IsAcceptableMetric to
	// verify they are eligible to consume the metric.
	Flush(context.Context, []samplers.InterMetric) error
	// Handle non-metric, non-span samples.
	FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample)
}

// IsAcceptableMetric returns true if a metric is meant to be ingested
// by a given sink.
func IsAcceptableMetric(metric samplers.InterMetric, sink MetricSink) bool {
	if metric.Sinks == nil {
		return true
	}
	return metric.Sinks.RouteTo(sink.Name())
}

// MetricKeySpanFlushDuration should be emitted as a timer by a SpanSink
// if possible. Tagged with `sink:sink.Name()`. The `Flush` function is a great
// place to do this. If your sync does async sends, this might not be necessary.
const MetricKeySpanFlushDuration = "sink.span_flush_total_duration_ns"

// MetricKeyTotalSpansFlushed should be emitted as a counter by a SpanSink
// if possible. Tagged with `sink:sink.Name()`. The `Flush` function is a great
// place to do this.
const MetricKeyTotalSpansFlushed = "sink.spans_flushed_total"

const MetricKeySpanIngestDuration = "sink.span_ingest_total_duration_ns"

// MetricKeyTotalSpansDropped tracks the number of spans that the sink is aware
// it has dropped. It should be emitted as a counter by a SpanSink if possible.
// Tagged with `sink:sink.Name()`. The `Flush` function is a great place to do
// this.
const MetricKeyTotalSpansDropped = "sink.spans_dropped_total"

// MetricKeyTotalSpansSkipped tracks the number of spans that are skipped due to
// sampling, if sampling is enabled.
const MetricKeyTotalSpansSkipped = "sink.spans_skipped_total"

// SpanSink is a receiver of spans that handles sending those spans to some
// downstream sink. Calls to `Ingest(span)` are meant to give the sink control
// of the span, with periodic calls to flush as a signal for sinks that don't
// handle their own flushing in a separate goroutine, etc. Note that SpanSinks
// differ from MetricSinks because Veneur does *not* aggregate Spans.
type SpanSink interface {
	// Start finishes setting up the sink and starts any
	// background processing tasks that the sink might have to run
	// in the background. It's invoked when the server starts.
	Start(*trace.Client) error

	// Name returns the span sink's name for debugging purposes
	Name() string

	// Flush receives `SSFSpan`s from Veneur **as they arrive**. If the sink wants
	// to buffer spans it may do so and defer sending until `Flush` is called.
	Ingest(*ssf.SSFSpan) error

	// Invoked at the same interval as metric flushes, this can be used as a
	// signal for the sink to write out if it was buffering or something.
	Flush()
}

const FlushCompleteMessage = "Flush complete"
