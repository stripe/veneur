//go:build go1.9
// +build go1.9

package falconer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/connectivity"
)

func waitThroughStateSequence(
	t *testing.T, sink *FalconerSpanSink, dur time.Duration,
	seq ...connectivity.State,
) {
	t.Helper()

	first, current := seq[0], sink.grpcConn.GetState()
	// ObjectsAreEqual instead of assert/require.Equal because those funcs aren't
	// smart about t.Helper() yet.
	if !assert.ObjectsAreEqual(first, current) {
		t.Fatalf(
			"Wanted %q for initial connection state, but got %q", first, current)
	}

	for i, state := range seq[:] {
		ctx, cf := context.WithTimeout(context.Background(), dur)
		if !sink.grpcConn.WaitForStateChange(ctx, state) {
			t.Fatalf(
				"(seq %v) Connection never transitioned from %q", i, state.String())
		}
		cf()
	}
}

func reconnectWithin(t *testing.T, sink *FalconerSpanSink, dur time.Duration) {
	t.Helper()
	ctx, cf := context.WithTimeout(context.Background(), dur)
	for {
		state := sink.grpcConn.GetState()
		switch state {
		case connectivity.Ready:
			cf()
			return
		default:
			if !sink.grpcConn.WaitForStateChange(ctx, state) {
				t.Fatal(
					"Connection did not move back to READY state within alloted time")
			}
		}
	}
}
