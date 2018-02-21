package grpsink

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/ssf"
	"google.golang.org/grpc"
)

const testaddr = "127.0.0.1:15111"

var tags = map[string]string{"foo": "bar"}

// MockSpanSinkServer is a mock of SpanSinkServer interface
type MockSpanSinkServer struct {
	spans []*ssf.SSFSpan
	lock  sync.Mutex
}

// SendSpans mocks base method
func (m *MockSpanSinkServer) SendSpans(stream SpanSink_SendSpansServer) error {
	for {
		span, err := stream.Recv()
		m.lock.Lock()
		if err != nil {
			if err == io.EOF {
				return stream.SendMsg(&SpanResponse{
					Greeting: "fin",
				})
			}
			return err
		}
		m.spans = append(m.spans, span)
		m.lock.Unlock()
	}
}

// Extra method and locking to avoid a weird data race
func (m *MockSpanSinkServer) getFirstSpan() *ssf.SSFSpan {
	m.lock.Lock()
	defer m.lock.Unlock()
	if len(m.spans) == 0 {
		panic("no spans yet")
	}

	return m.spans[0]
}

func TestEndToEnd(t *testing.T) {
	// Set up a server
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		fmt.Printf("Failed to set up net listener with err %s", err)
		os.Exit(1)
	}

	srv := grpc.NewServer()
	mock := &MockSpanSinkServer{}
	RegisterSpanSinkServer(srv, mock)
	block := make(chan struct{})
	go func() {
		close(block)
		srv.Serve(lis)
	}()
	<-block // Make sure the goroutine's started proceeding

	sink, err := NewGRPCStreamingSpanSink(context.Background(), testaddr, "test1", tags, logrus.New(), grpc.WithInsecure())
	assert.NoError(t, err)
	assert.Equal(t, sink.commonTags, tags)
	assert.NotNil(t, sink.grpcConn)

	err = sink.Start(nil)
	assert.NoError(t, err)

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
	time.Sleep(50 * time.Millisecond)
	testSpan.Tags = map[string]string{
		"foo": "bar",
		"baz": "qux",
	}

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.getFirstSpan())

	srv.Stop()
}
