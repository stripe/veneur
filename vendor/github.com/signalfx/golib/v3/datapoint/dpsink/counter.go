package dpsink

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/signalfx/golib/v3/datapoint"
	"github.com/signalfx/golib/v3/event"
	"github.com/signalfx/golib/v3/log"
	"github.com/signalfx/golib/v3/sfxclient"
	"github.com/signalfx/golib/v3/sfxclient/spanfilter"
	"github.com/signalfx/golib/v3/trace"
)

// DefaultLogger is used by package structs that don't have a default logger set.
var DefaultLogger = log.DefaultLogger.CreateChild()

// Counter records stats on datapoints to go through it as a sink middleware
type Counter struct {
	TotalProcessErrors int64
	TotalDatapoints    int64
	TotalEvents        int64
	TotalSpans         int64
	TotalProcessCalls  int64
	ProcessErrorPoints int64
	ProcessErrorEvents int64
	ProcessErrorSpans  int64
	TotalProcessTimeNs int64
	CallsInFlight      int64
	Logger             log.Logger
	LoggerFunc         func(context.Context) string
	LoggerKey          log.Key
	DroppedReason      string
}

// Datapoints returns counter stats
func (c *Counter) Datapoints() []*datapoint.Datapoint {
	var dims map[string]string
	if c.DroppedReason != "" {
		dims = map[string]string{"reason": c.DroppedReason}
	}
	return []*datapoint.Datapoint{
		sfxclient.Cumulative("total_process_errors", nil, atomic.LoadInt64(&c.TotalProcessErrors)),
		sfxclient.Cumulative("total_datapoints", nil, atomic.LoadInt64(&c.TotalDatapoints)),
		sfxclient.Cumulative("total_events", nil, atomic.LoadInt64(&c.TotalEvents)),
		sfxclient.Cumulative("total_spans", nil, atomic.LoadInt64(&c.TotalSpans)),
		sfxclient.Cumulative("total_process_calls", nil, atomic.LoadInt64(&c.TotalProcessCalls)),
		sfxclient.Cumulative("dropped_points", dims, atomic.LoadInt64(&c.ProcessErrorPoints)),
		sfxclient.Cumulative("dropped_events", dims, atomic.LoadInt64(&c.ProcessErrorEvents)),
		sfxclient.Cumulative("dropped_spans", dims, atomic.LoadInt64(&c.ProcessErrorSpans)),
		sfxclient.Cumulative("process_time_ns", nil, atomic.LoadInt64(&c.TotalProcessTimeNs)),
		sfxclient.Gauge("calls_in_flight", nil, atomic.LoadInt64(&c.CallsInFlight)),
	}
}

func (c *Counter) logErrMsg(ctx context.Context, err error, msg string) {
	var ret []interface{}
	if c.LoggerFunc != nil && c.LoggerKey != "" {
		value := c.LoggerFunc(ctx)
		if value != "" {
			ret = append(ret, c.LoggerKey, value)
		}
	}
	ret = append(ret, log.Err, err, msg)
	c.logger().Log(ret...)
}

// AddDatapoints will send points to the next sink and track points send to the next sink
func (c *Counter) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint, next Sink) error {
	atomic.AddInt64(&c.TotalDatapoints, int64(len(points)))
	atomic.AddInt64(&c.TotalProcessCalls, 1)
	atomic.AddInt64(&c.CallsInFlight, 1)
	start := time.Now()
	err := next.AddDatapoints(ctx, points)
	atomic.AddInt64(&c.TotalProcessTimeNs, time.Since(start).Nanoseconds())
	atomic.AddInt64(&c.CallsInFlight, -1)
	if err != nil {
		atomic.AddInt64(&c.TotalProcessErrors, 1)
		atomic.AddInt64(&c.ProcessErrorPoints, int64(len(points)))
		c.logErrMsg(ctx, err, "Unable to process datapoints")
	}
	return err
}

func (c *Counter) logger() log.Logger {
	if c.Logger == nil {
		return DefaultLogger
	}
	return c.Logger
}

// AddEvents will send events to the next sink and track events sent to the next sink
func (c *Counter) AddEvents(ctx context.Context, events []*event.Event, next Sink) error {
	atomic.AddInt64(&c.TotalEvents, int64(len(events)))
	atomic.AddInt64(&c.TotalProcessCalls, 1)
	atomic.AddInt64(&c.CallsInFlight, 1)
	start := time.Now()
	err := next.AddEvents(ctx, events)
	atomic.AddInt64(&c.TotalProcessTimeNs, time.Since(start).Nanoseconds())
	atomic.AddInt64(&c.CallsInFlight, -1)
	if err != nil {
		atomic.AddInt64(&c.TotalProcessErrors, 1)
		atomic.AddInt64(&c.ProcessErrorEvents, int64(len(events)))
		c.logErrMsg(ctx, err, "Unable to process events")
	}
	return err
}

// AddSpans will send spans to the next sink and track spans sent to the next sink
func (c *Counter) AddSpans(ctx context.Context, spans []*trace.Span, next trace.Sink) error {
	atomic.AddInt64(&c.TotalSpans, int64(len(spans)))
	atomic.AddInt64(&c.TotalProcessCalls, 1)
	atomic.AddInt64(&c.CallsInFlight, 1)
	start := time.Now()
	err := next.AddSpans(ctx, spans)
	atomic.AddInt64(&c.TotalProcessTimeNs, time.Since(start).Nanoseconds())
	atomic.AddInt64(&c.CallsInFlight, -1)
	if err != nil && spanfilter.IsInvalid(err) {
		atomic.AddInt64(&c.TotalProcessErrors, 1)
		if m, ok := err.(*spanfilter.Map); ok {
			atomic.AddInt64(&c.ProcessErrorSpans, int64(len(m.Invalid)))
		} else {
			atomic.AddInt64(&c.ProcessErrorSpans, int64(len(spans)))
		}
		c.logErrMsg(ctx, err, "Unable to process spans")
	}
	return err
}

// HistoCounter wraps a Counter with a histogram around batch sizes
type HistoCounter struct {
	sink            *Counter
	DatapointBucket *sfxclient.RollingBucket
	EventBucket     *sfxclient.RollingBucket
	SpanBucket      *sfxclient.RollingBucket
}

// AddDatapoints sample length of slice and pass on
func (h *HistoCounter) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint, next Sink) error {
	h.DatapointBucket.Add(float64(len(points)))
	return h.sink.AddDatapoints(ctx, points, next)
}

// AddEvents sample length of slice and pass on
func (h *HistoCounter) AddEvents(ctx context.Context, events []*event.Event, next Sink) error {
	h.EventBucket.Add(float64(len(events)))
	return h.sink.AddEvents(ctx, events, next)
}

// AddSpans sample length of slice and pass on
func (h *HistoCounter) AddSpans(ctx context.Context, spans []*trace.Span, next trace.Sink) error {
	h.SpanBucket.Add(float64(len(spans)))
	return h.sink.AddSpans(ctx, spans, next)
}

// Datapoints is rather self explanitory
func (h *HistoCounter) Datapoints() []*datapoint.Datapoint {
	dps := h.sink.Datapoints()
	dps = append(dps, h.DatapointBucket.Datapoints()...)
	dps = append(dps, h.EventBucket.Datapoints()...)
	dps = append(dps, h.SpanBucket.Datapoints()...)
	return dps
}

// NewHistoCounter is a constructor
func NewHistoCounter(sink *Counter) *HistoCounter {
	return &HistoCounter{
		sink:            sink,
		DatapointBucket: sfxclient.NewRollingBucket("datapoint_batch_size", map[string]string{}),
		EventBucket:     sfxclient.NewRollingBucket("event_batch_size", map[string]string{}),
		SpanBucket:      sfxclient.NewRollingBucket("span_batch_size", map[string]string{}),
	}
}
