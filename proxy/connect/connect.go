package connect

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/proxy/grpcstats"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"google.golang.org/grpc"
)

var ErrSampleDropped = errors.New("sample dropped")

type Connect interface {
	Connect(context.Context, string, DestinationHash) (Destination, error)
}

var _ Connect = &connect{}

type connect struct {
	dialTimeout time.Duration
	logger      *logrus.Entry
	sendBuffer  uint
	statsd      scopedstatsd.Client
}

func Create(
	dialTimeout time.Duration, logger *logrus.Entry, sendBuffer uint,
	statsd scopedstatsd.Client,
) Connect {
	return &connect{
		dialTimeout: dialTimeout,
		logger:      logger,
		sendBuffer:  sendBuffer,
		statsd:      statsd,
	}
}

type Destination interface {
	SendChannel() chan<- SendRequest
	Close()
}

type SendRequest struct {
	Metric *metricpb.Metric
}

type DestinationHash interface {
	RemoveDestination(string)
	ConnectionClosed()
}

var _ Destination = &destination{}

type destination struct {
	address         string
	cancel          func()
	client          forwardrpc.Forward_SendMetricsV2Client
	connection      *grpc.ClientConn
	destinationHash DestinationHash
	logger          *logrus.Entry
	sendChannel     chan SendRequest
	statsd          scopedstatsd.Client
}

func (connect *connect) Connect(
	ctx context.Context, address string, destinationHash DestinationHash,
) (Destination, error) {
	logger := connect.logger.WithField("destination", address)

	// Dial the destination.
	logger.Debug("dialing destination")
	connection, err := func() (*grpc.ClientConn, error) {
		dialContext, cancel := context.WithTimeout(ctx, connect.dialTimeout)
		defer cancel()
		return grpc.DialContext(
			dialContext, address, grpc.WithBlock(), grpc.WithInsecure(),
			grpc.WithStatsHandler(&grpcstats.StatsHandler{
				IsClient: true,
				Statsd:   connect.statsd,
			}))
	}()
	if err != nil {
		logger.WithError(err).Error("failed to dial destination")
		connect.statsd.Count(
			"veneur_proxy.forward.connect", 1, []string{"status:failed_dial"}, 1.0)
		return nil, err
	}

	// Open a streaming gRPC connection to the destination.
	logger.Debug("connecting to destination")
	forwardClient := forwardrpc.NewForwardClient(connection)
	client, err := forwardClient.SendMetricsV2(ctx)
	if err != nil {
		logger.WithError(err).Error("failed to connect to destination")
		connect.statsd.Count(
			"veneur_proxy.forward.connect", 1, []string{"status:failed_connect"}, 1.0)
		return nil, err
	}

	sendContext, cancel := context.WithCancel(ctx)
	d := destination{
		address:         address,
		cancel:          cancel,
		client:          client,
		connection:      connection,
		destinationHash: destinationHash,
		logger:          logger,
		sendChannel:     make(chan SendRequest, connect.sendBuffer),
		statsd:          connect.statsd,
	}

	go d.sendMetrics(sendContext)
	go d.listenForClose()

	// Add the destination to the consistent hash.
	d.logger.Debug("connected to destination")
	d.statsd.Count(
		"veneur_proxy.forward.connect", 1, []string{"status:success"}, 1.0)
	return &d, nil
}

// Send metrics to the destination. Once the context expires, remove the the
// destination and close the connection.
func (d *destination) sendMetrics(ctx context.Context) {
sendLoop:
	for {
		select {
		case request := <-d.sendChannel:
			var errorTag string
			err := d.client.Send(request.Metric)
			if err == io.EOF {
				d.logger.WithError(err).Debug("failed to forward metric")
				errorTag = "error:eof"
			} else if err != nil {
				d.logger.WithError(err).Debug("failed to forward metric")
				errorTag = "error:forward"
			} else {
				errorTag = "error:false"
			}
			d.statsd.Count(
				"veneur_proxy.forward.metrics_count", 1,
				[]string{errorTag}, 1.0)
			if err == io.EOF {
				break sendLoop
			}
		case <-ctx.Done():
			break sendLoop
		}
	}

	d.destinationHash.RemoveDestination(d.address)
	close(d.sendChannel)

	err := d.client.CloseSend()
	if err != nil {
		d.logger.WithError(err).Error("failed to close stream")
	}
	err = d.connection.Close()
	if err != nil {
		d.logger.WithError(err).Error("failed to close connection")
	}

	d.statsd.Count(
		"veneur_proxy.forward.metrics_count", int64(len(d.sendChannel)),
		[]string{"error:dropped"}, 1.0)
	for range d.sendChannel {
		// Do nothing.
	}

	d.destinationHash.ConnectionClosed()
}

// Listen for the streaming gRPC connection to the destination to close, and
// cancel the context once it does.
func (d *destination) listenForClose() {
	var empty empty.Empty
	err := d.client.RecvMsg(&empty)
	if err == nil || err == io.EOF {
		d.logger.Debug("disconnected from destination")
		d.statsd.Count(
			"veneur_proxy.forward.disconnect", 1, []string{"error:false"}, 1.0)
	} else {
		d.logger.WithError(err).Error("disconnected from destination")
		d.statsd.Count(
			"veneur_proxy.forward.disconnect", 1, []string{"error:true"}, 1.0)
	}

	d.cancel()
}

func (d *destination) SendChannel() chan<- SendRequest {
	return d.sendChannel
}

func (d *destination) Close() {
	d.cancel()
}
