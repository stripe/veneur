package proxy_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/discovery"
	"github.com/stripe/veneur/v14/proxy"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/scopedstatsd"
)

type TestServer struct {
	*proxy.Proxy
	Statsd       *scopedstatsd.MockClient
	Destinations *destinations.MockDestinations
	Discoverer   *discovery.MockDiscoverer
}

func CreateTestServer(
	ctrl *gomock.Controller, config *proxy.Config,
) *TestServer {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinations := destinations.NewMockDestinations(ctrl)
	mockDiscoverer := discovery.NewMockDiscoverer(ctrl)

	return &TestServer{
		Proxy: proxy.Create(&proxy.CreateParams{
			Config:       config,
			Destinations: mockDestinations,
			Discoverer:   mockDiscoverer,
			HttpHandler:  http.NewServeMux(),
			Logger:       logrus.NewEntry(logger),
			Statsd:       mockStatsd,
		}),
		Destinations: mockDestinations,
		Discoverer:   mockDiscoverer,
		Statsd:       mockStatsd,
	}
}

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := CreateTestServer(ctrl, &proxy.Config{
		ForwardAddresses: []string{},
		ShutdownTimeout:  time.Second,
	})
	server.Destinations.EXPECT().Add(gomock.Any(), []string{})
	server.Destinations.EXPECT().Clear()

	ctx, cancel := context.WithCancel(context.Background())
	proxyError := make(chan error)
	go func() {
		proxyError <- server.Start(ctx)
	}()

	<-server.Ready()

	server.Destinations.EXPECT().Size().Return(1)
	httpResponse, err :=
		http.Get(fmt.Sprintf("http://%s/healthcheck", server.GetHttpAddress()))
	if assert.NoError(t, err) {
		assert.Equal(t, http.StatusNoContent, httpResponse.StatusCode)
	}

	server.Destinations.EXPECT().Wait()
	cancel()

	err = <-proxyError
	assert.NoError(t, err)
}

func TestStartCloseHttpListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := CreateTestServer(ctrl, &proxy.Config{
		ForwardAddresses: []string{},
		ShutdownTimeout:  time.Second,
	})
	server.Destinations.EXPECT().Add(gomock.Any(), []string{})
	server.Destinations.EXPECT().Clear()

	proxyError := make(chan error)
	go func() {
		proxyError <- server.Start(context.Background())
	}()

	<-server.Ready()

	server.Destinations.EXPECT().Wait()
	server.CloseHttpListener()

	err := <-proxyError
	assert.Error(t, err)
}

func TestStartCloseGrpcListener(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := CreateTestServer(ctrl, &proxy.Config{
		ForwardAddresses: []string{},
		ShutdownTimeout:  time.Second,
	})
	server.Destinations.EXPECT().Add(gomock.Any(), []string{})
	server.Destinations.EXPECT().Clear()

	proxyError := make(chan error)
	go func() {
		proxyError <- server.Start(context.Background())
	}()

	<-server.Ready()

	server.Destinations.EXPECT().Wait()
	server.CloseGrpcListener()

	err := <-proxyError
	assert.Error(t, err)
}

func TestHandleDiscovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	server := CreateTestServer(ctrl, &proxy.Config{
		ForwardAddresses: []string{},
		ForwardService:   "service-name",
		ShutdownTimeout:  time.Second,
	})
	ctx := context.Background()

	server.Destinations.EXPECT().Add(ctx, []string{"address1", "address2"})
	server.Discoverer.EXPECT().GetDestinationsForService("service-name").
		Return([]string{"address1", "address2"}, nil)
	server.Statsd.EXPECT().Count(
		"veneur_proxy.discovery.duration_total_ms", gomock.Any(), []string{}, 1.0)
	server.Statsd.EXPECT().Count(
		"veneur_proxy.discovery.count", int64(1), []string{"status:success"}, 1.0)
	server.Statsd.EXPECT().Gauge(
		"veneur_proxy.discovery.destinations", 2.0, []string{}, 1.0)

	server.HandleDiscovery(ctx)
}
