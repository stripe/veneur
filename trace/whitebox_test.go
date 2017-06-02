package trace_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/trace"
)

func TestStartSpanDefaultName(t *testing.T) {
	const resource = "TestResourceName"
	const expectedName = "github.com/stripe/veneur/trace_test.TestStartSpanDefaultName"

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
	const expectedName = "github.com/stripe/veneur/trace_test.TestStartSpanFromContextDefaultName"

	ctx := context.Background()
	tracer := trace.Tracer{}
	span := tracer.StartSpan(resource).(*trace.Span)
	ctx = span.Attach(ctx)

	span, _ = trace.StartSpanFromContext(ctx, "")

	assert.Equal(t, span.Resource, resource)
	assert.Equal(t, span.Name, expectedName)

}
