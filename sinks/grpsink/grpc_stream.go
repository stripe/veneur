package grpsink

import (
	"context"
	"sync"
	"time"

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

	// We want our stream in fail-fast mode (the default, but good to be explicit)
	// because otherwise Ingest() calls will block while connections get re-established
	// in the background, resulting in undesirable backpressure; it's preferable to
	// just drop the spans instead.
	var err error
	gs.stream, err = gs.sinkClient.SendSpans(context.TODO(), grpc.FailFast(true))
	if err != nil {
		gs.log.WithFields(logrus.Fields{
			"name":          gs.name,
			"target":        gs.target,
			"gchan-state":   gs.grpcConn.GetState().String(),
			logrus.ErrorKey: err,
		}).Error("Failed to set up a stream over gRPC channel to sink target")
		return err
	}

	// Run a goroutine in the background that reacts to state changes in the underlying
	// connection by automatically attempting to re-establish the stream connection. See
	// https://github.com/grpc/grpc/blob/master/doc/connectivity-semantics-and-api.mdA
	// for information about gRPC's connectivity state machine.
	go func() {
		var state, laststate connectivity.State
		for {
			// This call will block on a channel receive until the gRPC connection
			// becomes unhealthy. Then, spring into action!
			state = gs.grpcConn.GetState()
			switch state {
			case connectivity.Ready, connectivity.Idle:
				if laststate != connectivity.Ready && laststate != connectivity.Idle {
					gs.streamMut.Lock()
					stream, err := gs.sinkClient.SendSpans(context.TODO(), grpc.FailFast(true))
					if err != nil {
						// This is a weird case and probably shouldn't be reachable, but if
						// stream setup fails after recovery, then just wait for a second
						// and try again.
						gs.log.WithFields(logrus.Fields{
							"name":          gs.name,
							"target":        gs.target,
							"gchan-state":   gs.grpcConn.GetState().String(),
							logrus.ErrorKey: err,
						}).Error("Failed to set up a new stream after gRPC channel recovered")
						time.Sleep(1 * time.Second)
						gs.streamMut.Unlock()
						continue
					}
					gs.stream = stream
					gs.streamMut.Unlock()
				}
			case connectivity.Connecting:
				// Nothing to do here except wait.
			case connectivity.TransientFailure:
				gs.log.WithFields(logrus.Fields{
					"name":          gs.name,
					"target":        gs.target,
					"gchan-state":   state,
					logrus.ErrorKey: err,
				}).Warn("gRPC connection moved into transient failure mode")
			case connectivity.Shutdown:
				// The current design has no path to actually shutting down the
				// connection, so this should be unreachable.
				return
			}

			laststate = state
			gs.grpcConn.WaitForStateChange(context.Background(), state)
		}
	}()

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
	err := gs.stream.Send(ssfSpan)
	gs.streamMut.Unlock()

	if err != nil {
		gs.log.WithFields(logrus.Fields{
			logrus.ErrorKey: err,
			"target":        gs.target,
			"name":          gs.name,
			"gchan-state":   gs.grpcConn.GetState().String(),
		}).Error("Error streaming trace to gRPC server")
	}
	return err
}

// Flush doesn't need to do anything to the LS tracer, so we emit metrics
// instead.
func (gs *GRPCStreamingSpanSink) Flush() {
	// TODO for now, do nothing
	return
}
