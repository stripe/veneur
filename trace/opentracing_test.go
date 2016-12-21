// This package tests the OpenTracing API
// which delegates to our own tracing implementation
package trace

import (
	"testing"
	"time"

	"github.com/stripe/veneur/ssf"

	"github.com/opentracing/opentracing-go"
	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, trace.TraceId, trace.SpanId)
	assert.Equal(t, trace.ParentId, expectedParent)
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
	var expectedTags = []*ssf.SSFTag{
		{
			Name:  "foo",
			Value: "bar",
		},
		{
			Name:  "baz",
			Value: "quz",
		},
	}

	tracer := Tracer{}

	parent := StartTrace(resource)
	var expectedParent = parent.SpanId

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

	assert.Equal(t, parent.TraceId, parent.SpanId)
	assert.Equal(t, expectedParent, trace.ParentId)
	assert.Equal(t, resource, trace.Resource)

	assert.Len(t, trace.Tags, len(expectedTags))

	for _, tag := range expectedTags {
		assert.Contains(t, trace.Tags, tag)
	}
}
