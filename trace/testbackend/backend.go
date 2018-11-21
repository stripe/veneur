// Package testbackend contains helpers to make it easier to test the
// tracing behavior of code using veneur's trace API.
package testbackend

import (
	"context"

	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

// SendErrorSource is a function that a test can provide. It returns
// whether sending a span should return an error or not.
type SendErrorSource func(*ssf.SSFSpan) error

// BackendOption is a functional option for Backends provided by this
// package.
type BackendOption func(*Backend)

// SendErrors allows tests to provide a function that will be
// consulted on whether a send operation should return an error.
func SendErrors(src SendErrorSource) BackendOption {
	return func(be *Backend) {
		be.errorSrc = src
	}
}

// Backend is a ClientBackend that sends spans into a provided
// channel. It does not support flushing.
type Backend struct {
	ch       chan<- *ssf.SSFSpan
	errorSrc SendErrorSource
}

// Close is a no-op.
func (be *Backend) Close() error {
	return nil
}

// SendSync sends the span into the Backend's channel.
func (be *Backend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	if be.errorSrc != nil {
		if err := be.errorSrc(span); err != nil {
			return err
		}
	}

	select {
	case be.ch <- span:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// NewBackend returns a new trace.ClientBackend that sends spans down
// a channel.
func NewBackend(ch chan<- *ssf.SSFSpan, opts ...BackendOption) trace.ClientBackend {
	be := &Backend{ch: ch}
	for _, opt := range opts {
		opt(be)
	}
	return be
}

// FlushErrorSource is a function that a test can provide. It returns
// whether flushing a batch of spans should return an error or not.
type FlushErrorSource func([]*ssf.SSFSpan) error

// FlushingBackendOption is a functional option for Backends provided by this
// package.
type FlushingBackendOption func(*FlushingBackend)

// FlushErrors allows tests to provide a function that will be
// consulted on whether a send operation should return an error.
func FlushErrors(src FlushErrorSource) FlushingBackendOption {
	return func(be *FlushingBackend) {
		be.errorSrc = src
	}
}

// FlushingBackend is a ClientBackend that behaves much like Backend
// does, but also supports flushing. On flush, it sends the number of
// spans contained in each batch.
type FlushingBackend struct {
	errorSrc FlushErrorSource
	batch    []*ssf.SSFSpan
	flushCh  chan<- []*ssf.SSFSpan
}

// FlushSync sends the batch of submitted spans back.
func (be *FlushingBackend) FlushSync(ctx context.Context) error {
	if be.errorSrc != nil {
		if err := be.errorSrc(be.batch); err != nil {
			return err
		}
	}

	select {
	case be.flushCh <- be.batch:
		be.batch = []*ssf.SSFSpan{}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close is a no-op.
func (be *FlushingBackend) Close() error {
	return nil
}

// SendSync sends the span into the FlushingBackend's channel and counts it.
func (be *FlushingBackend) SendSync(ctx context.Context, span *ssf.SSFSpan) error {
	be.batch = append(be.batch, span)
	return nil
}

// NewFlushingBackend constructs a new FlushableClientBackend. It will
// collect the metrics submitted to it in an array (the order of Spans
// in the array represents the order in which the backend's SendSync
// was called).
func NewFlushingBackend(ch chan<- []*ssf.SSFSpan, opts ...FlushingBackendOption) trace.FlushableClientBackend {
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
