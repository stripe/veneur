package trace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	opentracing "github.com/opentracing/opentracing-go"
	opentracinglog "github.com/opentracing/opentracing-go/log"
	"github.com/stripe/veneur/ssf"
)

// Lists the names of headers that a specification uses for representing trace information.
type HeaderGroup struct {
	TraceID string
	SpanID  string
}

// Veneur supports multiple tracing header formats. We try each set of headers until we find one that exists.
// Note: textMapReaderGet is case insensitive, so the capitalization of these headers is not important.
var HeaderFormats = []HeaderGroup{
	// Envoy format. We check Envoy first because Envoy sits between services and will most likely be the nearest parent.
	HeaderGroup{
		TraceID: "x-request-id",
		SpanID:  "x-client-trace-id",
	},
	// OpenTracing format.
	HeaderGroup{
		TraceID: "Trace-Id",
		SpanID:  "Span-Id",
	},
	// Veneur format.
	HeaderGroup{
		TraceID: "Traceid",
		SpanID:  "Spanid",
	},
}

// GlobalTracer is the… global tracer!
var GlobalTracer = Tracer{}

func init() {
	opentracing.SetGlobalTracer(GlobalTracer)

}

var _ opentracing.Tracer = &Tracer{}
var _ opentracing.Span = &Span{}
var _ opentracing.SpanContext = &spanContext{}
var _ opentracing.StartSpanOption = &spanOption{}
var _ opentracing.TextMapReader = textMapReaderWriter(map[string]string{})
var _ opentracing.TextMapWriter = textMapReaderWriter(map[string]string{})

var ErrUnsupportedSpanContext = errors.New("Unsupported SpanContext")

type ErrContractViolation struct {
	details interface{}
}

func (e ErrContractViolation) Error() string {
	return fmt.Sprintf("Contract violation: %#v", e.details)
}

type textMapReaderWriter map[string]string

func (t textMapReaderWriter) ForeachKey(handler func(k, v string) error) error {
	for k, v := range t {
		err := handler(k, v)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t textMapReaderWriter) Set(k, v string) {
	t[k] = v
}

// Clone creates a new textMapReaderWriter with the same
// key-value pairs
func (t textMapReaderWriter) Clone() textMapReaderWriter {
	clone := textMapReaderWriter(map[string]string{})
	t.CloneTo(clone)
	return clone
}

// CloneTo clones the textMapReaderWriter into the provided TextMapWriter
func (t textMapReaderWriter) CloneTo(w opentracing.TextMapWriter) {
	t.ForeachKey(func(k, v string) error {
		w.Set(k, v)
		return nil
	})
}

type spanContext struct {
	baggageItems map[string]string
}

func (c *spanContext) Init() {
	c.baggageItems = map[string]string{}
}

// ForeachBaggageItem calls the handler function on each key/val pair in
// the spanContext's baggage items. If the handler function returns false, it
// terminates iteration immediately.
func (c *spanContext) ForeachBaggageItem(handler func(k, v string) bool) {
	errHandler := func(k, v string) error {
		b := handler(k, v)
		if !b {
			return errors.New("dummy error")
		}
		return nil
	}

	textMapReaderWriter(c.baggageItems).ForeachKey(errHandler)
}

// TraceID extracts the Trace ID from the BaggageItems.
// It assumes the TraceID is present and valid.
func (c *spanContext) TraceID() int64 {
	return c.parseBaggageInt64("traceid")
}

// ParentID extracts the Parent ID from the BaggageItems.
// It assumes the ParentID is present and valid.
func (c *spanContext) ParentID() int64 {
	return c.parseBaggageInt64("parentid")
}

// SpanID extracts the Span ID from the BaggageItems.
// It assumes the SpanId is present and valid.
func (c *spanContext) SpanID() int64 {
	return c.parseBaggageInt64("spanid")
}

// parseBaggageInt64 searches for the target key in the BaggageItems
// and parses it as an int64. It treats keys as case-insensitive.
func (c *spanContext) parseBaggageInt64(key string) int64 {
	var val int64
	c.ForeachBaggageItem(func(k, v string) bool {
		if strings.ToLower(k) == strings.ToLower(key) {
			i, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				// TODO handle err
				return true
			}
			val = i
			return false
		}
		return true
	})
	return val
}

// Resource returns the resource assocaited with the spanContext
func (c *spanContext) Resource() string {
	var resource string
	c.ForeachBaggageItem(func(k, v string) bool {
		if strings.ToLower(k) == ResourceKey {
			resource = v
			return false
		}
		return true
	})
	return resource
}

// Span is a member of a trace
type Span struct {
	tracer Tracer

	*Trace

	recordErr error

	// These are currently ignored
	logLines []opentracinglog.Field
}

// Finish ends a trace end records it with DefaultClient.
func (s *Span) Finish() {
	s.ClientFinish(DefaultClient)
}

// ClientFinish ends a trace and records it with the given Client.
func (s *Span) ClientFinish(cl *Client) {
	// This should never happen,
	// but calling defer span.Finish() should always be
	// a safe operation.
	if s == nil {
		return
	}
	s.ClientFinishWithOptions(cl, opentracing.FinishOptions{
		FinishTime:  time.Now(),
		LogRecords:  nil,
		BulkLogData: nil,
	})
}

// FinishWithOptions finishes the span, but with explicit
// control over timestamps and log data.
// The BulkLogData field is deprecated and ignored.
func (s *Span) FinishWithOptions(opts opentracing.FinishOptions) {
	s.ClientFinishWithOptions(DefaultClient, opts)
}

// ClientFinishWithOptions finishes the span and records it on the
// given client, but with explicit control over timestamps and log
// data.  The BulkLogData field is deprecated and ignored.
func (s *Span) ClientFinishWithOptions(cl *Client, opts opentracing.FinishOptions) {
	// This should never happen,
	// but calling defer span.FinishWithOptions() should always be
	// a safe operation.
	if s == nil {
		return
	}

	// TODO remove the name tag from the slice of tags

	s.recordErr = s.ClientRecord(cl, s.Name, s.Tags)
}

func (s *Span) Context() opentracing.SpanContext {
	if s == nil {
		return nil
	}
	return s.context()
}

// contextAsParent() is like its exported counterpart,
// except it returns the concrete type for local package use
func (s *Span) contextAsParent() *spanContext {
	//TODO baggageItems

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(s.TraceID, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(s.ParentID, 10)
	c.baggageItems[ResourceKey] = s.Resource
	return c
}

// SetOperationName sets the name of the operation being performed
// in this span.
func (s *Span) SetOperationName(name string) opentracing.Span {
	s.Trace.Resource = name
	return s
}

// SetTag sets the tags on the underlying span
func (s *Span) SetTag(key string, value interface{}) opentracing.Span {
	if s.Tags == nil {
		s.Tags = map[string]string{}
	}
	var val string

	// TODO mutex
	switch v := value.(type) {
	case string:
		val = v
	case fmt.Stringer:
		val = v.String()
	default:
		// TODO maybe just ban non-strings?
		val = fmt.Sprintf("%#v", value)
	}
	s.Tags[key] = val
	return s
}

// Attach attaches the span to the context.
// It delegates to opentracing.ContextWithSpan
func (s *Span) Attach(ctx context.Context) context.Context {
	return opentracing.ContextWithSpan(ctx, s)
}

// LogFields sets log fields on the underlying span.
// Currently these are ignored, but they can be fun to set anyway!
func (s *Span) LogFields(fields ...opentracinglog.Field) {
	// TODO mutex this
	s.logLines = append(s.logLines, fields...)
}

func (s *Span) LogKV(alternatingKeyValues ...interface{}) {
	// TODO handle error
	fs, _ := opentracinglog.InterleavedKVToFields(alternatingKeyValues...)
	s.LogFields(fs...)
}

// SetBaggageItem sets the value of a baggage in the span.
func (s *Span) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	s.contextAsParent().baggageItems[restrictedKey] = value
	return s
}

// BaggageItem fetches the value of a baggage item in the span.
func (s *Span) BaggageItem(restrictedKey string) string {
	return s.contextAsParent().baggageItems[restrictedKey]
}

// Tracer returns the tracer that created this Span
func (s *Span) Tracer() opentracing.Tracer {
	return s.tracer
}

// LogEvent is deprecated and unimplemented.
// It is included only to satisfy the opentracing.Span interface.
func (s *Span) LogEvent(event string) {
}

// LogEventWithPayload is deprecated and unimplemented.
// It is included only to satisfy the opentracing.Span interface.
func (s *Span) LogEventWithPayload(event string, payload interface{}) {
}

// Log is deprecated and unimplemented.
// It is included only to satisfy the opentracing.Span interface.
func (s *Span) Log(data opentracing.LogData) {
}

// Tracer is a tracer
type Tracer struct {
}

type spanOption struct {
	apply func(*opentracing.StartSpanOptions)
}

func (so *spanOption) Apply(sso *opentracing.StartSpanOptions) {
	so.apply(sso)
}

// customSpanStart returns a StartSpanOption that can be passed to
// StartSpan, and which will set the created Span's StartTime to the specified
// value.
func customSpanStart(t time.Time) opentracing.StartSpanOption {
	return &spanOption{
		apply: func(sso *opentracing.StartSpanOptions) {
			sso.StartTime = t
		},
	}
}

func customSpanTags(k, v string) opentracing.StartSpanOption {
	return &spanOption{
		apply: func(sso *opentracing.StartSpanOptions) {
			if sso.Tags == nil {
				sso.Tags = map[string]interface{}{}
			}
			sso.Tags[k] = v
		},
	}
}

func customSpanParent(t *Trace) opentracing.StartSpanOption {
	return &spanOption{
		apply: func(sso *opentracing.StartSpanOptions) {
			sso.References = append(sso.References, opentracing.SpanReference{
				Type:              opentracing.ChildOfRef,
				ReferencedContext: t.context(),
			})
		},
	}
}

// StartSpan starts a span with the specified operationName (name) and options.
// If the options specify a parent span and/or root trace, the name from the
// root trace will be used.
// The value returned is always a concrete Span (which satisfies the opentracing.Span interface)
func (t Tracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	// TODO implement References

	sso := opentracing.StartSpanOptions{
		Tags: map[string]interface{}{},
	}
	for _, o := range opts {
		o.Apply(&sso)
	}

	span := &Span{}

	if len(sso.References) == 0 {
		// This is a root-level span
		// beginning a new trace
		span = &Span{
			Trace:  StartTrace(operationName),
			tracer: t,
		}
	} else {

		// First, let's extract the parent's information
		parent := Trace{}

		// TODO don't assume that the ReferencedContext is a concrete spanContext
		for _, ref := range sso.References {
			// at the moment, I believe Datadog treats children and follow-children
			// the same way
			switch ref.Type {
			case opentracing.FollowsFromRef:
				fallthrough
			case opentracing.ChildOfRef:
				ctx, ok := ref.ReferencedContext.(*spanContext)
				if !ok {
					continue
				}
				parent.TraceID = ctx.TraceID()
				parent.SpanID = ctx.SpanID()
				parent.Resource = ctx.Resource()

			default:
				// TODO handle error
			}
		}

		// TODO allow us to start the trace as a separate operation
		// to prevent measurement error in timing
		trace := StartChildSpan(&parent)

		if !sso.StartTime.IsZero() {
			trace.Start = sso.StartTime
		}

		span = &Span{
			Trace:  trace,
			tracer: t,
		}
	}

	for k, v := range sso.Tags {
		span.SetTag(k, v)
		if k == "name" {
			span.Name = v.(string)
		}
	}

	if span.Name == "" {
		pc, _, _, ok := runtime.Caller(1)
		details := runtime.FuncForPC(pc)
		if ok && details != nil {
			span.Name = stripPackageName(details.Name())
		}
	}

	return span

}

// InjectRequest injects a trace into an HTTP request header.
// It is a convenience function for Inject.
func (tracer Tracer) InjectRequest(t *Trace, req *http.Request) error {
	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	return tracer.Inject(t.context(), opentracing.HTTPHeaders, carrier)
}

// ExtractRequestChild extracts a span from an HTTP request
// and creates and returns a new child of that span
func (tracer Tracer) ExtractRequestChild(resource string, req *http.Request, name string) (*Span, error) {
	carrier := opentracing.HTTPHeadersCarrier(req.Header)
	parentSpan, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	if err != nil {
		return nil, err
	}

	parent := parentSpan.(*spanContext)

	t := StartChildSpan(&Trace{
		SpanID:   parent.SpanID(),
		TraceID:  parent.TraceID(),
		ParentID: parent.ParentID(),
		Resource: resource,
	})

	t.Name = name
	return &Span{
		tracer: tracer,
		Trace:  t,
	}, nil
}

// Inject injects the provided SpanContext into the carrier for propagation.
// It will return opentracing.ErrUnsupportedFormat if the format is not supported.
// TODO support other SpanContext implementations
// TODO support all the BuiltinFormats
func (t Tracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// TODO annotate this error type
			err = ErrContractViolation{r}
		}
	}()

	sc, ok := sm.(*spanContext)
	if !ok {
		return ErrUnsupportedSpanContext
	}

	if format == opentracing.Binary {
		// carrier is guaranteed to be an io.Writer by contract
		w := carrier.(io.Writer)

		trace := &Trace{
			TraceID:  sc.TraceID(),
			ParentID: sc.ParentID(),
			SpanID:   sc.SpanID(),
			Resource: sc.Resource(),
			Tags:     map[string]string{ResourceKey: sc.Resource()},
		}

		return trace.ProtoMarshalTo(w)
	}

	// If the carrier is a TextMapWriter, treat it as one, regardless of what the format is
	if w, ok := carrier.(opentracing.TextMapWriter); ok {

		textMapReaderWriter(sc.baggageItems).CloneTo(w)
		return nil
	}

	return opentracing.ErrUnsupportedFormat
}

// Extract returns a SpanContext given the format and the carrier.
// The SpanContext returned represents the parent span (ie, SpanId refers to the parent span's own SpanId).
// TODO support all the BuiltinFormats
func (t Tracer) Extract(format interface{}, carrier interface{}) (ctx opentracing.SpanContext, err error) {
	defer func() {
		if r := recover(); r != nil {
			// TODO annotate this error type
			err = ErrContractViolation{r}
		}
	}()

	if format == opentracing.Binary {
		// carrier is guaranteed to be an io.Reader by contract
		r := carrier.(io.Reader)
		packet, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}

		sample := ssf.SSFSpan{}
		err = proto.Unmarshal(packet, &sample)
		if err != nil {
			return nil, err
		}

		resource := sample.Tags[ResourceKey]

		trace := &Trace{
			TraceID:  sample.TraceId,
			SpanID:   sample.Id,
			Resource: resource,
		}

		return trace.context(), nil
	}

	if tm, ok := carrier.(opentracing.TextMapReader); ok {
		// carrier is guaranteed to be an opentracing.TextMapReader by contract
		// TODO support other TextMapReader implementations
		parsedHeaders := false
		var traceID int64
		var spanID int64
		for _, headers := range HeaderFormats {
			var err1 error
			var err2 error
			traceID, err1 = strconv.ParseInt(textMapReaderGet(tm, headers.TraceID), 10, 64)
			spanID, err2 = strconv.ParseInt(textMapReaderGet(tm, headers.SpanID), 10, 64)

			if err1 == nil && err2 == nil {
				parsedHeaders = true
				break
			}
		}
		if !parsedHeaders {
			return nil, errors.New("error parsing fields from TextMapReader")
		}

		trace := &Trace{
			TraceID:  traceID,
			SpanID:   spanID,
			Resource: textMapReaderGet(tm, ResourceKey),
		}

		return trace.context(), nil
	}

	return nil, opentracing.ErrUnsupportedFormat
}

func textMapReaderGet(tmr opentracing.TextMapReader, key string) (value string) {
	tmr.ForeachKey(func(k, v string) error {
		if strings.ToLower(key) == strings.ToLower(k) {
			value = v
			// terminate early by returning an error
			return errors.New("dummy")
		}
		return nil
	})
	return value
}
