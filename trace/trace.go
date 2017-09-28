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
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stripe/veneur/ssf"

	"github.com/Sirupsen/logrus"
	"github.com/golang/protobuf/proto"
	opentracing "github.com/opentracing/opentracing-go"
)

// Experimental
const ResourceKey = "resource"

func init() {
	rand.Seed(time.Now().Unix())
}

// (Experimental)
// If this is set to true,
// traces will be generated but not actually sent.
// This should only be set before any traces are generated
var disabled bool = false

var enabledMtx sync.RWMutex

// Make an unexported `key` type that we use as a String such
// that we don't get lint warnings from using it as a key in
// Context. See https://blog.golang.org/context#TOC_3.2.
type key string

const traceKey key = "trace"

// Service is our service name and should be set exactly once,
// at startup
var Service = ""

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
	TraceID int64

	// For the root span, this will be equal
	// to the TraceId
	SpanID int64

	// For the root span, this will be <= 0
	ParentID int64

	// The Resource should be the same for all spans in the same trace
	Resource string

	Start time.Time

	End time.Time

	// If non-zero, the trace will be treated
	// as an error
	Status ssf.SSFSample_Status

	Tags map[string]string

	// Unlike the Resource, this should not contain spaces
	// It should be of the format foo.bar.baz
	Name string

	// Sent holds a channel. If set, this channel receives an
	// error (or nil) when the span has been serialized and sent.
	Sent chan<- error

	error bool
}

// Set the end timestamp and finalize Span state
func (t *Trace) finish() {
	if t.End.IsZero() {
		t.End = time.Now()
	}
}

// (Experimental)
// Enabled sets tracing to enabled.
func Enable() {
	enabledMtx.Lock()
	defer enabledMtx.Unlock()

	disabled = false
}

// (Experimental)
// Disabled sets tracing to disabled.
func Disable() {
	enabledMtx.Lock()
	defer enabledMtx.Unlock()

	disabled = true
}

func Disabled() bool {
	enabledMtx.RLock()
	defer enabledMtx.RUnlock()

	return disabled
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

// SSFSample converts the Trace to an SSFSpan type.
// It sets the duration, so it assumes the span has already ended.
// (It is safe to call on a span that has not ended, but the duration
// field will be invalid)
func (t *Trace) SSFSpan() *ssf.SSFSpan {
	name := t.Name

	span := &ssf.SSFSpan{
		StartTimestamp: t.Start.UnixNano(),
		Error:          t.error,
		TraceId:        t.TraceID,
		Id:             t.SpanID,
		ParentId:       t.ParentID,
		EndTimestamp:   t.End.UnixNano(),
		Name:           name,
		Tags:           t.Tags,
		Service:        Service,
	}

	return span
}

// ProtoMarshalTo writes the Trace as a protocol buffer
// in text format to the specified writer.
func (t *Trace) ProtoMarshalTo(w io.Writer) error {
	packet, err := proto.Marshal(t.SSFSpan())
	if err != nil {
		return err
	}
	_, err = w.Write(packet)
	return err
}

// Record sends a trace to a veneur instance using the DefaultClient .
func (t *Trace) Record(name string, tags map[string]string) error {
	return t.ClientRecord(DefaultClient, name, tags)
}

// ClientRecord uses the given client to send a trace to a veneur
// instance.
func (t *Trace) ClientRecord(cl *Client, name string, tags map[string]string) error {
	if t.Tags == nil {
		t.Tags = map[string]string{}
	}
	t.finish()

	for k, v := range tags {
		t.Tags[k] = v
	}

	if name == "" {
		name = t.Name
	}

	span := t.SSFSpan()
	span.Name = name

	return Record(cl, span, t.Sent)
}

func (t *Trace) Error(err error) {
	t.Status = ssf.SSFSample_CRITICAL
	t.error = true

	errorType := reflect.TypeOf(err).Name()
	if errorType == "" {
		errorType = "error"
	}

	t.Tags[errorMessageTag] = err.Error()
	t.Tags[errorTypeTag] = errorType
	t.Tags[errorStackTag] = err.Error()
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

	if name == "" {
		pc, _, _, ok := runtime.Caller(1)
		details := runtime.FuncForPC(pc)
		if ok && details != nil {
			name = stripPackageName(details.Name())
		}
	}

	sp, c := opentracing.StartSpanFromContext(ctx, name, opts...)

	s = sp.(*Span)
	s.Name = name
	return s, c
}

// SetParent updates the ParentId, TraceId, and Resource of a trace
// based on the parent's values (SpanId, TraceId, Resource).
func (t *Trace) SetParent(parent *Trace) {
	t.ParentID = parent.SpanID
	t.TraceID = parent.TraceID
	t.Resource = parent.Resource
}

// context returns a spanContext representing the trace
// from the point of view of itself .
// (The parentid for the trace will be set as the parentid for the context)
func (t *Trace) context() *spanContext {

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(t.TraceID, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(t.ParentID, 10)
	c.baggageItems["spanid"] = strconv.FormatInt(t.SpanID, 10)
	c.baggageItems[ResourceKey] = t.Resource
	return c
}

// contextAsParent returns a spanContext representing the trace
// from the point of view of its direct children.
// (The SpanId for the trace will be set as the ParentId for the context)
func (t *Trace) contextAsParent() *spanContext {

	c := &spanContext{}
	c.Init()
	c.baggageItems["traceid"] = strconv.FormatInt(t.TraceID, 10)
	c.baggageItems["parentid"] = strconv.FormatInt(t.SpanID, 10)
	c.baggageItems[ResourceKey] = t.Resource
	return c
}

// StartTrace is called by to create the root-level span
// for a trace
func StartTrace(resource string) *Trace {
	traceID := proto.Int64(rand.Int63())

	t := &Trace{
		TraceID:  *traceID,
		SpanID:   *traceID,
		ParentID: 0,
		Resource: resource,
		Tags:     map[string]string{},
	}

	t.Start = time.Now()
	return t
}

// StartChildSpan creates a new Span with the specified parent
func StartChildSpan(parent *Trace) *Trace {
	spanID := proto.Int64(rand.Int63())
	span := &Trace{
		SpanID: *spanID,
	}

	span.SetParent(parent)
	span.Start = time.Now()

	return span
}

// stripPackageName strips the package name from a function
// name (as formatted by the runtime package)
func stripPackageName(name string) string {
	i := strings.LastIndex(name, "/")
	if i < 0 || i >= len(name)-1 {
		return name
	}

	return name[i+1:]
}
