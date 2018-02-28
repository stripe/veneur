package grpsink

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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

// GRPCStreamingSpanSink is a sink that streams spans to a configurable target
// service over gRPC. The sink is only tied to the grpc_sink.proto definition of
// a SpanSink service, and thus is generic with respect to the specific server
// it is connecting to.
type GRPCStreamingSpanSink struct {
	name                     string
	target                   string
	grpcConn                 *grpc.ClientConn
	sinkClient               SpanSinkClient
	stream                   SpanSink_SendSpansClient
	streamMut                sync.Mutex
	stats                    *statsd.Client
	commonTags               map[string]string
	traceClient              *trace.Client
	log                      *logrus.Logger
	sentCount, errCount, bad uint32
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
	// We want the stream in fail-fast mode. This is the default, but it's
	// important to the design, so it's worth being explicit: if the streams
	// weren't in fail-fast mode, then Ingest() calls will block while connections
	// get re-established in the background, resulting in undesirable backpressure.
	// For a use case like this sink, unequivocally preferred to fail fast instead,
	// resulting in dropped spans.
	dco := grpc.WithDefaultCallOptions(grpc.FailFast(true))
	conn, err := grpc.DialContext(ctx, target, append(opts, dco)...)
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

// Start performs final preparations on the sink before it is
// ready to begin ingesting spans.
func (gs *GRPCStreamingSpanSink) Start(cl *trace.Client) error {
	gs.traceClient = cl

	var err error
	gs.stream, err = gs.sinkClient.SendSpans(context.TODO())
	if err != nil {
		gs.log.WithFields(logrus.Fields{
			"name":          gs.name,
			"target":        gs.target,
			"chanstate":     gs.grpcConn.GetState().String(),
			logrus.ErrorKey: err,
		}).Error("Failed to set up a stream over gRPC channel to sink target")
		return err
	}

	go gs.maintainStream()
	return nil
}

// maintainStream is intended to be run in a background goroutine. It is kicked
// off by Start(), and is only kept as a standalone method for testing purposes.
//
// This method runs an endless loop that reacts to state changes in the
// underlying channel by automatically attempting to re-establish the stream
// connection when it moves back into the 'READY' state. See
// https://github.com/grpc/grpc/blob/master/doc/connectivity-semantics-and-api.mdA
// for details about gRPC's connectivity state machine.
func (gs *GRPCStreamingSpanSink) maintainStream() {
	for {
		// This call will block on a channel receive until the gRPC connection
		// becomes unhealthy. Then, spring into action!
		state := gs.grpcConn.GetState()
		switch state {
		case connectivity.Idle:
			// gRPC considers an open stream to be an active RPC, and channels
			// normally only go idle if there are no active RPC within the
			// channel's timeout window. It should be nigh-impossible for that
			// to happen here, so we record entering the Idle state as an error.
			gs.log.WithFields(logrus.Fields{
				"name":          gs.name,
				"target":        gs.target,
				"chanstate":     gs.grpcConn.GetState().String(),
				logrus.ErrorKey: fmt.Errorf("gRPC sink became idle; should be nearly impossible"),
			}).Error("gRPC channel transitioned to idle")

			// With the error sent, now fall through to recreating the stream
			// as if the channel were ready, as stream creation should force
			// the channel back out of idle. This isn't ideal - it'll end up
			// recreating the stream again after re-reaching READY - but it's
			// not a huge problem, as this case is essentially unreachable.
			fallthrough
		case connectivity.Ready:
			gs.streamMut.Lock()
			// CAS inside the mutex so that we can't ever get a double-send of the
			// "first time channel degradation" log error.
			atomic.CompareAndSwapUint32(&gs.bad, 1, 0)
			stream, err := gs.sinkClient.SendSpans(context.TODO())
			if err != nil {
				// This is a weird case and probably shouldn't be reachable, but
				// if stream setup fails after recovery, then just wait for a
				// second and try again.
				gs.log.WithFields(logrus.Fields{
					"name":          gs.name,
					"target":        gs.target,
					"chanstate":     gs.grpcConn.GetState().String(),
					logrus.ErrorKey: err,
				}).Error("Failed to set up a new stream after gRPC channel recovered")
				time.Sleep(1 * time.Second)
				gs.streamMut.Unlock()
				continue
			}
			gs.stream = stream
			gs.streamMut.Unlock()
			gs.log.WithFields(logrus.Fields{
				"name":      gs.name,
				"target":    gs.target,
				"chanstate": gs.grpcConn.GetState().String(),
			}).Info("gRPC channel and stream re-established")
		case connectivity.Connecting:
			gs.log.WithFields(logrus.Fields{
				"name":      gs.name,
				"target":    gs.target,
				"chanstate": state,
			}).Info("Attempting to re-establish gRPC channel connection")
			// Nothing to do here except wait.
		case connectivity.TransientFailure:
			gs.log.WithFields(logrus.Fields{
				"name":      gs.name,
				"target":    gs.target,
				"chanstate": state,
			}).Warn("gRPC channel now in transient failure")
		case connectivity.Shutdown:
			// The current design has no path to actually shutting down the
			// connection, so this should be unreachable.
			return
		}
		gs.grpcConn.WaitForStateChange(context.Background(), state)
	}
}

// Name returns this sink's name. As the gRPC sink is generic, it's expected
// that this is set via configuration and injected.
func (gs *GRPCStreamingSpanSink) Name() string {
	return gs.name
}

// Ingest takes in a span and streams it over gRPC to the connected server.
func (gs *GRPCStreamingSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}
	// TODO(sdboyer) validation of span, e.g. time bounds like in datadog sink?

	// Apply any common tags, superseding defaults passed in at sink creation
	// in the event of overlap.
	for k, v := range gs.commonTags {
		ssfSpan.Tags[k] = v
	}

	// gRPC indicates that it is safe to send and receive on a stream at the
	// same time, but it is not safe to simultaneously send. Guard against that
	// with a mutex.
	gs.streamMut.Lock()
	err := gs.stream.Send(ssfSpan)
	gs.streamMut.Unlock()

	if err != nil {
		atomic.AddUint32(&gs.errCount, 1)

		// gRPC guarantees that an error returned from an RPC call will be of
		// type status.Status. In the unexpected event that they're not, this
		// call creates a dummy type, so there's no risk of panic.
		serr := status.Convert(err)

		state := gs.grpcConn.GetState()
		// We also want to emit a log for the very first time a connection
		// appears to go bad. This value is reset by the stream maintainer
		// goroutine when the channel re-enters a good state.
		if atomic.CompareAndSwapUint32(&gs.bad, 0, 1) {
			gs.log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"target":        gs.target,
				"name":          gs.name,
				"chanstate":     state.String(),
				"code":          serr.Code(),
				"details":       serr.Details(),
				"message":       serr.Message(),
			}).Warn("Error sending trace to gRPC server")

		} else if state == connectivity.Ready {
			// We expect to see dropped spans while the connection is recovering
			// from temporary failure. Logging every such failure would be a
			// vector for write amplification. However, it's abnormal to see an
			// error when the underlying connection is in a good (READY) state.
			// (Note that may be possible for some spans to error before gRPC
			// channel state transitions away from READY)
			gs.log.WithFields(logrus.Fields{
				logrus.ErrorKey: err,
				"target":        gs.target,
				"name":          gs.name,
				"chanstate":     state.String(),
				"code":          serr.Code(),
				"details":       serr.Details(),
				"message":       serr.Message(),
			}).Error("gRPC chan state is READY but error was returned")
		}
	} else {
		atomic.AddUint32(&gs.sentCount, 1)
	}

	return err
}

// Flush reports metrics about the number of failed and successful
//
// No data is sent to the target sink on Flush(); this sink operates
// entirely over a stream on Ingest().
func (gs *GRPCStreamingSpanSink) Flush() {
	samples := &ssf.Samples{}
	samples.Add(ssf.Count(
		sinks.MetricKeyTotalSpansFlushed,
		float32(atomic.LoadUint32(&gs.sentCount)),
		map[string]string{"sink": gs.Name()},
	))
	samples.Add(ssf.Count(
		sinks.MetricKeyTotalSpansDropped,
		float32(atomic.LoadUint32(&gs.errCount)),
		map[string]string{"sink": gs.Name()},
	))

	metrics.Report(gs.traceClient, samples)
	return
}
