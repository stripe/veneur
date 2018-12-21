// Package importsrv receives metrics over gRPC and sends them to workers.
//
// The Server wraps a grpc.Server, and implements the forwardrpc.Forward
// service.  It receives batches of metrics, then hashes them to a specific
// "MetricIngester" and forwards them on.
package importsrv

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/stripe/veneur/metricingester"

	"github.com/golang/protobuf/ptypes/empty"
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

// metricsIngester is the contract expected of objects to which metrics will be submitted.
type metricsIngester interface {
	Ingest(context.Context, metricingester.Metric) error
	Merge(context.Context, metricingester.Digest) error
}

// Server wraps a gRPC server and implements the forwardrpc.Forward service.
// It reads a list of metrics, and based on the provided key chooses a
// MetricIngester to send it to.  A unique metric (name, tags, and type)
// should always be routed to the same MetricIngester.
type Server struct {
	*grpc.Server
	ingester metricsIngester
	opts     *options
}

type options struct {
	traceClient *trace.Client
}

// Option is returned by functions that serve as options to New, like
// "With..."
type Option func(*options)

// New creates an unstarted Server with the input MetricIngester's to send
// output to.
func New(ingester metricsIngester, opts ...Option) *Server {
	res := &Server{
		Server:   grpc.NewServer(),
		ingester: ingester,
		opts:     &options{},
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
	grpcTags         = map[string]string{"protocol": "grpc"}
	responseSendTags = map[string]string{
		"protocol": "grpc",
		"part":     "send",
	}
)

// SendMetrics accepts a batch of metrics for importing.
func (s *Server) SendMetrics(ctx context.Context, mlist *forwardrpc.MetricList) (*empty.Empty, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "veneur.opentracing.importsrv.handle_send_metrics")
	span.SetTag("protocol", "grpc")
	defer span.ClientFinish(s.opts.traceClient)

	sendStart := time.Now()
	for _, m := range mlist.Metrics {
		hostname := m.GetHostname()
		tags := m.GetTags()
		switch v := m.GetValue().(type) {
		case *metricpb.Metric_Gauge:
			s.ingester.Ingest(ctx, metricingester.NewGauge(m.Name, v.Gauge.GetValue(), tags, 1.0, hostname))
		case *metricpb.Metric_Counter:
			s.ingester.Ingest(ctx, metricingester.NewCounter(m.Name, v.Counter.GetValue(), tags, 1.0, hostname))
		case *metricpb.Metric_Set:
			s.ingester.Merge(ctx, metricingester.NewSetDigest(m.Name, v.Set, tags, hostname))
		case *metricpb.Metric_Histogram:
			// Scope is a legacy concept used to designate whether a metric needed to be emitted locally
			// or aggregated globally.
			//
			// The presence of a hostname now encodes the same concept.
			//
			// However, histograms have a special "MixedScope" that emits percentiles globally and
			// min/max/count values locally.
			//
			// We need a special metric type to represent this: the MixedHistogram.
			//
			// We also need to distinguish between the behavior before all metrics were forwarded
			// to the behavior after. The before case is represented by Scope_Mixed, which makes that host's
			// "min/max/count" not get emitted by the central veneur, since the local veneur is emitting it
			// still. Scoped_MixedGlobal makes the central veneur emit the min/max/count.
			//
			// TODO(clin): After we completely migrate to the new veneur, delete support for this latter distinction!
			switch m.GetScope() {
			case metricpb.Scope_Mixed:
				s.ingester.Merge(
					ctx,
					metricingester.NewMixedHistogramDigest(m.Name, v.Histogram, tags, hostname, false),
				)
			case metricpb.Scope_MixedGlobal:
				s.ingester.Merge(
					ctx,
					metricingester.NewMixedHistogramDigest(m.Name, v.Histogram, tags, hostname, true),
				)
			case metricpb.Scope_Local, metricpb.Scope_Global:
				s.ingester.Merge(
					ctx,
					metricingester.NewHistogramDigest(m.Name, v.Histogram, tags, hostname),
				)
			}
		case nil:
			return nil, errors.New("can't import a metric with a nil value")
		}
	}

	span.Add(
		ssf.Timing(responseDurationMetric, time.Since(sendStart), time.Nanosecond, responseSendTags),
		ssf.Count("import.metrics_total", float32(len(mlist.Metrics)), grpcTags),
	)

	return &empty.Empty{}, nil
}
