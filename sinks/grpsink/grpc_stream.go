package grpsink

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// GRPCStreamingSpanSink is a sink that streams spans to a configurable target
// service over gRPC. The sink is only tied to the grpc_sink.proto definition of
// a SpanSink service, and thus is generic with respect to the specific server
// it is connecting to.
type GRPCStreamingSpanSink struct {
	name        string
	grpcConn    *grpc.ClientConn
	sinkClient  SpanSinkClient
	stream      SpanSink_SendSpansClient
	stats       *statsd.Client
	commonTags  map[string]string
	traceClient *trace.Client
	log         *logrus.Logger
}

var _ sinks.SpanSink = &GRPCStreamingSpanSink{}

// NewGRPCStreamingSpanSink creates a sinks.SpanSink that can write to any
// compliant gRPC server.
//
// The target parameter should be of the "host:port"; the name parameter is
// prepended with "grpc-", and is used when reporting logs in order to permit
// differentiation between various services.
//
// Any grpc.CallOpts that are provided will be used while first establishing the
// connection to the target server (in grpc.DialContext()).
func NewGRPCStreamingSpanSink(ctx context.Context, target, name string, commonTags map[string]string, log *logrus.Logger, opts ...grpc.DialOption) (*GRPCStreamingSpanSink, error) {
	name = "grpc-" + name
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"name":   name,
			"target": target,
		}).Error("Error establishing connection to gRPC server")
		return nil, err
	}

	return &GRPCStreamingSpanSink{
		grpcConn:   conn,
		name:       name,
		sinkClient: NewSpanSinkClient(conn),
		commonTags: commonTags,
		log:        log,
	}, nil
}

func (gs *GRPCStreamingSpanSink) Start(cl *trace.Client) error {
	// TODO this API is experimental - perhaps we shouldn't use it yet
	if !gs.grpcConn.WaitForStateChange(context.TODO(), connectivity.Ready) {
		err := errors.New("gRPC connection failed to become ready")
		gs.log.WithError(err).WithFields(logrus.Fields{
			"name":          gs.name,
			"current-state": gs.grpcConn.GetState().String(),
		}).Error("Error establishing connection to gRPC server")
		return err
	}

	// TODO How do we allow grpc.CallOption to be injected in here?
	// TODO Setting this up here (probably) means one stream per lifecycle of this object. Is that the right design?
	stream, err := gs.sinkClient.SendSpans(context.TODO())
	if err != nil {
		gs.log.WithFields(logrus.Fields{
			"name":          gs.name,
			logrus.ErrorKey: err,
		}).Error("Failed to set up a gRPC stream to sink target")
		return err
	}

	gs.stream, gs.traceClient = stream, cl
	return nil
}

// Name returns this sink's name.
func (gs *GRPCStreamingSpanSink) Name() string {
	return gs.name
}

// Ingest takes in a span and streams it over gRPC to the connected server.
func (gs *GRPCStreamingSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}
	// TODO validate gRPC connection state? Either that has to happen here, or we rely on an err from the Send() call below

	// Apply any common tags, superseding
	for k, v := range gs.commonTags {
		ssfSpan.Tags[k] = v
	}

	// TODO validation of span, e.g. time bounds like in datadog span sink?
	err := gs.stream.Send(ssfSpan)
	if err != nil {
		gs.log.WithError(err).WithFields(logrus.Fields{
			"name": gs.name,
		}).Warn("Error streaming traces to gRPC server")
		return err
	}

	return nil
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (gs *GRPCStreamingSpanSink) Flush() {
	// TODO for now, do nothing
	return
}
