package trace

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
)

func benchmarkPlainCombination(backend ClientBackend, span *ssf.SSFSpan) func(*testing.B) {
	ctx := context.TODO()
	return func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			assert.NoError(b, backend.SendSync(ctx, span))
		}
	}
}

func benchmarkFlushingCombination(backend FlushableClientBackend, span *ssf.SSFSpan, every time.Duration) func(*testing.B) {
	return func(b *testing.B) {
		ctx := context.TODO()
		tick := time.NewTicker(every)
		defer tick.Stop()
		for i := 0; i < b.N; i++ {
			select {
			case <-tick.C:
				assert.NoError(b, backend.FlushSync(ctx))
			default:
				assert.NoError(b, backend.SendSync(ctx, span))
			}
		}
		assert.NoError(b, backend.FlushSync(ctx))
	}
}

// BenchmarkSerialization tests how long the serialization of a span
// over each kind of network link can take: UNIX with no buffer, UNIX
// with a buffer, UDP (only unbuffered); combined with the kinds of
// spans we send: spans with metrics attached, spans with no metrics
// attached, and empty spans with only metrics.
//
// The counterpart is either a fresh UDP port with nothing reading
// packets, or a network reader that discards every byte read.
func BenchmarkSerialization(b *testing.B) {
	dir, err := ioutil.TempDir("", "test_unix")
	require.NoError(b, err)
	defer os.RemoveAll(dir)

	sockName := filepath.Join(dir, "sock")
	laddr, err := net.ResolveUnixAddr("unix", sockName)
	require.NoError(b, err)
	cleanup := serveUNIX(b, laddr, func(conn net.Conn) {
		io.Copy(ioutil.Discard, conn)
	})
	defer cleanup()

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(b, err)
	defer udpConn.Close()

	unixBackend := &streamBackend{
		backendParams: backendParams{
			addr: laddr,
		},
	}
	flushyUnixBackend := &streamBackend{
		backendParams: backendParams{
			addr:       laddr,
			bufferSize: uint(BufferSize),
		},
	}
	udpBackend := &packetBackend{
		backendParams: backendParams{
			addr: udpConn.LocalAddr(),
		},
	}

	spanWithMetrics := &ssf.SSFSpan{
		Name:     "realistic_span",
		Service:  "hi-there-srv",
		Id:       1,
		ParentId: 2,
		TraceId:  3,
		Error:    false,
		Tags: map[string]string{
			"span_purpose": "testing",
		},
		Metrics: []*ssf.SSFSample{
			ssf.Count("oh.hai", 1, map[string]string{"purpose": "testing"}),
			ssf.Histogram("hello.there", 1, map[string]string{"purpose": "testing"}, ssf.Unit("absolute")),
		},
	}

	spanNoMetrics := &ssf.SSFSpan{
		Name:     "realistic_span",
		Service:  "hi-there-srv",
		Id:       1,
		ParentId: 2,
		TraceId:  3,
		Error:    false,
		Tags: map[string]string{
			"span_purpose": "testing",
		},
	}

	emptySpanWithMetrics := &ssf.SSFSpan{
		Metrics: []*ssf.SSFSample{
			ssf.Count("oh.hai", 1, map[string]string{"purpose": "testing"}),
			ssf.Histogram("hello.there", 1, map[string]string{"purpose": "testing"}, ssf.Unit("lad")),
		},
	}

	// Warm up things:
	connect(context.TODO(), unixBackend)
	_ = flushyUnixBackend.FlushSync(context.TODO())
	connect(context.TODO(), udpBackend)
	b.ResetTimer()

	// Start benchmarking:
	for _, every := range []int{10, 50, 100} {
		b.Run(fmt.Sprintf("UNIX_flush_span_with_metrics_%dms", every),
			benchmarkFlushingCombination(flushyUnixBackend, spanWithMetrics, time.Duration(every)*time.Millisecond))
		b.Run(fmt.Sprintf("UNIX_flush_span_no_metrics_%dms", every),
			benchmarkFlushingCombination(flushyUnixBackend, spanNoMetrics, time.Duration(every)*time.Millisecond))
		b.Run(fmt.Sprintf("UNIX_flush_empty_span_with_metrics_%dms", every),
			benchmarkFlushingCombination(flushyUnixBackend, emptySpanWithMetrics, time.Duration(every)*time.Millisecond))
	}

	b.Run("UNIX_plain_span_with_metrics", benchmarkPlainCombination(unixBackend, spanWithMetrics))
	b.Run("UNIX_plain_span_no_metrics", benchmarkPlainCombination(unixBackend, spanNoMetrics))
	b.Run("UNIX_plain_empty_span_with_metrics", benchmarkPlainCombination(unixBackend, emptySpanWithMetrics))

	b.Run("UDP_plain_span_with_metrics", benchmarkPlainCombination(udpBackend, spanWithMetrics))
	b.Run("UDP_plain_span_no_metrics", benchmarkPlainCombination(udpBackend, spanNoMetrics))
	b.Run("UDP_plain_empty_span_with_metrics", benchmarkPlainCombination(udpBackend, emptySpanWithMetrics))
}
