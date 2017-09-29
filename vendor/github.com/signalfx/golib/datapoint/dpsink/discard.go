package dpsink

import (
	"github.com/signalfx/golib/datapoint"
	"github.com/signalfx/golib/event"
	"golang.org/x/net/context"
)

type discardSink struct {
}

func (d discardSink) AddDatapoints(_ context.Context, _ []*datapoint.Datapoint) error {
	return nil
}

func (d discardSink) AddEvents(_ context.Context, _ []*event.Event) error {
	return nil
}

// Discard is a datapoint sink that does nothing with points it gets
var Discard = discardSink{}
