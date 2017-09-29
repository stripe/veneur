package dptest

import (
	"sync"

	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"golang.org/x/net/context"
)

// BasicSink is a pure testing sink that blocks forwarded points onto a channel
type BasicSink struct {
	RetErr     error
	PointsChan chan []*datapoint.Datapoint
	EventsChan chan []*event.Event

	mu sync.Mutex
}

// Next returns a single datapoint from the top of PointsChan and panics if the top doesn't contain
// only one point
func (f *BasicSink) Next() *datapoint.Datapoint {
	r := <-f.PointsChan
	if len(r) != 1 {
		panic("Expect a single point")
	}
	return r[0]
}

// NextEvent returns a single event from the top of EventsChan and panics if the top doesn't contain
// only one event
func (f *BasicSink) NextEvent() *event.Event {
	r := <-f.EventsChan
	if len(r) != 1 {
		panic("Expect a single event")
	}
	return r[0]
}

// AddDatapoints buffers the point on an internal chan or returns errors if RetErr is set
func (f *BasicSink) AddDatapoints(ctx context.Context, points []*datapoint.Datapoint) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RetErr != nil {
		return f.RetErr
	}
	select {
	case f.PointsChan <- points:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// AddEvents buffers the event on an internal chan or returns errors if RetErr is set
func (f *BasicSink) AddEvents(ctx context.Context, points []*event.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.RetErr != nil {
		return f.RetErr
	}
	select {
	case f.EventsChan <- points:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RetError sets an error that is returned on AddDatapoints calls
func (f *BasicSink) RetError(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.RetErr = err
}

// Resize the internal chan of points sent here
func (f *BasicSink) Resize(size int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.PointsChan) != 0 {
		panic("can only resize when empty")
	}
	f.PointsChan = make(chan []*datapoint.Datapoint, size)
	f.EventsChan = make(chan []*event.Event, size)
}

// NewBasicSink creates a BasicSink with an unbuffered chan.  Note, calls to AddDatapoints will then
// block until you drain the PointsChan.
func NewBasicSink() *BasicSink {
	return &BasicSink{
		PointsChan: make(chan []*datapoint.Datapoint),
		EventsChan: make(chan []*event.Event),
	}
}
