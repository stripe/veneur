package testbackend_test

import (
	"context"
	"fmt"

	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/testbackend"
)

func ExampleNewBackend() {
	ctx := context.Background()

	// The channel is buffered so we don't have to do async
	// receives:
	ch := make(chan *ssf.SSFSpan, 1)
	client, _ := trace.NewBackendClient(testbackend.NewBackend(ch))

	span, ctx := trace.StartSpanFromContext(ctx, "hi_there")
	span.ClientFinish(client)
	rcvd := <-ch
	fmt.Println(rcvd.Name)
	// Output:
	// hi_there
}
