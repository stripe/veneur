package grpsink

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/ssf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/keepalive"
)

var tags = map[string]string{"foo": "bar"}

type MockSpanSinkServer struct {
	spans []*ssf.SSFSpan
	mut   sync.Mutex
	got   chan struct{}
}

// SendSpans mocks base method
func (m *MockSpanSinkServer) SendSpans(stream SpanSink_SendSpansServer) error {
	for {
		span, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return stream.SendMsg(&SpanResponse{
					Greeting: "fin",
				})
			}
			return err
		}
		m.mut.Lock()
		m.spans = append(m.spans, span)
		m.mut.Unlock()
		m.got <- struct{}{}
	}
}

// Extra method and locking to avoid a weird data race
func (m *MockSpanSinkServer) firstSpan() *ssf.SSFSpan {
	m.mut.Lock()
	defer m.mut.Unlock()
	if len(m.spans) == 0 {
		panic("no spans yet")
	}

	return m.spans[0]
}

func (m *MockSpanSinkServer) spanCount() int {
	m.mut.Lock()
	defer m.mut.Unlock()
	return len(m.spans)
}

func TestEndToEnd(t *testing.T) {
	testaddr := "127.0.0.1:15111"
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}

	mock, srv := &MockSpanSinkServer{got: make(chan struct{})}, grpc.NewServer()
	RegisterSpanSinkServer(srv, mock)

	block := make(chan struct{})
	go func() {
		<-block
		srv.Serve(lis)
	}()
	block <- struct{}{}

	sink, err := NewGRPCStreamingSpanSink(context.Background(), testaddr, "test1", tags, logrus.New(), grpc.WithInsecure())
	require.NoError(t, err)
	assert.Equal(t, sink.commonTags, tags)
	assert.NotNil(t, sink.grpcConn)

	err = sink.Start(nil)
	require.NoError(t, err)

	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpan := &ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}

	err = sink.Ingest(testSpan)
	<-mock.got
	testSpan.Tags = map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.firstSpan())
	require.Equal(t, mock.spanCount(), 1)

	// Stoppage will happen in the background, and forward progress will be blocked by the wait sequence.
	go srv.Stop()
	waitThroughStateSequence(t, sink, 5*time.Second,
		connectivity.Ready,
		connectivity.TransientFailure,
	)

	err = sink.Ingest(testSpan)
	require.Equal(t, mock.spanCount(), 1)
	require.Error(t, err)

	// Set up new net listener and server; Stop() closes the listener we used before.
	srv = grpc.NewServer()
	RegisterSpanSinkServer(srv, mock)
	lis, err = net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}
	go func() {
		<-block
		err = srv.Serve(lis)
		assert.NoError(t, err)
	}()
	block <- struct{}{}
	reconnectWithin(t, sink, 2*time.Second)

	err = sink.Ingest(testSpan)
	require.NoError(t, err)
	<-mock.got
	require.Equal(t, mock.spanCount(), 2)
}

// It should be nearly unreachable for an idle state to be reached by the
// channel, but this test ensures we handle it properly in the event that it
// does. It can be flaky, though (it's too dependent on sleep timings), so
// it's disabled, but preserved for future debugging purposes.
func TestClientIdleRecovery(t *testing.T) {
	testaddr := "127.0.0.1:15112"
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}

	mock, srv := &MockSpanSinkServer{got: make(chan struct{})}, grpc.NewServer()
	RegisterSpanSinkServer(srv, mock)

	block := make(chan struct{})
	go func() {
		<-block
		srv.Serve(lis)
	}()
	block <- struct{}{}

	sink, err := NewGRPCStreamingSpanSink(context.Background(),
		testaddr, "test1", tags, logrus.New(),
		grpc.WithInsecure(),
		// Very short timeout in order to ensure the channel becomes idle
		// almost immediately.
		grpc.WithKeepaliveParams(keepalive.ClientParameters{Timeout: time.Millisecond}),
	)
	require.NoError(t, err)
	go func() {
		sink.maintainStream(context.Background())
	}()

	// Pre-mark the stream as in a bad state.
	atomic.StoreUint32(&sink.bad, 1)

	// SUT here is the channel and stream maintenance system, so express the
	// requirement as a series of states through which it should automatically
	// proceed.
	waitThroughFiniteStateSequence(t, sink, 5*time.Second,
		connectivity.Idle,
		connectivity.Connecting,
		connectivity.Ready,
	)

	reconnectWithin(t, sink, 5*time.Second)

	// Send a span, just to be sure.
	start := time.Now()
	end := start.Add(2 * time.Second)
	testSpan := &ssf.SSFSpan{
		TraceId:        1,
		ParentId:       1,
		Id:             2,
		StartTimestamp: int64(start.UnixNano()),
		EndTimestamp:   int64(end.UnixNano()),
		Error:          false,
		Service:        "farts-srv",
		Tags: map[string]string{
			"baz": "qux",
		},
		Indicator: false,
		Name:      "farting farty farts",
	}

	err = sink.Ingest(testSpan)
	<-mock.got
	testSpan.Tags = map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.firstSpan())
	require.Equal(t, mock.spanCount(), 1)
}
