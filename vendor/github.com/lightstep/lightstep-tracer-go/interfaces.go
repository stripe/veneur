package lightstep

import (
	"io"

	ot "github.com/opentracing/opentracing-go"
	"golang.org/x/net/context"
)

// Connection describes a closable connection. Exposed for testing.
type Connection interface {
	io.Closer
}

// ConnectorFactory is for testing purposes.
type ConnectorFactory func() (interface{}, Connection, error)

// collectorResponse encapsulates internal thrift/grpc responses.
type collectorResponse interface {
	GetErrors() []string
	Disable() bool
}

// collectorClient encapsulates internal thrift/grpc transports.
type collectorClient interface {
	Report(context.Context, *reportBuffer) (collectorResponse, error)
	ConnectClient() (Connection, error)
	ShouldReconnect() bool
}

// A SpanRecorder handles all of the `RawSpan` data generated via an
// associated `Tracer` instance.
type SpanRecorder interface {
	RecordSpan(RawSpan)
}

// Tracer extends the opentracing.Tracer interface with methods to
// probe implementation state, for use by basictracer consumers.
type Tracer interface {
	ot.Tracer

	// Close flushes and then terminates the LightStep collector
	Close() error
	// Flush sends all spans currently in the buffer to the LighStep collector
	Flush()
	// Options gets the Options used in New() or NewWithOptions().
	Options() Options
	// Disable prevents the tracer from recording spans or flushing
	Disable()
}

// lightStepStartSpanOption is used to identify lightstep-specific Span options.
type lightStepStartSpanOption interface {
	applyLS(*startSpanOptions)
}
