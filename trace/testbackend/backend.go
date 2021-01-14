// Package testbackend contains helpers to make it easier to test the
// tracing behavior of code using veneur's trace API.
package testbackend

import (
	"context"

	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
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
