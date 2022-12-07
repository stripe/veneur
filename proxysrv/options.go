package proxysrv

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util/matcher"
)

// WithForwardTimeout sets the time after which an individual RPC to a
// downstream Veneur times out
func WithForwardTimeout(d time.Duration) Option {
	return func(opts *options) {
		opts.forwardTimeout = d
	}
}

// WithIgnoredTags sets matching rules to ignore tags for sharding metrics
// across global veneur instances.
func WithIgnoredTags(ignoredTags []matcher.TagMatcher) Option {
	return func(opts *options) {
		opts.ignoredTags = ignoredTags
	}
}

// WithLog sets the logger entry used in the object.
func WithLog(e *logrus.Entry) Option {
	return func(opts *options) {
		opts.log = e
	}
}

// WithStatsInterval sets the time interval at which diagnostic metrics about
// the server will be emitted.
func WithStatsInterval(d time.Duration) Option {
	return func(opts *options) {
		opts.statsInterval = d
	}
}

// WithTraceClient sets the trace client used by the server.
func WithTraceClient(c *trace.Client) Option {
	return func(opts *options) {
		opts.traceClient = c
	}
}

func WithEnableStreaming(streaming bool) Option {
	return func(opts *options) {
		opts.streaming = streaming
	}
}
