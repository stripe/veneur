package trace_test

import (
	"context"
	"fmt"

	"github.com/stripe/veneur/trace"
)

func ExampleStartSpanFromContext() {
	ctx := context.TODO() // assume your code gets a Context from somewhere

	// Create a span:
	span, ctx := trace.StartSpanFromContext(ctx, "")
	defer span.Finish()

	span.Tags = map[string]string{
		"operation": "example",
	}

	// Function name inference will have guessed the name of this
	// example: "trace_test.ExampleStartSpanFromContext"
	fmt.Println(span.Name)
	// Output:
	// trace_test.ExampleStartSpanFromContext
}
