// Package trace provies an experimental API for initiating
// traces. Veneur's tracing API also provides
// an opentracing compatibility layer. The Veneur tracing API
// is completely independent of the opentracing
// compatibility layer, with the exception of one convenience
// function.
package trace

import (
	"context"
	"io"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/stripe/veneur/ssf"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	opentracing "github.com/opentracing/opentracing-go"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// (Experimental)
// If this is set to true,
// traces will be generated but not actually sent.
// This should only be set before any traces are generated
var disabled bool = false

var disabledMtx = &sync.RWMutex{}

// Disable is an experimental function. If this
// is run, traces will be generated but not actually sent.
// This should only be set before any traces are generated
func Disable() {
	disabledMtx.Lock()
	defer disabledMtx.Unlock()

	disabled = true
}

// Enable is an experimental function that will re-enable
// traces if they have been disabled. (This is intended to
// be used only by testing functions).
func Enable() {
	disabledMtx.Lock()
	defer disabledMtx.Unlock()

	disabled = false
}

// Disabled is an experimental function which
// indicates whether traces have been disabled
// (using the Disable function).
func Disabled() bool {
	disabledMtx.RLock()
	result := disabled
	defer disabledMtx.RUnlock()
	return result
}

const traceKey = "trace"

// this should be set exactly once, at startup
var Service = ""

const localVeneurAddress = "127.0.0.1:8128"

// For an error to be recorded correctly in DataDog, these three tags
// need to be set
const errorMessageTag = "error.msg"
const errorTypeTag = "error.type"
const errorStackTag = "error.stack"

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
func (t *Trace) finish() {
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
	t.finish()
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
			Name:  errorMessageTag,
			Value: err.Error(),
		},
		{
			Name:  errorTypeTag,
			Value: errorType,
		},
		{
			Name:  errorStackTag,
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

// StartSpanFromContext is used to create a child span
// when the parent trace is in the context
func StartSpanFromContext(ctx context.Context, name string, opts ...opentracing.StartSpanOption) (s *Span, c context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s = nil
			c = ctx
		}
	}()
	sp, c := opentracing.StartSpanFromContext(ctx, name, opts...)

	s = sp.(*Span)
	return s, c
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

	if Disabled() {
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
