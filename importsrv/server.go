// Package importsrv receives metrics over gRPC and sends them to workers.
//
// The Server wraps a grpc.Server, and implements the forwardrpc.Forward
// service.  It receives batches of metrics, then hashes them to a specific
// "MetricIngester" and forwards them on.
package importsrv

import (
	"fmt"
	"net"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/segmentio/fasthash/fnv1a"
	"golang.org/x/net/context" // This can be replace with "context" after Go 1.8 support is dropped
	"google.golang.org/grpc"

	"github.com/stripe/veneur/forwardrpc"
	"github.com/stripe/veneur/samplers/metricpb"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
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
	metricOuts []MetricIngester
	opts       *options
}

type options struct {
	traceClient *trace.Client
}

// Option is returned by functions that serve as options to New, like
// "With..."
type Option func(*options)

// New creates an unstarted Server with the input MetricIngester's to send
// output to.
func New(metricOuts []MetricIngester, opts ...Option) *Server {
	res := &Server{
		Server:     grpc.NewServer(grpc.RPCCompressor(grpc.NewGZIPCompressor()), grpc.RPCDecompressor(grpc.NewGZIPDecompressor())),
		metricOuts: metricOuts,
		opts:       &options{},
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

// Serve starts a gRPC listener on the specified address and blocks while
// listening for requests. If listening is interrupted by some means other
// than Stop or GracefulStop being called, it returns a non-nil error.
func (s *Server) Serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to bind the import server to '%s': %v",
			addr, err)
	}

	return s.Server.Serve(ln)
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
