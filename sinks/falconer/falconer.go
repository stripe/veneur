package falconer

import (
	"context"
	"strconv"
	"sync/atomic"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/util"

	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/protocol"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/trace/metrics"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type FalconerSpanSinkConfig struct {
	Target string `yaml:"target"`
}

// FalconerSpanSink is a generic sink that sends spans to a configurable target
// service over gRPC. The sink is only tied to the grpc_sink.proto definition of
// a SpanSink service, and thus is agnostic with respect to the specific server
// it is connecting to.
type FalconerSpanSink struct {
	// Total counts of dropped spans
	dropCount uint32
	// The underlying gRPC connection ("channel") to the target server.
	grpcConn *grpc.ClientConn
	log      *logrus.Entry
	// Marker to indicate if an error has been logged since the last state
	// transition. Allows us to guarantee only one error message per state
	// change.
	loggedSinceTransition uint32
	name                  string
	// Total counts of sent spans
	sentCount   uint32
	ssc         SpanSinkClient
	target      string
	traceClient *trace.Client
}

var _ sinks.SpanSink = &FalconerSpanSink{}

func MigrateConfig(conf *veneur.Config) {
	if conf.FalconerAddress == "" {
		return
	}
	conf.SpanSinks = append(conf.SpanSinks, veneur.SinkConfig{
		Kind: "falconer",
		Name: "falconer",
		Config: FalconerSpanSinkConfig{
			Target: conf.FalconerAddress,
		},
	})
}

// creates a sinks.SpanSink that can write to any
// compliant gRPC server.
//
// The target parameter should be of the "host:port"; the name parameter is
// prepended with "grpc-", and is used when reporting logs in order to permit
// differentiation between various services.
//
// Any grpc.CallOpts that are provided will be used while first establishing the
// connection to the target server (in grpc.DialContext()).
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.SpanSinkConfig,
) (sinks.SpanSink, error) {
	grpcConfig := sinkConfig.(FalconerSpanSinkConfig)
	name = "grpc-" + name
	conn, err := grpc.DialContext(
		context.Background(), grpcConfig.Target, grpc.WithInsecure())
	if err != nil {
		logger.WithError(err).WithFields(logrus.Fields{
			"name":   name,
			"target": grpcConfig.Target,
		}).Error("Error establishing connection to gRPC server")
		return nil, err
	}

	return &FalconerSpanSink{
		grpcConn: conn,
		ssc:      NewSpanSinkClient(conn),
		name:     name,
		target:   grpcConfig.Target,
		log:      logger,
	}, nil
}

// ParseConfig decodes the map config for a Grpc sink into a GrpcSpanSinkConfig
// struct.
func ParseConfig(
	name string, config interface{},
) (veneur.SpanSinkConfig, error) {
	grpcConfig := FalconerSpanSinkConfig{}
	err := util.DecodeConfig(name, config, &grpcConfig)
	if err != nil {
		return nil, err
	}
	return grpcConfig, nil
}

// Start performs final preparations on the sink before it is
// ready to begin ingesting spans.
func (gs *FalconerSpanSink) Start(cl *trace.Client) error {
	gs.traceClient = cl

	// Run a background goroutine to do a little bit of connection state
	// tracking.
	go func() {
		for {
			// This call will block on a channel receive until the gRPC connection
			// state changes. When it does, flip the marker over to allow another
			// error to be logged from Ingest().
			gs.grpcConn.WaitForStateChange(
				context.Background(), gs.grpcConn.GetState())
			atomic.StoreUint32(&gs.loggedSinceTransition, 0)
		}
	}()
	return nil
}

// Name returns this sink's name. As the gRPC sink is generic, it's expected
// that this is set via configuration and injected.
func (gs *FalconerSpanSink) Name() string {
	return gs.name
}

// Ingest takes in a span and streams it over gRPC to the connected server.
func (gs *FalconerSpanSink) Ingest(ssfSpan *ssf.SSFSpan) error {
	if err := protocol.ValidateTrace(ssfSpan); err != nil {
		return err
	}

	ctx := metadata.AppendToOutgoingContext(
		context.Background(), "x-veneur-trace-id",
		strconv.FormatInt(ssfSpan.TraceId, 16))
	_, err := gs.ssc.SendSpan(ctx, ssfSpan)

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
		if state == connectivity.Ready ||
			atomic.CompareAndSwapUint32(&gs.loggedSinceTransition, 0, 1) {
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

// Flush reports total counts of the number of sent and dropped spans since the
// last flush.
//
// No data is sent to the target sink by this call, as this sink dispatches all
// spans directly via gRPC during Ingest().
func (gs *FalconerSpanSink) Flush() {
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
}
