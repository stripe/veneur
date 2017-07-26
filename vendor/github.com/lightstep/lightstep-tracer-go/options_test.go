package lightstep

import (
	"testing"

	"github.com/lightstep/lightstep-tracer-go/basictracer"
	"github.com/stretchr/testify/assert"
)

func TestSpan_SetID3(t *testing.T) {
	var (
		expectedTraceID      uint64 = 1
		expectedSpanID       uint64 = 2
		expectedParentSpanID uint64 = 3
	)

	recorder := basictracer.NewInMemoryRecorder()
	tracer := basictracer.NewWithOptions(basictracer.Options{Recorder: recorder})

	tracer.StartSpan("x", SetTraceID(1), SetSpanID(2), SetParentSpanID(3)).Finish()

	spans := recorder.GetSpans()
	assert.Equal(t, 1, len(spans))

	span := spans[0]
	assert.Equal(t, span.Context.TraceID, expectedTraceID)
	assert.Equal(t, span.Context.SpanID, expectedSpanID)
	assert.Equal(t, span.ParentSpanID, expectedParentSpanID)
}

func TestSpan_SetID2(t *testing.T) {
	var (
		expectedTraceID uint64 = 1
		expectedSpanID  uint64 = 2
	)

	recorder := basictracer.NewInMemoryRecorder()
	tracer := basictracer.NewWithOptions(basictracer.Options{Recorder: recorder})

	tracer.StartSpan("x", SetTraceID(1), SetSpanID(2)).Finish()

	spans := recorder.GetSpans()
	assert.Equal(t, 1, len(spans))

	span := spans[0]
	assert.Equal(t, span.Context.TraceID, expectedTraceID)
	assert.Equal(t, span.Context.SpanID, expectedSpanID)
	assert.Equal(t, span.ParentSpanID, uint64(0))
}

func TestSpan_SetID1(t *testing.T) {
	var (
		expectedTraceID uint64 = 1
	)

	recorder := basictracer.NewInMemoryRecorder()
	tracer := basictracer.NewWithOptions(basictracer.Options{Recorder: recorder})

	tracer.StartSpan("x", SetTraceID(1)).Finish()

	spans := recorder.GetSpans()
	assert.Equal(t, 1, len(spans))

	span := spans[0]
	assert.Equal(t, span.Context.TraceID, expectedTraceID)
	assert.NotEqual(t, span.Context.SpanID, uint64(0))
	assert.Equal(t, span.ParentSpanID, uint64(0))
}
