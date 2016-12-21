// This package tests the OpenTracing API
// which delegates to our own tracing implementation
package trace

import (
	"testing"
	"time"

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
