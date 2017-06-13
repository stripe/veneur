package lightstep

import (
	"github.com/lightstep/lightstep-tracer-go/basictracer"
	opentracing "github.com/opentracing/opentracing-go"
)

type (
	// SetSpanID is a opentracing.StartSpanOption that sets an
	// explicit SpanID.  It must be used in conjunction with
	// SetTraceID or the result is undefined.
	SetSpanID uint64

	// SetTraceID is an opentracing.StartSpanOption that sets an
	// explicit TraceID.  It must be used in order to set an
	// explicit SpanID or ParentSpanID.  If a ChildOf or
	// FollowsFrom span relation is also set in the start options,
	// it will override this value.
	SetTraceID uint64

	// SetParentSpanID is an opentracing.StartSpanOption that sets
	// an explicit parent SpanID.  It must be used in conjunction
	// with SetTraceID or the result is undefined.  If the value
	// is zero, it will be disregarded.  If a ChildOf or
	// FollowsFrom span relation is also set in the start options,
	// it will override this value.
	SetParentSpanID uint64
)

// just kidding these aren't real OT start span options
func (sid SetTraceID) Apply(sso *opentracing.StartSpanOptions)      {}
func (sid SetSpanID) Apply(sso *opentracing.StartSpanOptions)       {}
func (sid SetParentSpanID) Apply(sso *opentracing.StartSpanOptions) {}

func (sid SetTraceID) ApplyLS(sso *basictracer.StartSpanOptions) {
	sso.SetTraceID = uint64(sid)
}
func (sid SetSpanID) ApplyLS(sso *basictracer.StartSpanOptions) {
	sso.SetSpanID = uint64(sid)
}
func (sid SetParentSpanID) ApplyLS(sso *basictracer.StartSpanOptions) {
	sso.SetParentSpanID = uint64(sid)
}
