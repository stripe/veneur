package grpsink

import (
	"context"
	"errors"
	"sync"

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
	target      string
	grpcConn    *grpc.ClientConn
	sinkClient  SpanSinkClient
	stream      SpanSink_SendSpansClient
	streamMut   sync.Mutex
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
		target:     target,
		sinkClient: NewSpanSinkClient(conn),
		commonTags: commonTags,
		log:        log,
	}, nil
}

func (gs *GRPCStreamingSpanSink) Start(cl *trace.Client) error {
	gs.traceClient = cl

	// TODO this API is experimental - perhaps we shouldn't use it yet
	if !gs.grpcConn.WaitForStateChange(context.TODO(), connectivity.Ready) {
		err := errors.New("gRPC connection failed to become ready")
		gs.log.WithError(err).WithFields(logrus.Fields{
			"name":          gs.name,
			"target":        gs.target,
			"current-state": gs.grpcConn.GetState().String(),
		}).Error("Error establishing connection to gRPC server")
		return err
	}

	// TODO How do we allow grpc.CallOption to be injected in here?
	return gs.establishStream(context.TODO())
}

// Name returns this sink's name.
func (gs *GRPCStreamingSpanSink) Name() string {
	return gs.name
}

// Ensure a stream over the existing connection to the target server is
// established.
//
// Calls to this method must be guarded by the streamMut mutex.
func (gs *GRPCStreamingSpanSink) establishStream(ctx context.Context) error {
	if gs.stream != nil {
		return nil
	}

	stream, err := gs.sinkClient.SendSpans(ctx, grpc.FailFast(false))
	if err != nil {
		gs.log.WithFields(logrus.Fields{
			"name":          gs.name,
			"target":        gs.target,
			"current-state": gs.grpcConn.GetState().String(),
			logrus.ErrorKey: err,
		}).Error("Failed to set up a gRPC stream over connection to sink target")
	} else {
		gs.stream = stream
	}

	return err
}

// Ingest takes in a span and streams it over gRPC to the connected server.
func (gs *GRPCStreamingSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	// Apply any common tags, superseding defaults passed in at sink creation
	// in the event of overlap.
	for k, v := range gs.commonTags {
		ssfSpan.Tags[k] = v
	}

	// TODO validation of span, e.g. time bounds like in datadog span sink?
	// gRPC indicates that it is safe to send and receive on a stream at the same
	// time, but it is not safe to simultaneously send. Guard against that with a
	// mutex.
	gs.streamMut.Lock()
	if err := gs.establishStream(context.TODO()); err != nil {
		gs.streamMut.Unlock()
		return err
	}
	err := gs.stream.Send(ssfSpan)
	if err != nil {
		// An error means the stream is dead. Nil out the var; the next Ingest() call
		// will automatically attempt to create a new stream.
		gs.stream.CloseSend()
		gs.stream = nil
	}
	gs.streamMut.Unlock()

	if err != nil {
		gs.log.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"target":        gs.target,
			"name":          gs.name,
		}).Error("Error streaming traces to gRPC server")
	}
	return err
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (gs *GRPCStreamingSpanSink) Flush() {
	// TODO for now, do nothing
	return
}
