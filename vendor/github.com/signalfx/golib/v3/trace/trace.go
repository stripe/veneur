package trace

import (
	"context"

	"github.com/signalfx/golib/v3/log"
)

// Trace is a list of spans
type Trace []*Span

// Span defines a span
type Span struct {
	TraceID        string                      `json:"traceId"` // required
	Name           *string                     `json:"name,omitempty"`
	ParentID       *string                     `json:"parentId,omitempty"`
	ID             string                      `json:"id"` // required
	Kind           *string                     `json:"kind,omitempty"`
	Timestamp      *int64                      `json:"timestamp,omitempty"`
	Duration       *int64                      `json:"duration,omitempty"`
	Debug          *bool                       `json:"debug,omitempty"`
	Shared         *bool                       `json:"shared,omitempty"`
	LocalEndpoint  *Endpoint                   `json:"localEndpoint,omitempty"`
	RemoteEndpoint *Endpoint                   `json:"remoteEndpoint,omitempty"`
	Annotations    []*Annotation               `json:"annotations,omitempty"`
	Tags           map[string]string           `json:"tags,omitempty"`
	Meta           map[interface{}]interface{} `json:"-"` // non serializeable field to hold any meta data we want to keep around
}

// Endpoint is the network context of a node in the service graph
type Endpoint struct {
	ServiceName *string `json:"serviceName,omitempty"`
	Ipv4        *string `json:"ipv4,omitempty"`
	Ipv6        *string `json:"ipv6,omitempty"`
	Port        *int32  `json:"port,omitempty"`
}

// Annotation associates an event that explains latency with a timestamp.
// Unlike log statements, annotations are often codes. Ex. “ws” for WireSend
type Annotation struct {
	Timestamp *int64  `json:"timestamp,omitempty"`
	Value     *string `json:"value,omitempty"`
}

// DefaultLogger is used by package structs that don't have a default logger set.
var DefaultLogger = log.DefaultLogger.CreateChild()

// A Sink is an object that can accept traces and do something with them, like forward them to some endpoint
type Sink interface {
	AddSpans(ctx context.Context, traces []*Span) error
}

// A MiddlewareConstructor is used by FromChain to chain together a bunch of sinks that forward to each other
type MiddlewareConstructor func(sendTo Sink) Sink

// FromChain creates an endpoint Sink that sends calls between multiple middlewares for things like counting traces in between.
func FromChain(endSink Sink, sinks ...MiddlewareConstructor) Sink {
	for i := len(sinks) - 1; i >= 0; i-- {
		endSink = sinks[i](endSink)
	}
	return endSink
}

// NextSink is a special case of a sink that forwards to another sink
type NextSink interface {
	AddSpans(ctx context.Context, traces []*Span, next Sink) error
}

type nextWrapped struct {
	forwardTo Sink
	wrapping  NextSink
}

func (n *nextWrapped) AddSpans(ctx context.Context, traces []*Span) error {
	return n.wrapping.AddSpans(ctx, traces, n.forwardTo)
}

// NextWrap wraps a NextSink to make it usable by MiddlewareConstructor
func NextWrap(wrapping NextSink) MiddlewareConstructor {
	return func(sendTo Sink) Sink {
		return &nextWrapped{
			forwardTo: sendTo,
			wrapping:  wrapping,
		}
	}
}
