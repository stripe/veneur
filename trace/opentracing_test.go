// This package tests the OpenTracing API
// which delegates to our own tracing implementation
package trace

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/ssf"
)

// Test that the Tracer can correctly create a root-level
// span.
// This is similar to TestStartTrace, but it tests the
// OpenTracing API for the same operations.
func TestTracerRootSpan(t *testing.T) {
	// TODO test tags!
	const resource = "Robert'); DROP TABLE students;"
	const expectedParent int64 = 0

	tracer := Tracer{}

	start := time.Now()
	trace := tracer.StartSpan(resource).(*Span)
	end := time.Now()

	between := end.After(trace.Start) && trace.Start.After(start)

	assert.Equal(t, trace.TraceID, trace.SpanID)
	assert.Equal(t, trace.ParentID, expectedParent)
	assert.Equal(t, trace.Resource, resource)
	assert.True(t, between)
}

// Test that the Tracer can correctly create a child span
func TestTracerChildSpan(t *testing.T) {
	// TODO test grandchild as well
	// to ensure we can do nested children properly

	const resource = "Robert'); DROP TABLE students;"
	// This will be a *really* slow trace!
	const expectedTimestamp = 1136239445
	var expectedTags = map[string]string{
		"foo": "bar",
		"baz": "quz",
	}

	tracer := Tracer{}

	parent := StartTrace(resource)
	var expectedParent = parent.SpanID

	start := time.Now()
	opts := []opentracing.StartSpanOption{
		customSpanStart(time.Unix(expectedTimestamp, 0)),
		customSpanParent(parent),
		customSpanTags("foo", "bar"),
		customSpanTags("baz", "quz"),
	}
	trace := tracer.StartSpan(resource, opts...).(*Span)
	end := time.Now()

	// The end time should be something between these two
	between := end.After(trace.End) && trace.End.After(start)
	assert.False(t, between)

	assert.Equal(t, time.Unix(expectedTimestamp, 0), trace.Start)

	assert.Equal(t, parent.TraceID, parent.SpanID)
	assert.Equal(t, expectedParent, trace.ParentID)
	assert.Equal(t, resource, trace.Resource)

	assert.Len(t, trace.Tags, len(expectedTags))

	for k, v := range expectedTags {
		assert.Equal(t, trace.Tags[k], v)
	}
}

// DummySpan is a helper function that gives
// a simple Span to use in tests
func DummySpan() *Span {
	const resource = "Robert'); DROP TABLE students;"
	const expectedTimestamp = 1136239445
	tracer := &Tracer{}

	parent := StartTrace(resource)
	opts := []opentracing.StartSpanOption{
		customSpanStart(time.Unix(expectedTimestamp, 0)),
		customSpanParent(parent),
		customSpanTags("foo", "bar"),
		customSpanTags("baz", "quz"),
	}
	trace := tracer.StartSpan(resource, opts...).(*Span)
	return trace
}

// TestTracerInjectBinary tests that we can inject
// a protocol buffer using the Binary format.
func TestTracerInjectBinary(t *testing.T) {
	trace := DummySpan().Trace

	trace.finish()

	tracer := Tracer{}
	var b bytes.Buffer

	err := tracer.Inject(trace.context(), opentracing.Binary, &b)
	assert.NoError(t, err)

	packet, err := ioutil.ReadAll(&b)
	assert.NoError(t, err)

	sample := &ssf.SSFSpan{}
	err = proto.Unmarshal(packet, sample)
	assert.NoError(t, err)

	assertContextUnmarshalEqual(t, trace, sample)
}

// TestTracerExtractBinary tests that we can extract
// a protobuf representing an SSF (using the Binary format)
func TestTracerExtractBinary(t *testing.T) {
	trace := DummySpan().Trace

	trace.finish()

	tracer := Tracer{}

	packet, err := proto.Marshal(trace.SSFSpan())
	assert.NoError(t, err)

	b := bytes.NewBuffer(packet)

	_, err = tracer.Extract(opentracing.Binary, b)
	assert.NoError(t, err)
}

// TestTracerInjectExtractBinary tests that we can inject a span
// and then Extract it (end-to-end).
func TestTracerInjectExtractBinary(t *testing.T) {
	trace := DummySpan().Trace
	tracer := Tracer{}
	var b bytes.Buffer
	var _ io.Reader = &b

	err := tracer.Inject(trace.context(), opentracing.Binary, &b)
	assert.NoError(t, err)

	c, err := tracer.Extract(opentracing.Binary, &b)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceID, ctx.TraceID())

	assert.Equal(t, trace.SpanID, ctx.SpanID(), "original trace and context should share the same SpanId")
	assert.Equal(t, trace.Resource, ctx.Resource())
}

// TestTracerInjectTextMap tests that we can inject
// a protocol buffer using the TextMap format.
func TestTracerInjectTextMap(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	tm := textMapReaderWriter(map[string]string{})

	err := tracer.Inject(trace.context(), opentracing.TextMap, tm)
	assert.NoError(t, err)

	assert.Equal(t, strconv.FormatInt(trace.TraceID, 10), tm["traceid"])
	assert.Equal(t, strconv.FormatInt(trace.SpanID, 10), tm["spanid"])
	assert.Equal(t, trace.Resource, tm["resource"])
}

// TestTracerInjectExtractBinary tests that we can inject a span
// and then Extract it (end-to-end).
func TestTracerInjectExtractExtractTextMap(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	tm := textMapReaderWriter(map[string]string{})

	err := tracer.Inject(trace.context(), opentracing.TextMap, tm)
	assert.NoError(t, err)

	c, err := tracer.Extract(opentracing.TextMap, tm)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceID, ctx.TraceID())

	assert.Equal(t, trace.SpanID, ctx.SpanID(), "original trace and context should share the same SpanId")
	assert.Equal(t, trace.Resource, ctx.Resource())
}

// TestTracerInjectExtractHeader tests that we can inject a span
// using HTTP headers and then extract it (end-to-end)
func TestTracerInjectExtractHeader(t *testing.T) {
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}

	req, err := http.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(nil))
	assert.NoError(t, err)

	carrier := opentracing.HTTPHeadersCarrier(req.Header)

	err = tracer.Inject(trace.context(), opentracing.HTTPHeaders, carrier)
	assert.NoError(t, err)

	assert.NotEqual(t, "", req.Header.Get(defaultHeaderFormat.SpanID),
		"Headers %v should contain %q", req.Header, defaultHeaderFormat.SpanID)
	assert.NotEqual(t, "", req.Header.Get(defaultHeaderFormat.TraceID),
		"Headers %v should contain %q", req.Header, defaultHeaderFormat.TraceID)

	c, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	assert.NoError(t, err)

	ctx := c.(*spanContext)

	assert.Equal(t, trace.TraceID, ctx.TraceID())

	assert.Equal(t, trace.SpanID, ctx.SpanID(), "original trace and context should share the same SpanId")
}

func TestTraceExtractHeaderEnvoy(t *testing.T) {
	tracer := Tracer{}
	tm := textMapReaderWriter(map[string]string{
		"ot-tracer-traceid": "3039",
		"ot-tracer-spanid":  "10932",
	})

	c, _ := tracer.Extract(opentracing.TextMap, tm)

	ctx := c.(*spanContext)

	assert.Equal(t, int64(12345), ctx.TraceID())
	assert.Equal(t, int64(67890), ctx.SpanID())
}

func TestTraceExtractHeaderOpenTracing(t *testing.T) {
	tracer := Tracer{}
	tm := textMapReaderWriter(map[string]string{
		"Trace-Id": "24680",
		"Span-Id":  "13579",
	})

	c, _ := tracer.Extract(opentracing.TextMap, tm)

	ctx := c.(*spanContext)

	assert.Equal(t, int64(24680), ctx.TraceID())
	assert.Equal(t, int64(13579), ctx.SpanID())
}

func TestTraceExtractHeaderError(t *testing.T) {
	tracer := Tracer{}
	tm := textMapReaderWriter(map[string]string{
		"wrong-header": "24680",
		"bad header":   "13579",
	})

	c, err := tracer.Extract(opentracing.TextMap, tm)

	assert.Nil(t, c)
	assert.EqualError(t, err, "error parsing fields from TextMapReader")
}

// assertContextUnmarshalEqual is a helper that asserts that the given SSFSpan
// matches the expected *Trace on all fields that are passed through a SpanContext.
// Since a SpanContext doesn't pass fields like tags, this function will not cause
// the assertion to fail if those differ.
// Since Resource and Name are passed in as tags, this will NOT check for equality on those fields!
func assertContextUnmarshalEqual(t *testing.T, expected *Trace, sample *ssf.SSFSpan) {
	assert.Equal(t, expected.SSFSpan().Metrics, sample.Metrics)
	assert.Equal(t, expected.SSFSpan().Error, sample.Error)

	// The TraceId, ParentId, and Resource should all be the same.
	assert.Equal(t, expected.SSFSpan().TraceId, sample.TraceId)
	assert.Equal(t, expected.SSFSpan().ParentId, sample.ParentId)
	assert.Equal(t, expected.SSFSpan().Id, sample.Id)
}

// assertTagEqual checks that the two representations share the same value
// for the specified tag name
func assertTagEqual(t *testing.T, tagName string, expected *Trace, sample *ssf.SSFSpan) {
	assert.Equal(t, expected.Tags[tagName], sample.Tags[tagName], "Tag '%s' has wrong value", tagName)
}

// TestInjectRequestExtractRequestChild tests the InjectRequest, InjectHeader,
// and ExtractRequestChild helper functions
func TestInjectRequestInjectHeaderExtractRequestChild(t *testing.T) {
	const childResource = "my child resource"
	const traceName = "my.child.name"
	trace := DummySpan().Trace
	trace.finish()
	tracer := Tracer{}
	req, err := http.NewRequest(http.MethodPost, "/test", bytes.NewBuffer(nil))
	assert.NoError(t, err)

	req2, err := http.NewRequest(http.MethodPost, "/test2", bytes.NewBuffer(nil))
	assert.NoError(t, err)

	err = tracer.InjectRequest(trace, req)
	assert.NoError(t, err)

	err = tracer.InjectHeader(trace, req2.Header)
	assert.NoError(t, err)
	assert.True(t, reflect.DeepEqual(req.Header, req2.Header), "injected HTTP headers should be the same")

	span, err := tracer.ExtractRequestChild(childResource, req, traceName)
	assert.NoError(t, err)

	assert.NotEqual(t, trace.SpanID, span.SpanID, "original trace and child should have different SpanIds")
	assert.Equal(t, trace.SpanID, span.ParentID, "child should have the original trace's SpanId as its ParentId")
	assert.Equal(t, trace.TraceID, span.TraceID)
}
