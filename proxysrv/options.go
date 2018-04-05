package proxysrv

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/trace"
)

// WithForwardTimeout sets the time after which an individual RPC to a
// downstream Veneur times out
func WithForwardTimeout(d time.Duration) Option {
	return func(opts *options) {
		opts.forwardTimeout = d
	}
}

// WithLog sets the logger entry used in the object.
func WithLog(e *logrus.Entry) Option {
	return func(opts *options) {
		opts.log = e
	}
}

// WithTraceClient sets the trace client used by the server.
func WithTraceClient(c *trace.Client) Option {
	return func(opts *options) {
		opts.traceClient = c
	}
}
