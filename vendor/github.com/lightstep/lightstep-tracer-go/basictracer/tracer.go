package basictracer

import (
	"time"

	opentracing "github.com/opentracing/opentracing-go"
)

// Tracer extends the opentracing.Tracer interface with methods to
// probe implementation state, for use by basictracer consumers.
type Tracer interface {
	opentracing.Tracer

	// Options gets the Options used in New() or NewWithOptions().
	Options() Options
}

// Options allows creating a customized Tracer via NewWithOptions. The object
// must not be updated when there is an active tracer using it.
type Options struct {
	// Recorder receives Spans which have been finished.
	Recorder SpanRecorder
	// DropAllLogs turns log events on all Spans into no-ops.
	// If NewSpanEventListener is set, the callbacks will still fire.
	DropAllLogs bool
	// MaxLogsPerSpan limits the number of Logs in a span (if set to a nonzero
	// value). If a span has more logs than this value, logs are dropped as
	// necessary (and replaced with a log describing how many were dropped).
	//
	// About half of the MaxLogPerSpan logs kept are the oldest logs, and about
	// half are the newest logs.
	//
	// If NewSpanEventListener is set, the callbacks will still fire for all log
	// events. This value is ignored if DropAllLogs is true.
	MaxLogsPerSpan int
}

type StartSpanOptions struct {
	Options opentracing.StartSpanOptions

	// Options to explicitly set span_id, trace_id,
	// parent_span_id, expected to be used when exporting spans
	// from another system into LightStep via opentracing APIs.
	SetSpanID       uint64
	SetParentSpanID uint64
	SetTraceID      uint64
}

type LightStepStartSpanOption interface {
	ApplyLS(*StartSpanOptions)
}

// DefaultOptions returns an Options object with a 1 in 64 sampling rate and
// all options disabled. A Recorder needs to be set manually before using the
// returned object with a Tracer.
func DefaultOptions() Options {
	return Options{
		MaxLogsPerSpan: 100,
	}
}

// NewWithOptions creates a customized Tracer.
func NewWithOptions(opts Options) opentracing.Tracer {
	return &tracerImpl{options: opts}
}

// New creates and returns a standard Tracer which defers completed Spans to
// `recorder`.
// Spans created by this Tracer support the ext.SamplingPriority tag: Setting
// ext.SamplingPriority causes the Span to be Sampled from that point on.
func New(recorder SpanRecorder) opentracing.Tracer {
	opts := DefaultOptions()
	opts.Recorder = recorder
	return NewWithOptions(opts)
}

// Implements the `Tracer` interface.
type tracerImpl struct {
	options          Options
	textPropagator   textMapPropagator
	binaryPropagator lightstepBinaryPropagator
}

func (t *tracerImpl) StartSpan(
	operationName string,
	opts ...opentracing.StartSpanOption,
) opentracing.Span {
	sso := StartSpanOptions{}
	for _, o := range opts {
		switch o := o.(type) {
		case LightStepStartSpanOption:
			o.ApplyLS(&sso)
		default:
			o.Apply(&sso.Options)
		}
	}
	return t.startSpanWithOptions(operationName, &sso)
}

func (t *tracerImpl) startSpanWithOptions(
	operationName string,
	opts *StartSpanOptions,
) opentracing.Span {
	// Start time.
	startTime := opts.Options.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	// Tags.
	tags := opts.Options.Tags

	// Build the new span. This is the only allocation: We'll return this as
	// an opentracing.Span.
	sp := &spanImpl{}

	// It's meaningless to provide wither SpanID or ParentSpanID
	// without also providing TraceID, so just test for TraceID.
	if opts.SetTraceID != 0 {
		sp.raw.Context.TraceID = opts.SetTraceID
		sp.raw.Context.SpanID = opts.SetSpanID
		sp.raw.ParentSpanID = opts.SetParentSpanID
	}

	// Look for a parent in the list of References.
	//
	// TODO: would be nice if basictracer did something with all
	// References, not just the first one.
ReferencesLoop:
	for _, ref := range opts.Options.References {
		switch ref.Type {
		case opentracing.ChildOfRef,
			opentracing.FollowsFromRef:

			refCtx := ref.ReferencedContext.(SpanContext)
			sp.raw.Context.TraceID = refCtx.TraceID
			sp.raw.ParentSpanID = refCtx.SpanID

			if l := len(refCtx.Baggage); l > 0 {
				sp.raw.Context.Baggage = make(map[string]string, l)
				for k, v := range refCtx.Baggage {
					sp.raw.Context.Baggage[k] = v
				}
			}
			break ReferencesLoop
		}
	}
	if sp.raw.Context.TraceID == 0 {
		// TraceID not set by parent reference or explicitly
		sp.raw.Context.TraceID, sp.raw.Context.SpanID = randomID2()
	} else if sp.raw.Context.SpanID == 0 {
		// TraceID set but SpanID not set
		sp.raw.Context.SpanID = randomID()
	}

	return t.startSpanInternal(
		sp,
		operationName,
		startTime,
		tags,
	)
}

func (t *tracerImpl) startSpanInternal(
	sp *spanImpl,
	operationName string,
	startTime time.Time,
	tags opentracing.Tags,
) opentracing.Span {
	sp.tracer = t
	sp.raw.Operation = operationName
	sp.raw.Start = startTime
	sp.raw.Duration = -1
	sp.raw.Tags = tags
	return sp
}

func (t *tracerImpl) Inject(sc opentracing.SpanContext, format interface{}, carrier interface{}) error {
	switch format {
	case opentracing.TextMap, opentracing.HTTPHeaders:
		return t.textPropagator.Inject(sc, carrier)
	case BinaryCarrier:
		return t.binaryPropagator.Inject(sc, carrier)
	}
	return opentracing.ErrUnsupportedFormat
}

func (t *tracerImpl) Extract(format interface{}, carrier interface{}) (opentracing.SpanContext, error) {
	switch format {
	case opentracing.TextMap, opentracing.HTTPHeaders:
		return t.textPropagator.Extract(carrier)
	case BinaryCarrier:
		return t.binaryPropagator.Extract(carrier)
	}
	return nil, opentracing.ErrUnsupportedFormat
}

func (t *tracerImpl) Options() Options {
	return t.options
}
