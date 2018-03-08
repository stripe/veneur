package forwardtest

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/samplers/metricpb"
	"google.golang.org/grpc"
)

// SendMetricHandler is a handler that is called when a Server gets a
// SendMetrics RPC
type SendMetricHandler func([]*metricpb.Metric)

// Server is a gRPC server similar to httptest.Server
type Server struct {
	*grpc.Server
	lis      net.Listener
	handler  SendMetricHandler
	startMtx sync.Mutex
}

// NewServer creates an unstarted Server with the specified handler
func NewServer(handler SendMetricHandler) *Server {
	res := &Server{
		Server:  grpc.NewServer(),
		handler: handler,
	}

	forwardrpc.RegisterForwardServer(res.Server, res)
	return res
}

// Start starts the gRPC server listening on the loopback interface on a
// random port.  The address it is listening on can be retrieved from
// (*Server).Addr()
func (s *Server) Start(t *testing.T) {
	s.startMtx.Lock()
	defer s.startMtx.Unlock()

	var err error
	s.lis, err = net.Listen("tcp", "127.0.0.1:")
	if err != nil {
		t.Fatalf("failed to create a TCP connection for a test GRPC "+
			"server: %v", err)
	}

	go func() {
		if err := s.Serve(s.lis); err != nil {
			t.Logf("failed to stop the test forwarding gRPC server: %v", err)
		}
	}()
}

// Addr returns the address the server is listening on.
func (s *Server) Addr() net.Addr {
	s.startMtx.Lock()
	defer s.startMtx.Unlock()

	return s.lis.Addr()
}

// SendMetrics calls the input SendMetricsHandler whenever it receives an
// RPC
func (s *Server) SendMetrics(ctx context.Context, mlist *forwardrpc.MetricList) (*empty.Empty, error) {
	s.handler(mlist.Metrics)
	return &empty.Empty{}, nil
}
