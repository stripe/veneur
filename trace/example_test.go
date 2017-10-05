package trace_test

import (
	"github.com/stripe/veneur/trace"
)

// This demonstrates how to switch out an existing
// DefaultClient, closing the existing connection correctly.
func ExampleConnect() {
	// Create the new client first (we'll create one that can send
	// 20 span packets in parallel):
	cl, err := trace.NewClient(trace.DefaultVeneurAddress, trace.Capacity(20))
	if err != nil {
		panic(err)
	}

	// Replace the old client:
	oldCl := trace.DefaultClient

	// Now close the old default client so we don't leak connections
	trace.DefaultClient = cl
	oldCl.Close()
	// Output:
}
