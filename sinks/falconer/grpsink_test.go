package falconer

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stripe/veneur/v14"

	ocontext "context"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/ssf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

type MockSpanSinkServer struct {
	spans []*ssf.SSFSpan
	mut   sync.Mutex
}

// SendSpans mocks base method
func (m *MockSpanSinkServer) SendSpan(
	ctx ocontext.Context, span *ssf.SSFSpan,
) (*Empty, error) {
	m.mut.Lock()
	m.spans = append(m.spans, span)
	m.mut.Unlock()
	return &Empty{}, nil
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

func testLogger() *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	return logrus.NewEntry(logger)
}

func TestEndToEnd(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)

	testaddr := "127.0.0.1:0"
	lis, err := net.Listen("tcp", testaddr)
	if err != nil {
		t.Fatalf("Failed to set up net listener with err %s", err)
	}
	testaddr = lis.Addr().String()

	mock, srv := &MockSpanSinkServer{}, grpc.NewServer()
	RegisterSpanSinkServer(srv, mock)

	block := make(chan struct{})
	go func() {
		<-block
		srv.Serve(lis)
	}()
	block <- struct{}{}

	sink, err := Create(
		&veneur.Server{},
		"grpc",
		testLogger(),
		veneur.Config{},
		FalconerSpanSinkConfig{
			Target: testaddr,
		},
	)
	require.NoError(t, err)

	grpcSink, ok := sink.(*FalconerSpanSink)
	assert.NotNil(t, grpcSink.grpcConn)
	assert.True(t, ok)

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

	assert.NoError(t, err)
	assert.Equal(t, testSpan, mock.firstSpan())
	require.Equal(t, mock.spanCount(), 1)

	// Stoppage will happen in the background, and forward progress will be
	// blocked by the wait sequence.
	go srv.Stop()
	waitThroughStateSequence(t, grpcSink, 5*time.Second,
		connectivity.Ready,
		connectivity.TransientFailure,
	)

	err = sink.Ingest(testSpan)
	require.Equal(t, mock.spanCount(), 1)
	require.Error(t, err)

	// Set up new net listener and server; Stop() closes the listener we used
	// before.
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
	reconnectWithin(t, grpcSink, 2*time.Second)

	err = sink.Ingest(testSpan)
	require.NoError(t, err)
	require.Equal(t, mock.spanCount(), 2)
}
