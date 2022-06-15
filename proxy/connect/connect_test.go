package connect_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/proxy/connect"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

type FakeServer struct {
	closeConnection   chan struct{}
	connectionChannel chan forwardrpc.Forward_SendMetricsV2Server
	grpcListener      net.Listener
	handler           *forwardrpc.MockForwardServer
	serveError        chan error
	server            *grpc.Server
}

func CreateFakeServer(
	t *testing.T, ctrl *gomock.Controller,
) *FakeServer {
	grpcListener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	mockHandler := forwardrpc.NewMockForwardServer(ctrl)
	forwardrpc.RegisterForwardServer(server, mockHandler)

	serveError := make(chan error)
	go func() {
		serveError <- server.Serve(grpcListener)
	}()

	return &FakeServer{
		closeConnection:   make(chan struct{}),
		connectionChannel: make(chan forwardrpc.Forward_SendMetricsV2Server),
		grpcListener:      grpcListener,
		handler:           mockHandler,
		serveError:        serveError,
		server:            server,
	}
}

func (server *FakeServer) Close(t *testing.T) {
	close(server.closeConnection)

	server.server.GracefulStop()
	server.grpcListener.Close()

	serveError := <-server.serveError
	assert.NoError(t, serveError)
}

func TestConnect(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinationsHash := connect.NewMockDestinationHash(ctrl)
	server := CreateFakeServer(t, ctrl)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	metric := &metricpb.Metric{
		Name: "metric-name",
		Tags: []string{"tag1:value1"},
		Type: metricpb.Type_Counter,
		Value: &metricpb.Metric_Counter{
			Counter: &metricpb.CounterValue{
				Value: 1,
			},
		},
	}

	mockStatsd.EXPECT().Count(
		"veneur_proxy.grpc.conn_begin", int64(1),
		[]string{"client:true"}, 1.0)
	mockStatsd.EXPECT().Count(
		"veneur_proxy.forward.connect", int64(1),
		[]string{"status:success"}, 1.0)
	server.handler.EXPECT().SendMetricsV2(gomock.Any()).Times(1).DoAndReturn(func(
		connection forwardrpc.Forward_SendMetricsV2Server,
	) error {
		server.connectionChannel <- connection
		<-server.closeConnection
		connection.SendAndClose(&emptypb.Empty{})
		return nil
	})

	connecter := connect.Create(time.Second, logrus.NewEntry(logger), mockStatsd)
	destination, err := connecter.Connect(
		context.Background(), server.grpcListener.Addr().String(),
		mockDestinationsHash)
	assert.NoError(t, err)

	connection := <-server.connectionChannel

	errorChannel := make(chan error, 1)
	destination.SendChannel() <- connect.SendRequest{
		ErrorChannel: errorChannel,
		Metric:       metric,
	}
	actualMetric, err := connection.Recv()
	assert.NoError(t, err)
	assert.Equal(t, metric, actualMetric)

	mockStatsd.EXPECT().Count(
		"veneur_proxy.forward.disconnect", int64(1),
		[]string{"error:false"}, 1.0)
	mockDestinationsHash.EXPECT().RemoveDestination(
		server.grpcListener.Addr().String())
	connectionClosed := make(chan struct{})
	mockDestinationsHash.EXPECT().ConnectionClosed().Do(func() {
		close(connectionClosed)
	})
	mockStatsd.EXPECT().Count(
		"veneur_proxy.grpc.conn_end", int64(1),
		[]string{"client:true"}, 1.0)

	server.Close(t)
	<-connectionClosed
}

func TestConnectDialTimeoutExpired(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinationsHash := connect.NewMockDestinationHash(ctrl)

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStatsd.EXPECT().Count(
		"veneur_proxy.forward.connect", int64(1),
		[]string{"status:failed_dial"}, 1.0)

	connecter := connect.Create(0, logrus.NewEntry(logger), mockStatsd)
	_, err := connecter.Connect(
		context.Background(), "address", mockDestinationsHash)
	assert.Error(t, err)
	assert.Equal(t, "context deadline exceeded", err.Error())
}
