package dptest

import (
	"errors"
	"testing"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"github.com/signalfx/golib/log"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

func TestBasicSink(t *testing.T) {
	b := NewBasicSink()
	b.PointsChan = make(chan []*datapoint.Datapoint, 2)
	assert.NoError(t, b.AddDatapoints(context.Background(), []*datapoint.Datapoint{}))
	assert.Equal(t, 1, len(b.PointsChan))
}

func TestBasicSinkEvent(t *testing.T) {
	b := NewBasicSink()
	b.EventsChan = make(chan []*event.Event, 2)
	assert.NoError(t, b.AddEvents(context.Background(), []*event.Event{}))
	assert.Equal(t, 1, len(b.EventsChan))
}

func TestBasicSinkErr(t *testing.T) {
	b := NewBasicSink()
	b.RetError(errors.New("nope"))
	assert.Error(t, b.AddDatapoints(context.Background(), []*datapoint.Datapoint{}))
}

func TestBasicSinkErrEvent(t *testing.T) {
	b := NewBasicSink()
	b.RetError(errors.New("nope"))
	assert.Error(t, b.AddEvents(context.Background(), []*event.Event{}))
}

func TestContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	progressChan := make(chan struct{})
	b := NewBasicSink()
	go func() {
		cancel()
		close(progressChan)
	}()
	assert.Equal(t, context.Canceled, b.AddDatapoints(ctx, []*datapoint.Datapoint{}))
	<-progressChan
}

func TestContextErrorEvent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	progressChan := make(chan struct{})
	b := NewBasicSink()
	go func() {
		cancel()
		close(progressChan)
	}()
	assert.Equal(t, context.Canceled, b.AddEvents(ctx, []*event.Event{}))
	<-progressChan
}

func TestNext(t *testing.T) {
	ctx := context.Background()
	b := NewBasicSink()
	dp := DP()
	go func() {
		log.IfErr(log.Panic, b.AddDatapoints(ctx, []*datapoint.Datapoint{dp}))
	}()
	dpSeen := b.Next()
	assert.Equal(t, dpSeen, dp)

	go func() {
		log.IfErr(log.Panic, b.AddDatapoints(ctx, []*datapoint.Datapoint{dp, dp}))
	}()
	assert.Panics(t, func() {
		b.Next()
	})
}

func TestNextEvent(t *testing.T) {
	ctx := context.Background()
	b := NewBasicSink()
	e := E()
	go func() {
		log.IfErr(log.Panic, b.AddEvents(ctx, []*event.Event{e}))
	}()
	eSeen := b.NextEvent()
	assert.Equal(t, eSeen, e)

	go func() {
		log.IfErr(log.Panic, b.AddEvents(ctx, []*event.Event{e, e}))
	}()
	assert.Panics(t, func() {
		b.NextEvent()
	})
}

func TestResize(t *testing.T) {
	b := NewBasicSink()
	assert.Equal(t, 0, cap(b.PointsChan))
	b.Resize(1)
	assert.Equal(t, 1, cap(b.PointsChan))
	b.PointsChan <- []*datapoint.Datapoint{}
	assert.Panics(t, func() {
		b.Resize(0)
	})
}
