package dpsink

import (
	"encoding/json"
	"expvar"
	"fmt"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	"golang.org/x/net/context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// FlagCheck checks a context to see if a debug flag is set
type FlagCheck interface {
	HasFlag(ctx context.Context) bool
}

// ItemFlagger will flag events and datapoints according to which dimensions are set or if the
// connection context has a flag set.
type ItemFlagger struct {
	CtxFlagCheck        FlagCheck
	Logger              log.Logger
	EventMetaName       string
	MetricDimensionName string
	dimensions          map[string]string
	mu                  sync.RWMutex
	stats               struct {
		totalDpCtxSignals int64
		totalEvCtxSignals int64
		totalDpDimSignals int64
		totalEvDimSignals int64
	}
}

// SetDatapointFlag flags the datapoint as connected to this flagger
func (f *ItemFlagger) SetDatapointFlag(dp *datapoint.Datapoint) {
	dp.Meta[f] = struct{}{}
}

// HasDatapointFlag return true if the datapoint is connected to this item
func (f *ItemFlagger) HasDatapointFlag(dp *datapoint.Datapoint) bool {
	_, exists := dp.Meta[f]
	return exists
}

// SetEventFlag flags the event as connected to this flagger
func (f *ItemFlagger) SetEventFlag(ev *event.Event) {
	ev.Properties[f.EventMetaName] = f
}

// HasEventFlag return true if the event is connected to this item
func (f *ItemFlagger) HasEventFlag(ev *event.Event) bool {
	if f == nil {
		return false
	}
	setTo, exists := ev.Properties[f.EventMetaName]
	return exists && setTo == f
}

// SetDimensions controls which dimensions are flagged
func (f *ItemFlagger) SetDimensions(dims map[string]string) {
	f.mu.Lock()
	f.dimensions = dims
	f.mu.Unlock()
}

// GetDimensions returns which dimensions are flagged
func (f *ItemFlagger) GetDimensions() map[string]string {
	f.mu.RLock()
	ret := f.dimensions
	f.mu.RUnlock()
	return ret
}

// SetDatapointFlags sets the log flag for every datapoint if the signal is inside the context
func (f *ItemFlagger) SetDatapointFlags(ctx context.Context, points []*datapoint.Datapoint) {
	if f.CtxFlagCheck.HasFlag(ctx) {
		atomic.AddInt64(&f.stats.totalDpCtxSignals, 1)
		for _, dp := range points {
			f.SetDatapointFlag(dp)
		}
	}
	dims := f.GetDimensions()
	if len(dims) == 0 {
		return
	}
	for _, dp := range points {
		if dpMatches(dp, f.MetricDimensionName, dims) {
			atomic.AddInt64(&f.stats.totalDpDimSignals, 1)
			f.SetDatapointFlag(dp)
		}
	}
}

// AddDatapoints adds a signal to each datapoint if the signal is inside the context
func (f *ItemFlagger) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint, next Sink) error {
	f.SetDatapointFlags(ctx, points)
	return next.AddDatapoints(ctx, points)
}

// AddEvents adds a signal to each event if the signal is inside the context
func (f *ItemFlagger) AddEvents(ctx context.Context, events []*event.Event, next Sink) error {
	if f.CtxFlagCheck.HasFlag(ctx) {
		atomic.AddInt64(&f.stats.totalEvCtxSignals, 1)
		for _, dp := range events {
			f.SetEventFlag(dp)
		}
		return next.AddEvents(ctx, events)
	}
	dims := f.GetDimensions()
	if len(dims) == 0 {
		return next.AddEvents(ctx, events)
	}
	for _, ev := range events {
		if evMatches(ev, dims) {
			atomic.AddInt64(&f.stats.totalEvDimSignals, 1)
			f.SetEventFlag(ev)
		}
	}
	return next.AddEvents(ctx, events)
}

// Datapoints returns debug stat information about the flagger
func (f *ItemFlagger) Datapoints() []*datapoint.Datapoint {
	return []*datapoint.Datapoint{
		datapoint.New("totalDpCtxSignals", nil, datapoint.NewIntValue(f.stats.totalDpCtxSignals), datapoint.Counter, time.Time{}),
		datapoint.New("totalEvCtxSignals", nil, datapoint.NewIntValue(f.stats.totalEvCtxSignals), datapoint.Counter, time.Time{}),
		datapoint.New("totalDpDimSignals", nil, datapoint.NewIntValue(f.stats.totalDpDimSignals), datapoint.Counter, time.Time{}),
		datapoint.New("totalEvDimSignals", nil, datapoint.NewIntValue(f.stats.totalEvDimSignals), datapoint.Counter, time.Time{}),
	}
}

// ServeHTTP supports GET to see the current dimensions and POST to change the current dimensions.
// POST expects (and GET returns) a JSON encoded map[string]string
func (f *ItemFlagger) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.Method == "POST" {
		var newDimensions map[string]string
		err := json.NewDecoder(req.Body).Decode(&newDimensions)
		if err != nil {
			rw.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(rw, "Cannot decode request JSON: %s", err.Error())
			return
		}
		log.IfErr(f.Logger, req.Body.Close())
		f.SetDimensions(newDimensions)
		fmt.Fprintf(rw, "Dimensions updated!")
		return
	}
	if req.Method == "GET" {
		log.IfErr(f.Logger, json.NewEncoder(rw).Encode(f.GetDimensions()))
		return
	}
	http.NotFound(rw, req)
}

// Var returns the dimensions that are being filtered
func (f *ItemFlagger) Var() expvar.Var {
	return expvar.Func(func() interface{} {
		return f.GetDimensions()
	})
}

func dpMatches(dp *datapoint.Datapoint, MetricDimensionName string, dimsToCheck map[string]string) bool {
	for k, v := range dimsToCheck {
		if k == MetricDimensionName {
			if v != dp.Metric {
				return false
			}
			continue
		}
		dpVal, exists := dp.Dimensions[k]
		if !exists || dpVal != v {
			return false
		}
	}
	return true
}

func evMatches(ev *event.Event, dimsToCheck map[string]string) bool {
	for k, v := range dimsToCheck {
		dpVal, exists := ev.Dimensions[k]
		if !exists || dpVal != v {
			return false
		}
	}
	return true
}
