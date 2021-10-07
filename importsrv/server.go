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

	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/segmentio/fasthash/fnv1a"
	"google.golang.org/grpc"

	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/routing"
	"github.com/stripe/veneur/v14/samplers/metricpb"
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

type IngesterSet struct {
	Ingesters                []MetricIngester
	ComputationRoutingConfig routing.ComputationRoutingConfig
}

// Server wraps a gRPC server and implements the forwardrpc.Forward service.
// It reads a list of metrics, and based on the provided key chooses a
// MetricIngester to send it to.  A unique metric (name, tags, and type)
// should always be routed to the same MetricIngester.
type Server struct {
	*grpc.Server
	ingesterSets []IngesterSet
	opts         *options
}

type options struct {
	traceClient *trace.Client
}

// Option is returned by functions that serve as options to New, like
// "With..."
type Option func(*options)

// New creates an unstarted Server with the input MetricIngester's to send
// output to.
func New(ingesterSets []IngesterSet, opts ...Option) *Server {
	res := &Server{
		Server:       grpc.NewServer(),
		ingesterSets: ingesterSets,
		opts:         &options{},
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

	for _, ingesterSet := range s.ingesterSets {
		dests := make([][]*metricpb.Metric, len(ingesterSet.Ingesters))

		// group metrics by their destination
		groupStart := time.Now()
		for _, m := range mlist.Metrics {
			workerIdx := s.hashMetric(m) % uint32(len(dests))
			dests[workerIdx] = append(dests[workerIdx], m)
		}
		span.Add(ssf.Timing(responseDurationMetric, time.Since(groupStart), time.Nanosecond, s.withComputationRoutingTags(responseGroupTags, ingesterSet)))

		// send each set of metrics to its destination.  Since this is typically
		// implemented with channels, batching the metrics together avoids
		// repeated channel send operations
		sendStart := time.Now()
		for i, ms := range dests {
			filteredMs := make([]*metricpb.Metric, 0, len(ms))
			for _, m := range ms {
				if ingesterSet.ComputationRoutingConfig.MatcherConfigs.Match(m.GetName(), m.GetTags()) {
					filteredMs = append(filteredMs, m)
				}
			}
			if len(filteredMs) > 0 {
				ingesterSet.Ingesters[i].IngestMetrics(filteredMs)
			}
		}

		span.Add(
			ssf.Timing(responseDurationMetric, time.Since(sendStart), time.Nanosecond, s.withComputationRoutingTags(responseSendTags, ingesterSet)),
			ssf.Count("import.metrics_total", float32(len(mlist.Metrics)), s.withComputationRoutingTags(grpcTags, ingesterSet)),
		)
	}

	return &empty.Empty{}, nil
}

func (s *Server) withComputationRoutingTags(tags map[string]string, ingesterSet IngesterSet) map[string]string {
	ret := make(map[string]string, len(tags)+1)
	for k, v := range tags {
		ret[k] = v
	}
	ret["computation_routing_name"] = ingesterSet.ComputationRoutingConfig.Name
	return ret
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
