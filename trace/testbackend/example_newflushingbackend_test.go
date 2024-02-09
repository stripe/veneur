package testbackend_test

import (
	"fmt"

	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"github.com/stripe/veneur/v14/trace/testbackend"
)

func ExampleNewFlushingBackend() {
	// The channel is buffered so we don't have to do async
	// receives:
	ch := make(chan []*ssf.SSFSpan, 1)
	be := testbackend.NewFlushingBackend(ch)
	client, _ := trace.NewBackendClient(be)

	for i := 0; i < 100; {
		// Report a metric and ensure it actually got sent:
		err := metrics.ReportOne(client, ssf.Count("hi_there", 1, map[string]string{}))
		if err == nil {
			i++
		}
	}

	// Call the backend's flush method to avoid the trace client's
	// debouncing / back-off behavior causing spurious errors in tests:
	be.Flush()

	rcvd := <-ch
	fmt.Println(rcvd[0].Metrics[0].Name)
	// Output:
	// hi_there
}
