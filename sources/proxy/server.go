// Package importsrv receives metrics over gRPC and sends them to workers.
//
// The Server wraps a grpc.Server, and implements the forwardrpc.Forward
// service.  It receives batches of metrics, then hashes them to a specific
// "MetricIngester" and forwards them on.
package proxy

import (
	"fmt"
	"io"
	"net"
	"time"

	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/segmentio/fasthash/fnv1a"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/sources"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
)

const (
	responseDurationMetric = "import.response_duration_ns"
)

// MetricIngester reads metrics from protobufs
type MetricIngester interface {
	IngestMetrics([]*metricpb.Metric)
}

// Server wraps a gRPC server and implements the forwardrpc.Forward service.
// It reads a list of metrics, and based on the provided key chooses a
// MetricIngester to send it to.  A unique metric (name, tags, and type)
// should always be routed to the same MetricIngester.
type Server struct {
	*grpc.Server
	address    string
	logger     *logrus.Entry
	metricOuts []MetricIngester
	opts       *options
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
func New(address string, metricOuts []MetricIngester, logger *logrus.Entry, opts ...Option) *Server {
	res := &Server{
		address:    address,
		logger:     logger,
		metricOuts: metricOuts,
		opts:       &options{},
		Server:     grpc.NewServer(),
	}

	for _, opt := range opts {
		opt(res.opts)
	}

	if res.opts.traceClient == nil {
		res.opts.traceClient = trace.DefaultClient
	}

	forwardrpc.RegisterForwardServer(res.Server, res)

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
	entry := logrus.WithFields(logrus.Fields{"address": s.address})
	entry.Info("Starting gRPC server")

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return fmt.Errorf("failed to bind the import server to '%s': %v",
			s.address, err)
	}

	err = s.Server.Serve(listener)
	if err != nil {
		entry.WithError(err).Error("gRPC server was not shut down cleanly")
	}
	entry.Info("Stopped gRPC server")
	return err
}

// Try to perform a graceful stop of the gRPC server.  If it takes more than
// 10 seconds, timeout and force-stop.
func (s *Server) Stop() {
	done := make(chan struct{})
	defer close(done)
	go func() {
		s.GracefulStop()
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		s.logger.Info(
			"Force-stopping the gRPC server after waiting for a graceful shutdown")
		s.Stop()
	}
}

// Static maps of tags used in the SendMetrics handler
var (
	grpcTags          = map[string]string{"protocol": "grpc"}
	responseGroupTags = map[string]string{
		"protocol": "grpc",
		"part":     "group",
	}
	responseSendTags = map[string]string{
		"protocol": "grpc",
		"part":     "send",
	}
)

// SendMetrics takes a list of metrics and hashes each one (based on the
// metric key) to a specific metric ingester.
func (s *Server) SendMetrics(ctx context.Context, mlist *forwardrpc.MetricList) (*empty.Empty, error) {
	span, _ := trace.StartSpanFromContext(ctx, "veneur.opentracing.importsrv.handle_send_metrics")
	span.SetTag("protocol", "grpc")
	defer span.ClientFinish(s.opts.traceClient)

	dests := make([][]*metricpb.Metric, len(s.metricOuts))

	// group metrics by their destination
	groupStart := time.Now()
	for _, m := range mlist.Metrics {
		workerIdx := s.hashMetric(m) % uint32(len(dests))
		dests[workerIdx] = append(dests[workerIdx], m)
	}
	span.Add(ssf.Timing(responseDurationMetric, time.Since(groupStart), time.Nanosecond, responseGroupTags))

	// send each set of metrics to its destination.  Since this is typically
	// implemented with channels, batching the metrics together avoids
	// repeated channel send operations
	sendStart := time.Now()
	for i, ms := range dests {
		if len(ms) > 0 {
			s.metricOuts[i].IngestMetrics(ms)
		}
	}

	span.Add(
		ssf.Timing(responseDurationMetric, time.Since(sendStart), time.Nanosecond, responseSendTags),
		ssf.Count("import.metrics_total", float32(len(mlist.Metrics)), grpcTags),
	)

	return &empty.Empty{}, nil
}

func (s *Server) SendMetricsV2(
	server forwardrpc.Forward_SendMetricsV2Server,
) error {
	metrics := []*metricpb.Metric{}
	for {
		metric, err := server.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		metrics = append(metrics, metric)
	}
	_, err := s.SendMetrics(context.Background(), &forwardrpc.MetricList{
		Metrics: metrics,
	})
	return err
}

// hashMetric returns a 32-bit hash from the input metric based on its name,
// type, and tags.
//
// The fnv1a package is used as opposed to fnv from the standard library, as
// it avoids allocations by not using the hash.Hash interface and by avoiding
// string to []byte conversions.
func (s *Server) hashMetric(m *metricpb.Metric) uint32 {
	h := fnv1a.HashString32(m.Name)
	h = fnv1a.AddString32(h, m.Type.String())
	for _, tag := range m.Tags {
		h = fnv1a.AddString32(h, tag)
	}
	return h
}
