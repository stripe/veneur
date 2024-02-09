package testbackend

import (
	"context"
	"sync"

	"github.com/stripe/veneur/v14/ssf"
)

// FlushErrorSource is a function that a test can provide. It returns
// whether flushing a batch of spans should return an error or not.
type FlushErrorSource func([]*ssf.SSFSpan) error

// FlushingBackendOption is a functional option for Backends provided by this
// package.
type FlushingBackendOption func(*FlushingBackend)

// FlushErrors allows tests to provide functions that will be
// consulted on whether a send or flush operation should return an
// error.
func FlushErrors(sendSrc SendErrorSource, src FlushErrorSource) FlushingBackendOption {
	return func(be *FlushingBackend) {
		be.errorSrc = src
		be.sendErrorSrc = sendSrc
	}
}

// FlushingBackend is a ClientBackend that behaves much like Backend
// does, but also supports flushing. On flush, it sends the number of
// spans contained in each batch.
type FlushingBackend struct {
	mutex sync.Mutex

	sendErrorSrc SendErrorSource
	errorSrc     FlushErrorSource
	batch        []*ssf.SSFSpan
	flushCh      chan<- []*ssf.SSFSpan
}

// FlushSync sends the batch of submitted spans back.
func (be *FlushingBackend) FlushSync(ctx context.Context) error {
	return be.Flush()
}

// Close is a no-op.
func (be *FlushingBackend) Close() error {
	return nil
}

// SendSync sends the span into the FlushingBackend's channel and counts it.
func (be *FlushingBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	if be.sendErrorSrc != nil {
		if err := be.sendErrorSrc(span); err != nil {
			return err
		}
	}

	be.mutex.Lock()
	defer be.mutex.Unlock()

	be.batch = append(be.batch, span)
	return nil
}

// Flush on a FlushingBackend is an alternative to the Client's flush
// functionality for tests. It flushes spans deterministically and so
// ensures that the flush actually happens.
func (be *FlushingBackend) Flush() error {
	be.mutex.Lock()
	defer be.mutex.Unlock()

	if be.errorSrc != nil {
		if err := be.errorSrc(be.batch); err != nil {
			return err
		}
	}
	be.flushCh <- be.batch
	be.batch = make([]*ssf.SSFSpan, 0)
	return nil
}

// NewFlushingBackend constructs a new FlushableClientBackend. It will
// collect the metrics submitted to it in an array (the order of Spans
// in the array represents the order in which the backend's SendSync
// was called).
func NewFlushingBackend(ch chan<- []*ssf.SSFSpan, opts ...FlushingBackendOption) *FlushingBackend {
	be := &FlushingBackend{
		flushCh:  ch,
		errorSrc: nil,
		batch:    []*ssf.SSFSpan{},
	}
	for _, opt := range opts {
		opt(be)
	}
	return be
}
