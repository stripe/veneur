package proxy_test

import (
	"context"
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

func TestStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinations := destinations.NewMockDestinations(ctrl)

	server := proxy.Create(&proxy.CreateParams{
		Config: &proxy.Config{
			ForwardAddresses: []string{},
			ShutdownTimeout:  time.Second,
		},
		Destinations: mockDestinations,
		Discoverer:   nil,
		HttpHandler:  http.NewServeMux(),
		Logger:       logrus.NewEntry(logger),
		Statsd:       mockStatsd,
	})

	mockDestinations.EXPECT().Add(gomock.Any(), []string{})
	mockDestinations.EXPECT().Clear()

	ctx, cancel := context.WithCancel(context.Background())
	proxyError := make(chan error)
	go func() {
		proxyError <- server.Start(ctx)
	}()

	mockDestinations.EXPECT().Wait()

	cancel()
	err := <-proxyError
	assert.NoError(t, err)
}

func TestHandleDiscovery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	ctx := context.Background()
	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinations := destinations.NewMockDestinations(ctrl)
	mockDiscoverer := discovery.NewMockDiscoverer(ctrl)

	server := proxy.Create(&proxy.CreateParams{
		Config: &proxy.Config{
			ForwardAddresses: []string{},
			ForwardService:   "service-name",
			ShutdownTimeout:  time.Second,
		},
		Destinations: mockDestinations,
		Discoverer:   mockDiscoverer,
		HttpHandler:  http.NewServeMux(),
		Logger:       logrus.NewEntry(logger),
		Statsd:       mockStatsd,
	})

	mockDestinations.EXPECT().Add(ctx, []string{"address1", "address2"})
	mockDiscoverer.EXPECT().GetDestinationsForService("service-name").
		Return([]string{"address1", "address2"}, nil)
	mockStatsd.EXPECT().Count(
		"veneur_proxy.discovery.duration_total_ms", gomock.Any(), []string{}, 1.0)
	mockStatsd.EXPECT().Count(
		"veneur_proxy.discovery.count", int64(1), []string{"status:success"}, 1.0)
	mockStatsd.EXPECT().Gauge(
		"veneur_proxy.discovery.destinations", 2.0, []string{}, 1.0)

	server.HandleDiscovery(ctx)
}
