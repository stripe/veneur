package grpsink

import (
	"context"
	"io"
	"net"
	"sync"
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

	mock, srv := &MockSpanSinkServer{}, grpc.NewServer()
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
	// This should be enough to make it through loopback TCP. Bump up if flaky.
	time.Sleep(time.Millisecond)
	testSpan.Tags = map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.firstSpan())
	require.Equal(t, mock.spanCount(), 1)

	srv.Stop()
	time.Sleep(5 * time.Millisecond)

	err = sink.Ingest(testSpan)
	require.Equal(t, mock.spanCount(), 1)
	assert.Error(t, err)

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

	ctx, cf := context.WithTimeout(context.Background(), 1*time.Second)
	if !sink.grpcConn.WaitForStateChange(ctx, connectivity.TransientFailure) {
		t.Fatal("Connection never transitioned from TransientFailure")
	}
	cf()
	ctx, cf = context.WithTimeout(context.Background(), 1*time.Second)
	if !sink.grpcConn.WaitForStateChange(ctx, connectivity.Connecting) {
		t.Fatal("Connection never transitioned from Connecting")
	}
	cf()

	require.Equal(t, connectivity.Ready, sink.grpcConn.GetState())
	time.Sleep(time.Millisecond)
	err = sink.Ingest(testSpan)
	assert.NoError(t, err)
	time.Sleep(time.Millisecond)
	require.Equal(t, mock.spanCount(), 2)
}

// It should be nearly unreachable for an idle state to be reached by the
// channel, but this test ensures we handle it properly in the event that it
// does.
func TestClientIdleRecovery(t *testing.T) {
	testaddr := "127.0.0.1:15112"
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}

	mock, srv := &MockSpanSinkServer{}, grpc.NewServer()
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
		<-block
		sink.maintainStream()
	}()
	block <- struct{}{}

	// SUT here is the channel and stream maintenance system, so express the
	// requirement as a series of states through which it should automatically
	// proceed.
	seq := []connectivity.State{
		connectivity.Connecting,
		connectivity.Ready,
		connectivity.Idle,
		connectivity.Connecting,
	}

	for i, state := range seq {
		ctx, cf := context.WithTimeout(context.Background(), 1*time.Second)
		if !sink.grpcConn.WaitForStateChange(ctx, state) {
			t.Fatalf("(seq %v) Connection never transitioned from %s", i, state)
		}
		cf()
	}
	require.Equal(t, connectivity.Ready, sink.grpcConn.GetState())

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
	// This should be enough to make it through loopback TCP. Bump up if flaky.
	time.Sleep(time.Millisecond)
	testSpan.Tags = map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.firstSpan())
	require.Equal(t, mock.spanCount(), 1)
}
