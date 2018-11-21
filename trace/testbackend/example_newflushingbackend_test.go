package testbackend_test

import (
	"context"
	"fmt"

	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/testbackend"
)

func ExampleNewFlushingBackend() {
	ctx := context.Background()

	// The channel is buffered so we don't have to do async
	// receives:
	ch := make(chan []*ssf.SSFSpan, 1)
	client, _ := trace.NewBackendClient(testbackend.NewFlushingBackend(ch))

	span, ctx := trace.StartSpanFromContext(ctx, "hi_there")
	span.ClientFinish(client)

	trace.Flush(client)

	rcvd := <-ch
	fmt.Println(rcvd[0].Name)
	// Output:
	// hi_there
}
