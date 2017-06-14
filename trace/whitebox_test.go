package trace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/trace"
)

func TestStartSpanDefaultName(t *testing.T) {
	const resource = "TestResourceName"
	const expectedName = "trace_test.TestStartSpanDefaultName"

	ctx := context.Background()
	tracer := trace.Tracer{}
	span := tracer.StartSpan(resource).(*trace.Span)
	ctx = span.Attach(ctx)

	_, _ = trace.StartSpanFromContext(ctx, "")

	assert.Equal(t, span.Resource, resource)
	assert.Equal(t, span.Name, expectedName)

}

func TestStartSpanFromContextDefaultName(t *testing.T) {
	const resource = "TestResourceName"
	const expectedName = "trace_test.TestStartSpanFromContextDefaultName"

	ctx := context.Background()
	tracer := trace.Tracer{}
	root := tracer.StartSpan(resource).(*trace.Span)
	ctx = root.Attach(ctx)

	span, _ := trace.StartSpanFromContext(ctx, "")

	assert.Equal(t, span.Resource, resource)
	assert.Equal(t, span.Name, expectedName)
	assert.Equal(t, span.ParentID, root.SpanID)
	assert.Equal(t, span.TraceID, root.SpanID)

	ctx = span.Attach(ctx)

	grandchild, _ := trace.StartSpanFromContext(ctx, "")

	assert.Equal(t, grandchild.TraceID, root.SpanID)
	assert.Equal(t, grandchild.ParentID, span.SpanID)

}

// StartSpanFromContext should create a brand-new root span
// if the context does not contain a span
func TestSpanFromContextNoParent(t *testing.T) {
	const resource = "example"
	ctx := context.Background()

	span, _ := trace.StartSpanFromContext(ctx, resource)

	assert.Equal(t, span.TraceID, span.SpanID)
	assert.Equal(t, int64(0), span.ParentID)
}
