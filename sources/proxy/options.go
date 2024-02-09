package proxy

import "github.com/stripe/veneur/v14/trace"

// WithTraceClient sets the trace client for the server.  Otherwise it uses
// trace.DefaultClient.
func WithTraceClient(c *trace.Client) Option {
	return func(opts *options) {
		opts.traceClient = c
	}
}
