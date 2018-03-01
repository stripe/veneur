// +build !go1.9

package grpsink

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/connectivity"
)

func waitThroughStateSequence(t *testing.T, sink *GRPCStreamingSpanSink, dur time.Duration, seq ...connectivity.State) {
	first, current := seq[0], sink.grpcConn.GetState()
	require.Equal(t, first, current, "Wanted %q for initial connection state, but got %q", first, current)

	for i, state := range seq[:len(seq)] {
		ctx, cf := context.WithTimeout(context.Background(), dur)
		if !sink.grpcConn.WaitForStateChange(ctx, state) {
			t.Fatalf("(seq %v) Connection never transitioned from %q", i, state)
		}
		cf()
	}
}

func waitThroughFiniteStateSequence(t *testing.T, sink *GRPCStreamingSpanSink, dur time.Duration, seq ...connectivity.State) {
	waitThroughStateSequence(t, sink, dur, seq[:len(seq)-1]...)

	last, current := seq[len(seq)-1], sink.grpcConn.GetState()
	require.Equal(t, last, current, "Wanted %q for final connection state, but got %q", last, current)
}

func reconnectWithin(t *testing.T, sink *GRPCStreamingSpanSink, dur time.Duration) {
	ctx, cf := context.WithTimeout(context.Background(), dur)
	for {
		state := sink.grpcConn.GetState()
		switch state {
		case connectivity.Ready:
			// Spin on the internal state marker that indicates the stream state is bad
			for !atomic.CompareAndSwapUint32(&sink.bad, 0, 0) {
				time.Sleep(time.Millisecond)
			}
			cf()
			return
		default:
			if !sink.grpcConn.WaitForStateChange(ctx, state) {
				t.Fatal("Connection did not move back to READY state within alloted time")
			}
		}
	}
}
