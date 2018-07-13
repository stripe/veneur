package dpsink

import (
	"errors"
	"testing"
	"time"

	"sync/atomic"

	"context"
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/datapoint/dptest"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	"github.com/signalfx/golib/trace"
	"github.com/stretchr/testify/assert"
)

const numTests = 19

func TestCounterSink(t *testing.T) {
	dps := []*datapoint.Datapoint{
		{},
		{},
	}
	ctx := context.Background()
	bs := dptest.NewBasicSink()
	count := &Counter{
		Logger: log.Discard,
	}
	histo := NewHistoCounter(count)
	middleSink := NextWrap(histo)(bs)
	go func() {
		// Allow time for us to get in the middle of a call
		time.Sleep(time.Millisecond)
		assert.Equal(t, int64(1), atomic.LoadInt64(&count.CallsInFlight), "After a sleep, should be in flight")
		datas := <-bs.PointsChan
		assert.Equal(t, 2, len(datas), "Original datas should be sent")
	}()
	log.IfErr(log.Panic, middleSink.AddDatapoints(ctx, dps))
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.CallsInFlight), "Call is finished")
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.TotalProcessErrors), "No errors so far (see above)")
	assert.Equal(t, numTests, len(histo.Datapoints()), "Just checking stats len()")

	bs.RetError(errors.New("nope"))
	if err := middleSink.AddDatapoints(ctx, dps); err == nil {
		t.Fatal("Expected an error!")
	}
	assert.Equal(t, int64(1), atomic.LoadInt64(&count.TotalProcessErrors), "Error should be sent through")
}

func TestCounterSinkEvent(t *testing.T) {
	es := []*event.Event{
		{},
		{},
	}
	ctx := context.Background()
	bs := dptest.NewBasicSink()
	count := &Counter{}
	histo := NewHistoCounter(count)
	middleSink := NextWrap(histo)(bs)
	go func() {
		// Allow time for us to get in the middle of a call
		time.Sleep(time.Millisecond)
		assert.Equal(t, int64(1), atomic.LoadInt64(&count.CallsInFlight), "After a sleep, should be in flight")
		datas := <-bs.EventsChan
		assert.Equal(t, 2, len(datas), "Original datas should be sent")
	}()
	log.IfErr(log.Panic, middleSink.AddEvents(ctx, es))
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.CallsInFlight), "Call is finished")
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.TotalProcessErrors), "No errors so far (see above)")
	assert.Equal(t, numTests, len(histo.Datapoints()), "Just checking stats len()")

	bs.RetError(errors.New("nope"))
	if err := middleSink.AddEvents(ctx, es); err == nil {
		t.Fatal("Expected an error!")
	}
	assert.Equal(t, int64(1), atomic.LoadInt64(&count.TotalProcessErrors), "Error should be sent through")
}

func TestCounterSinkTrace(t *testing.T) {
	es := []*trace.Span{
		{},
		{},
	}
	ctx := context.Background()
	bs := dptest.NewBasicSink()
	count := &Counter{}
	histo := NewHistoCounter(count)
	middleSink := trace.NextWrap(histo)(bs)
	go func() {
		// Allow time for us to get in the middle of a call
		time.Sleep(time.Millisecond)
		assert.Equal(t, int64(1), atomic.LoadInt64(&count.CallsInFlight), "After a sleep, should be in flight")
		datas := <-bs.TracesChan
		assert.Equal(t, 2, len(datas), "Original datas should be sent")
	}()
	log.IfErr(log.Panic, middleSink.AddSpans(ctx, es))
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.CallsInFlight), "Call is finished")
	assert.Equal(t, int64(0), atomic.LoadInt64(&count.TotalProcessErrors), "No errors so far (see above)")
	assert.Equal(t, numTests, len(histo.Datapoints()), "Just checking stats len()")

	bs.RetError(errors.New("nope"))
	if err := middleSink.AddSpans(ctx, es); err == nil {
		t.Fatal("Expected an error!")
	}
	assert.Equal(t, int64(1), atomic.LoadInt64(&count.TotalProcessErrors), "Error should be sent through")
}
