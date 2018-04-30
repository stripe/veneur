package grpsink

import (
	"context"
	"sync/atomic"

	ocontext "golang.org/x/net/context"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/protocol"
	"github.com/stripe/veneur/sinks"
	"github.com/stripe/veneur/ssf"
	"github.com/stripe/veneur/trace"
	"github.com/stripe/veneur/trace/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/status"
)

// GRPCSpanSink is a generic sink that sends spans to a configurable target
// service over gRPC. The sink is only tied to the grpc_sink.proto definition of
// a SpanSink service, and thus is agnostic with respect to the specific server
// it is connecting to.
type GRPCSpanSink struct {
	name   string
	target string
	// The underlying gRPC connection ("channel") to the target server.
	grpcConn *grpc.ClientConn
	// Total counts of sent and dropped spans, respectively
	sentCount, dropCount uint32
	// Marker to indicate if an error has been logged since the last state
	// transition. Allows us to guarantee only one error message per state
	// change.
	loggedSinceTransition uint32
	stats                 *statsd.Client
	traceClient           *trace.Client
	log                   *logrus.Logger
}

var _ sinks.SpanSink = &GRPCSpanSink{}

// NewGRPCSpanSink creates a sinks.SpanSink that can write to any
// compliant gRPC server.
//
// The target parameter should be of the "host:port"; the name parameter is
// prepended with "grpc-", and is used when reporting logs in order to permit
// differentiation between various services.
//
// Any grpc.CallOpts that are provided will be used while first establishing the
// connection to the target server (in grpc.DialContext()).
func NewGRPCSpanSink(ctx context.Context, target, name string, log *logrus.Logger, opts ...grpc.DialOption) (*GRPCSpanSink, error) {
	name = "grpc-" + name
	conn, err := grpc.DialContext(convertContext(ctx), target, opts...)
	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"name":   name,
			"target": target,
		}).Error("Error establishing connection to gRPC server")
		return nil, err
	}

	return &GRPCSpanSink{
		grpcConn: conn,
		name:     name,
		target:   target,
		log:      log,
	}, nil
}

// Start performs final preparations on the sink before it is
// ready to begin ingesting spans.
func (gs *GRPCSpanSink) Start(cl *trace.Client) error {
	gs.traceClient = cl

	// Run a background goroutine to do a little bit of connection state
	// tracking.
	go func() {
		for {
			// This call will block on a channel receive until the gRPC connection
			// state changes. When it does, flip the marker over to allow another
			// error to be logged from Ingest().
			gs.grpcConn.WaitForStateChange(ocontext.Background(), gs.grpcConn.GetState())
			atomic.StoreUint32(&gs.loggedSinceTransition, 0)
		}
	}()
	return nil
}

// Name returns this sink's name. As the gRPC sink is generic, it's expected
// that this is set via configuration and injected.
func (gs *GRPCSpanSink) Name() string {
	return gs.name
}

// Ingest takes in a span and streams it over gRPC to the connected server.
func (gs *GRPCSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	client := NewSpanSinkClient(gs.grpcConn)
	_, err := client.SendSpan(ocontext.Background(), ssfSpan)

	if err != nil {
		atomic.AddUint32(&gs.dropCount, 1)

		// gRPC guarantees that an error returned from an RPC call will be of
		// type status.Status. In the unexpected event that they're not, this
		// call creates a dummy type, so there's no risk of panic.
		serr := status.Convert(err)
		state := gs.grpcConn.GetState()

		// Log all errors that occur in Ready state - that's weird. Otherwise,
		// Log only one error per underlying connection state transition. This
		// should be a reasonable heuristic to get an indicator that problems
		// are occurring, without resulting in massive log spew while
		// connections are under duress.
		if state == connectivity.Ready || atomic.CompareAndSwapUint32(&gs.loggedSinceTransition, 0, 1) {
			gs.log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"target":        gs.target,
				"name":          gs.name,
				"chanstate":     state.String(),
				"code":          serr.Code(),
				"details":       serr.Details(),
				"message":       serr.Message(),
			}).Error("Error sending span to gRPC sink target")
		}
	} else {
		atomic.AddUint32(&gs.sentCount, 1)
	}

	return err
}

// Flush reports total counts of the number of sent and dropped spans over its
// lifetime.
//
// No data is sent to the target sink on Flush(), as this sink operates entirely
// over a stream on Ingest().
func (gs *GRPCSpanSink) Flush() {
	samples := &ssf.Samples{}
	samples.Add(
		ssf.Count(
			sinks.MetricKeyTotalSpansFlushed,
			float32(atomic.SwapUint32(&gs.sentCount, 0)),
			map[string]string{"sink": gs.Name()}),
		ssf.Count(
			sinks.MetricKeyTotalSpansDropped,
			float32(atomic.SwapUint32(&gs.dropCount, 0)),
			map[string]string{"sink": gs.Name()},
		),
	)

	metrics.Report(gs.traceClient, samples)
	return
}
