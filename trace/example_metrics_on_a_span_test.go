package trace_test

import (
	"context"
	"time"

	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
)

func ExampleTrace_Add() {
	// Create a span for testing and ensure it gets reported:
	span, _ := trace.StartSpanFromContext(context.Background(), "an_example")
	defer span.Finish()

	// Add a counter:
	span.Add(ssf.Count("hey.there", 1, map[string]string{
		"a.tag": "a value",
	}))
	// Add a timer:
	span.Add(ssf.Timing("some.duration", 3*time.Millisecond, time.Nanosecond, nil))
	// Output:
}
