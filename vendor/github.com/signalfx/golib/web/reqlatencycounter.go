package web

import (
	"sync/atomic"
	"time"

	"golang.org/x/net/context"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/timekeeper"
)

// FastRequestLimitDuration is the interface for getting the cutoff time for bad requests.
type FastRequestLimitDuration interface {
	Get() time.Duration
}

// ReqLatencyCounter hold static data for the request latency tracking.
type ReqLatencyCounter struct {
	fastRequestLimitDuration FastRequestLimitDuration
	fastRequests             int64
	slowRequests             int64
	timeKeeper               timekeeper.TimeKeeper
}

// NewReqLatencyCounter creates a new ReqLatencyCounter
func NewReqLatencyCounter(fastRequestDurationLimit FastRequestLimitDuration) ReqLatencyCounter {
	return ReqLatencyCounter{
		fastRequestLimitDuration: fastRequestDurationLimit,
		timeKeeper:               timekeeper.RealTime{},
	}
}

// ModStats modifies the metric values for the ReqLatencyCounter.
func (a *ReqLatencyCounter) ModStats(ctx context.Context) {
	a.ModStatsTime(RequestTime(ctx))
}

// ModStatsTime modifies the metric values for the ReqLatencyCounter.
func (a *ReqLatencyCounter) ModStatsTime(start time.Time) {
	totalDur := a.timeKeeper.Now().Sub(start)
	if totalDur > a.fastRequestLimitDuration.Get() {
		atomic.AddInt64(&a.slowRequests, 1)
	} else {
		atomic.AddInt64(&a.fastRequests, 1)
	}
}

// appendDimension will blah blah blah
func appendDimensions(dimensions map[string]string, key string, value string) map[string]string {
	r := make(map[string]string, len(dimensions)+1)
	for k, v := range dimensions {
		r[k] = v
	}
	r[key] = value
	return r
}

// Stats gets the metrics for a ReqLatencyCounter.
func (a *ReqLatencyCounter) Stats(dimensions map[string]string) []*datapoint.Datapoint {
	now := a.timeKeeper.Now()
	getDp := func(key string, value int64) *datapoint.Datapoint {
		return datapoint.New(
			"ReqLatencyCounter.requestCounts", appendDimensions(dimensions, "requestClass", key),
			datapoint.NewIntValue(value), datapoint.Counter, now)
	}
	return []*datapoint.Datapoint{
		getDp("fast", a.fastRequests),
		getDp("slow", a.slowRequests),
	}
}
