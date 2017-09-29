package dpsink

import (
	"sync/atomic"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/sfxclient"
	"golang.org/x/net/context"
)

// DefaultLogger is used by package structs that don't have a default logger set.
var DefaultLogger = log.DefaultLogger.CreateChild()

// Counter records stats on datapoints to go through it as a sink middleware
type Counter struct {
	TotalProcessErrors int64
	TotalDatapoints    int64
	TotalEvents        int64
	TotalProcessCalls  int64
	ProcessErrorPoints int64
	TotalProcessTimeNs int64
	CallsInFlight      int64
	Logger             log.Logger
}

// Datapoints returns counter stats
func (c *Counter) Datapoints() []*datapoint.Datapoint {
	return []*datapoint.Datapoint{
		sfxclient.Cumulative("total_process_errors", nil, atomic.LoadInt64(&c.TotalProcessErrors)),
		sfxclient.Cumulative("total_datapoints", nil, atomic.LoadInt64(&c.TotalDatapoints)),
		sfxclient.Cumulative("total_events", nil, atomic.LoadInt64(&c.TotalEvents)),
		sfxclient.Cumulative("total_process_calls", nil, atomic.LoadInt64(&c.TotalProcessCalls)),
		sfxclient.Cumulative("dropped_points", nil, atomic.LoadInt64(&c.ProcessErrorPoints)),
		sfxclient.Cumulative("process_time_ns", nil, atomic.LoadInt64(&c.TotalProcessTimeNs)),
		sfxclient.Gauge("calls_in_flight", nil, atomic.LoadInt64(&c.CallsInFlight)),
	}
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
		c.logger().Log(log.Err, err, "Unable to process datapoints")
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
		atomic.AddInt64(&c.ProcessErrorPoints, int64(len(events)))
		c.logger().Log(log.Err, err, "Unable to process datapoints")
	}
	return err
}
