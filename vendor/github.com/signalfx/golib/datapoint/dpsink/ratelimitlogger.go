package dpsink

import (
	"sync/atomic"
	"time"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	"golang.org/x/net/context"
)

// RateLimitErrorLogging does a log of errors forwarding points in a rate limited manner
type RateLimitErrorLogging struct {
	lastLogTimeNs int64
	LogThrottle   time.Duration
	Logger        log.Logger
}

// AddDatapoints forwards points and will log any errors forwarding, but only one per LogThrottle
// duration
func (e *RateLimitErrorLogging) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint, next Sink) error {
	err := next.AddDatapoints(ctx, points)
	e.throttleLog(err)
	return err
}

func (e *RateLimitErrorLogging) throttleLog(err error) {
	if err != nil {
		now := time.Now()
		lastLogTimeNs := atomic.LoadInt64(&e.lastLogTimeNs)
		sinceLastLogNs := now.UnixNano() - lastLogTimeNs
		if sinceLastLogNs > e.LogThrottle.Nanoseconds() {
			nowUnixNs := now.UnixNano()
			if atomic.CompareAndSwapInt64(&e.lastLogTimeNs, lastLogTimeNs, nowUnixNs) {
				e.Logger.Log(log.Err, err)
			}
		}
	}
}

// AddEvents forwards points and will log any errors forwarding, but only one per LogThrottle
// duration
func (e *RateLimitErrorLogging) AddEvents(ctx context.Context, points []*event.Event, next Sink) error {
	err := next.AddEvents(ctx, points)
	e.throttleLog(err)
	return err
}
