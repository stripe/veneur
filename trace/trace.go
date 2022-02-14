package trace

import (
	"context"
	"io"
	"math/rand"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/protobuf/proto"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/stripe/veneur/v14/ssf"
	"golang.org/x/mod/module"
)

// Experimental
const ResourceKey = "resource"

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

// Service is our service name and must be set exactly once, by the
// main package. It is recommended to set this value in an init()
// function or at the beginning of the main() function.
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

	// Samples holds a list of samples / metrics to be reported
	// alongside a span.
	Samples []*ssf.SSFSample

	// An indicator span is one that represents an action that is included in a
	// service's Service Level Indicators (https://en.wikipedia.org/wiki/Service_level_indicator)
	// For more information, see the SSF definition at https://github.com/stripe/veneur/tree/master/ssf
	Indicator bool

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

// SSFSpan converts the Trace to an SSFSpan type.
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
		Metrics:        t.Samples,
		Indicator:      t.Indicator,
	}

	return span
}

// Add adds a number of metrics/samples to a Trace.
func (t *Trace) Add(samples ...*ssf.SSFSample) {
	t.Samples = append(t.Samples, samples...)
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

	if t.Tags == nil {
		t.Tags = map[string]string{}
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
// when the parent trace is in the context.
//
// Recommended usage
//
// SpanFromContext is considered experimental. Prefer
// StartSpanFromContext instead.
//
// Compatibility with OpenTracing
//
// SpanFromContext behaves differently from
// opentracing.SpanFromContext: The opentracing function returns the
// exact span that is stored on the Context, but this function
// allocates a new span with a parent ID set to that of the span
// stored on the context.
func SpanFromContext(c context.Context) *Trace {
	parent := c.Value(traceKey).(*Trace)
	return StartChildSpan(parent)
}

// StartSpanFromContext creates a span that fits into a span tree -
// child or root. It makes a new span, find a parent span if it exists
// on the context and fills in the data in the newly-created span to
// ensure a proper parent-child relationship (if no parent exists, is
// sets the new span up as a root Span, i.e., a Trace). Then
// StartSpanFromContext sets the newly-created span as the current
// span on a child Context. That child context is returned also.
//
// Name inference
//
// StartSpanFromContext takes a name argument. If passed "", it will
// infer the current function's name as the span name.
//
// Recommended usage
//
// StartSpanFromContext is the recommended way to create trace spans in a
// program.
func StartSpanFromContext(
	ctx context.Context, name string, opts ...opentracing.StartSpanOption,
) (s *Span, c context.Context) {
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
// name (as formatted by the runtime package). If the module has a
// major version >1 and the function is in the top-level package,
// it will be reported by the runtime as "module/foo/vN.Bar". This
// function strips the vN portion so that the above case becomes
// "foo.Bar".
// As a special case, gopkg.in paths are recognized directly.
// They require ".vN" instead of "/vN", and for all N, not just N >= 2.
// The version suffix is NOT stripped from gopkg.in paths.
func stripPackageName(name string) string {
	if strings.HasPrefix(name, "gopkg.in/") {
		return stripPackageNameGoPkgIn(name)
	}

	idx := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '/' {
			break
		}
		if name[i] == '.' {
			idx = i
		}
	}
	if idx == -1 || idx >= len(name) {
		return name
	}
	p, _, ok := module.SplitPathVersion(name[:idx])
	if !ok {
		return name
	}

	return path.Base(p) + name[idx:]
}

func stripPackageNameGoPkgIn(name string) string {
	i := strings.LastIndex(name, "/")
	if i < 0 || i >= len(name)-1 {
		return name
	}

	// The compiler escapes the package path.
	// See golang.org/issue/35558
	return strings.Replace(name[i+1:], "%2e", ".", 1)
}
