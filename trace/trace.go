package trace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/stripe/veneur/ssf"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	"github.com/opentracing/opentracing-go"
	opentracinglog "github.com/opentracing/opentracing-go/log"
)

var _ opentracing.Tracer = &Tracer{}
var _ opentracing.Span = &Span{}
var _ opentracing.SpanContext = &spanContext{}
var _ opentracing.StartSpanOption = &spanOption{}
var _ opentracing.TextMapReader = textMapReaderWriter(map[string]string{})
var _ opentracing.TextMapWriter = textMapReaderWriter(map[string]string{})

var ErrUnsupportedSpanContext = errors.New("Unsupported SpanContext")

// TODO make this more descriptive
var ErrContractViolation = errors.New("Contract violation")

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
func (c *spanContext) TraceId() int64 {
	return c.parseBaggageInt64("traceid")
}

// ParentID extracts the Parent ID from the BaggageItems.
// It assumes the ParentID is present and valid.
func (c *spanContext) ParentId() int64 {
	return c.parseBaggageInt64("parentid")
}

// SpanId extracts the Span ID from the BaggageItems.
// It assumes the SpanId is present and valid.
func (c *spanContext) SpanId() int64 {
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
		if strings.ToLower(k) == "resource" {
			resource = v
			return false
		}
		return true
	})
	return resource
}

type Span struct {
	tracer Tracer

	*Trace

	// These are currently ignored
	logLines []opentracinglog.Field
}

func (s *Span) Finish() {
	s.FinishWithOptions(opentracing.FinishOptions{
		FinishTime:  time.Now(),
		LogRecords:  nil,
		BulkLogData: nil,
	})

}

// FinishWithOptions finishes the span, but with explicit
// control over timestamps and log data.
// The BulkLogData field is deprecated and ignored.
func (s *Span) FinishWithOptions(opts opentracing.FinishOptions) {
}

func (s *Span) Context() opentracing.SpanContext {
	return s.contextAsParent()
}

// contextAsParent() is like its exported counterpart,
// except it returns the concrete type for local package use
func (s *Span) contextAsParent() *spanContext {
	//TODO baggageItems

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(s.TraceId, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(s.ParentId, 10)
	c.baggageItems["resource"] = s.Resource
	return c
}

func (s *Span) SetOperationName(name string) opentracing.Span {
	s.Trace.Resource = name
	return s
}

// SetTag sets the tags on the underlying span
func (s *Span) SetTag(key string, value interface{}) opentracing.Span {
	tag := ssf.SSFTag{Name: key}
	// TODO mutex
	switch v := value.(type) {
	case string:
		tag.Value = v
	case fmt.Stringer:
		tag.Value = v.String()
	default:
		// TODO maybe just ban non-strings?
		tag.Value = fmt.Sprintf("%#v", value)
	}
	s.Tags = append(s.Tags, &tag)
	return s
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

func (s *Span) SetBaggageItem(restrictedKey, value string) opentracing.Span {
	s.contextAsParent().baggageItems[restrictedKey] = value
	return s
}

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
				ReferencedContext: t.contextAsParent(),
			})
		},
	}
}

// StartSpan starts a span with the specified operationName (resource) and options.
// If the options specify a parent span and/or root trace, the resource from the
// root trace will be used.
func (t Tracer) StartSpan(operationName string, opts ...opentracing.StartSpanOption) opentracing.Span {
	// TODO implement References

	sso := opentracing.StartSpanOptions{
		Tags: map[string]interface{}{},
	}
	for _, o := range opts {
		o.Apply(&sso)
	}

	if len(sso.References) == 0 {
		// This is a root-level span
		// beginning a new trace
		return &Span{
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
				parent.TraceId = ctx.TraceId()
				parent.SpanId = ctx.ParentId()
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

		span := &Span{
			Trace:  trace,
			tracer: t,
		}

		for k, v := range sso.Tags {
			span.SetTag(k, v)
		}
		return span
	}
}

// Inject injects the provided SpanContext into the carrier for propagation.
// It will return opentracing.ErrUnsupportedFormat if the format is not supported.
// TODO support other SpanContext implementations
// TODO support all the BuiltinFormats
func (t Tracer) Inject(sm opentracing.SpanContext, format interface{}, carrier interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			// TODO annotate this error type
			err = ErrContractViolation
		}
	}()

	sc, ok := sm.(*spanContext)
	if !ok {
		return ErrUnsupportedSpanContext
	}

	switch format {
	case opentracing.Binary:
		// carrier is guaranteed to be an io.Writer by contract
		w := carrier.(io.Writer)

		trace := &Trace{
			TraceId:  sc.TraceId(),
			ParentId: sc.ParentId(),
			SpanId:   sc.SpanId(),
			Resource: sc.Resource(),
		}
		return trace.ProtoMarshalTo(w)

	case opentracing.TextMap:
	case opentracing.HTTPHeaders:
	}

	return opentracing.ErrUnsupportedFormat
}

// Extract returns a SpanContext given the format and the carrier.
// The SpanContext returned represents the parent span (ie, SpanId refers to the parent span's own SpanId).
// TODO support all the BuiltinFormats
func (t Tracer) Extract(format interface{}, carrier interface{}) (ctx opentracing.SpanContext, err error) {
	/*
		defer func() {
			if r := recover(); r != nil {
				// TODO annotate this error type
				err = ErrContractViolation
			}
		}()
	*/

	switch format {
	case opentracing.Binary:
		// carrier is guaranteed to be an io.Reader by contract
		r := carrier.(io.Reader)
		packet, err := ioutil.ReadAll(r)
		if err != nil {
			return nil, err
		}

		sample := ssf.SSFSample{}
		err = proto.Unmarshal(packet, &sample)
		if err != nil {
			return nil, err
		}

		trace := &Trace{
			TraceId:  sample.Trace.TraceId,
			ParentId: sample.Trace.ParentId,
			SpanId:   sample.Trace.Id,
			Resource: sample.Trace.Resource,
		}

		return trace.context(), nil

	case opentracing.TextMap:
	case opentracing.HTTPHeaders:
	}

	return nil, opentracing.ErrUnsupportedFormat
}

func init() {
	rand.Seed(time.Now().Unix())
}

// (Experimental)
// If this is set to true,
// traces will be generated but not actually sent.
// This should only be set before any traces are generated
var Disabled bool = false

const traceKey = "trace"

// this should be set exactly once, at startup
var Service = ""

const localVeneurAddress = "127.0.0.1:8128"

// Trace is a convenient structural representation
// of a TraceSpan. It is intended to map transparently
// to the more general type SSFSample.
type Trace struct {
	// the ID for the root span
	// which is also the ID for the trace itself
	TraceId int64

	// For the root span, this will be equal
	// to the TraceId
	SpanId int64

	// For the root span, this will be <= 0
	ParentId int64

	// The Resource should be the same for all spans in the same trace
	Resource string

	Start time.Time

	End time.Time

	// If non-zero, the trace will be treated
	// as an error
	Status ssf.SSFSample_Status

	Tags []*ssf.SSFTag

	// Unlike the Resource, this should not contain spaces
	// It should be of the format foo.bar.baz
	Name string
}

// Set the end timestamp and finalize Span state
func (t *Trace) Finish() {
	t.End = time.Now()
}

// Duration is a convenience function for
// the difference between the Start and End timestamps.
// It assumes the span has already ended.
func (t *Trace) Duration() time.Duration {
	if t.End.IsZero() {
		return -1
	}
	return t.End.Sub(t.Start)
}

// SSFSample converts the Trace to an SSFSample type.
// It sets the duration, so it assumes the span has already ended.
// (It is safe to call on a span that has not ended, but the duration
// field will be invalid)
func (t *Trace) SSFSample() *ssf.SSFSample {
	duration := t.Duration().Nanoseconds()
	name := t.Name

	return &ssf.SSFSample{
		Metric:    ssf.SSFSample_TRACE,
		Timestamp: t.Start.UnixNano(),
		Status:    t.Status,
		Name:      *proto.String(name),
		Trace: &ssf.SSFTrace{
			TraceId:  t.TraceId,
			Id:       t.SpanId,
			ParentId: t.ParentId,
			Duration: duration,
			Resource: t.Resource,
		},
		SampleRate: *proto.Float32(.10),
		Tags:       t.Tags,
		Service:    Service,
	}
}

// ProtoMarshalText writes the Trace as a protocol buffer
// in text format to the specified writer.
func (t *Trace) ProtoMarshalTo(w io.Writer) error {
	packet, err := proto.Marshal(t.SSFSample())
	if err != nil {
		return err
	}
	_, err = w.Write(packet)
	return err
}

// Record sends a trace to the (local) veneur instance,
// which will pass it on to the tracing agent running on the
// global veneur instance.
func (t *Trace) Record(name string, tags []*ssf.SSFTag) error {
	t.Finish()
	duration := t.Duration().Nanoseconds()

	t.Tags = append(t.Tags, tags...)

	if name == "" {
		name = t.Name
	}

	sample := &ssf.SSFSample{
		Metric:    ssf.SSFSample_TRACE,
		Timestamp: t.Start.UnixNano(),
		Status:    t.Status,
		Name:      *proto.String(name),
		Trace: &ssf.SSFTrace{
			TraceId:  t.TraceId,
			Id:       t.SpanId,
			ParentId: t.ParentId,
			Duration: duration,
			Resource: t.Resource,
		},
		SampleRate: *proto.Float32(.10),
		Tags:       t.Tags,
		Service:    Service,
	}

	err := sendSample(sample)
	if err != nil {
		logrus.WithError(err).Error("Error submitting sample")
	}
	return err
}

func (t *Trace) Error(err error) {
	t.Status = ssf.SSFSample_CRITICAL

	errorType := reflect.TypeOf(err).Name()
	if errorType == "" {
		errorType = "error"
	}

	tags := []*ssf.SSFTag{
		{
			Name:  "error.msg",
			Value: err.Error(),
		},
		{
			Name:  "error.type",
			Value: errorType,
		},
		{
			Name:  "error.stack",
			Value: err.Error(),
		},
	}

	t.Tags = append(t.Tags, tags...)
}

// Attach attaches the current trace to the context
// and returns a copy of the context with that trace
// stored under the key "trace".
func (t *Trace) Attach(c context.Context) context.Context {
	return context.WithValue(c, traceKey, t)
}

// SpanFromContext is used to create a child span
// when the parent trace is in the context
func SpanFromContext(c context.Context) *Trace {
	parent, ok := c.Value(traceKey).(*Trace)
	if !ok {
		logrus.WithField("type", reflect.TypeOf(c.Value(traceKey))).Error("expected *Trace from context")
	}

	return StartChildSpan(parent)
}

// SetParent updates the ParentId, TraceId, and Resource of a trace
// based on the parent's values (SpanId, TraceId, Resource).
func (t *Trace) SetParent(parent *Trace) {
	t.ParentId = parent.SpanId
	t.TraceId = parent.TraceId
	t.Resource = parent.Resource
}

// context returns a spanContext representing the trace
// from the point of view of itself .
// (The parentid for the trace will be set as the parentid for the context)
func (t *Trace) context() *spanContext {

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(t.TraceId, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(t.ParentId, 10)
	c.baggageItems["spanid"] = strconv.FormatInt(t.SpanId, 10)
	c.baggageItems["resource"] = t.Resource
	return c
}

// contextAsParent returns a spanContext representing the trace
// from the point of view of its direct children.
// (The SpanId for the trace will be set as the ParentId for the context)
func (t *Trace) contextAsParent() *spanContext {

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(t.TraceId, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(t.SpanId, 10)
	c.baggageItems["resource"] = t.Resource
	return c
}

// StartTrace is called by to create the root-level span
// for a trace
func StartTrace(resource string) *Trace {
	traceId := proto.Int64(rand.Int63())

	t := &Trace{
		TraceId:  *traceId,
		SpanId:   *traceId,
		ParentId: 0,
		Resource: resource,
	}

	t.Start = time.Now()
	return t
}

// StartChildSpan creates a new Span with the specified parent
func StartChildSpan(parent *Trace) *Trace {
	spanId := proto.Int64(rand.Int63())
	span := &Trace{
		SpanId: *spanId,
	}

	span.SetParent(parent)
	span.Start = time.Now()

	return span
}

// sendSample marshals the sample using protobuf and sends it
// over UDP to the local veneur instance
func sendSample(sample *ssf.SSFSample) error {
	if Disabled {
		return nil
	}

	server_addr, err := net.ResolveUDPAddr("udp", localVeneurAddress)
	if err != nil {
		return err
	}

	conn, err := net.DialUDP("udp", nil, server_addr)
	if err != nil {
		return err
	}

	defer conn.Close()

	data, err := proto.Marshal(sample)
	if err != nil {
		return err
	}

	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	return nil
}
