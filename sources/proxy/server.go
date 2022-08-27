// Package importsrv receives metrics over gRPC and sends them to workers.
//
// The Server wraps a grpc.Server, and implements the forwardrpc.Forward
// service.  It receives batches of metrics, then hashes them to a specific
// "MetricIngester" and forwards them on.
package proxy

import (
	"errors"
	"io"
	"net"
	"time"

	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/sources"
	"github.com/stripe/veneur/v14/trace"
)

// Server wraps a gRPC server and implements the forwardrpc.Forward service.
// It reads a list of metrics, and based on the provided key chooses a
// MetricIngester to send it to.  A unique metric (name, tags, and type)
// should always be routed to the same MetricIngester.
type Server struct {
	server       *grpc.Server
	address      string
	ingest       sources.Ingest
	listener     net.Listener
	logger       *logrus.Entry
	opts         *options
	readyChannel chan struct{}
}

var _ sources.Source = &Server{}

type options struct {
	traceClient *trace.Client
}

// Option is returned by functions that serve as options to New, like
// "With..."
type Option func(*options)

// New creates an unstarted Server with the input MetricIngester's to send
// output to.
func New(address string, logger *logrus.Entry, opts ...Option) *Server {
	res := &Server{
		address:      address,
		logger:       logger,
		opts:         &options{},
		server:       grpc.NewServer(),
		readyChannel: make(chan struct{}),
	}

	for _, opt := range opts {
		opt(res.opts)
	}

	if res.opts.traceClient == nil {
		res.opts.traceClient = trace.DefaultClient
	}

	forwardrpc.RegisterForwardServer(res.server, res)

	return res
}

func (s *Server) Name() string {
	return "proxy"
}

// Start starts a gRPC listener on the specified address and blocks while
// listening for requests. If listening is interrupted by some means other
// than Stop or GracefulStop being called, it returns a non-nil error.
//
// TODO this doesn't handle SIGUSR2 and SIGHUP on it's own, unlike HTTPServe
// As long as both are running this is actually fine, as Start will stop
// the gRPC server when the HTTP one exits.  When running just gRPC however,
// the signal handling won't work.
func (s *Server) Start(ingest sources.Ingest) error {
	s.ingest = ingest

	var err error
	s.listener, err = net.Listen("tcp", s.address)
	if err != nil {
		s.logger.WithError(err).WithField("address", s.address).
			Errorf("failed to bind import server")
		return err
	}

	logger := s.logger.WithFields(logrus.Fields{"address": s.listener.Addr()})
	logger.Info("Starting gRPC server")

	close(s.readyChannel)
	err = s.server.Serve(s.listener)
	if err != nil {
		logger.WithError(err).Error("gRPC server was not shut down cleanly")
	}
	logger.Info("Stopped gRPC server")
	return err
}

func (s *Server) GetAddress() string {
	return s.listener.Addr().String()
}

func (s *Server) Ready() <-chan struct{} {
	return s.readyChannel
}

// Try to perform a graceful stop of the gRPC server.  If it takes more than
// 10 seconds, timeout and force-stop.
func (s *Server) Stop() {
	done := make(chan struct{})
	defer close(done)
	go func() {
		s.server.GracefulStop()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.logger.Info(
			"Force-stopping the gRPC server after waiting for a graceful shutdown")
		s.server.Stop()
	}
}

// SendMetrics takes a list of metrics and hashes each one (based on the
// metric key) to a specific metric ingester.
func (s *Server) SendMetrics(
	_ context.Context, _ *forwardrpc.MetricList,
) (*empty.Empty, error) {
	return nil, errors.New("not implemented")
}

func (s *Server) SendMetricsV2(
	server forwardrpc.Forward_SendMetricsV2Server,
) error {
	for {
		metric, err := server.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			s.logger.WithError(err).Error("error recieving metrics")
			return err
		}
		s.ingest.IngestMetricProto(metric)
	}
	err := server.SendAndClose(&emptypb.Empty{})
	if err != nil {
		s.logger.WithError(err).Error("error closing stream")
	}
	return err
}
