package handlers_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/discovery"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/proxy/connect"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/proxy/handlers"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"github.com/stripe/veneur/v14/util/matcher"
	"google.golang.org/protobuf/types/known/emptypb"
)

type TestHandlers struct {
	Destination              *connect.MockDestination
	Destinations             *destinations.MockDestinations
	Discoverer               *discovery.MockDiscoverer
	Handlers                 *handlers.Handlers
	HealthcheckContext       context.Context
	HealthcheckContextCancel func()
	Statsd                   *scopedstatsd.MockClient
}

func CreateTestHandlers(
	ctrl *gomock.Controller, ignoreTags []matcher.TagMatcher,
) *TestHandlers {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	healthcheckContext, healthcheckContextCancel :=
		context.WithCancel(context.Background())
	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinations := destinations.NewMockDestinations(ctrl)
	mockDiscoverer := discovery.NewMockDiscoverer(ctrl)
	mockDestination := connect.NewMockDestination(ctrl)

	return &TestHandlers{
		Destination:  mockDestination,
		Destinations: mockDestinations,
		Discoverer:   mockDiscoverer,
		Handlers: &handlers.Handlers{
			Destinations:       mockDestinations,
			Logger:             logrus.NewEntry(logger),
			Statsd:             mockStatsd,
			HealthcheckContext: healthcheckContext,
			IgnoreTags:         ignoreTags,
		},
		HealthcheckContext:       healthcheckContext,
		HealthcheckContextCancel: healthcheckContextCancel,
		Statsd:                   mockStatsd,
	}
}

func TestHealthcheckFailDestinations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})
	fixture.Destinations.EXPECT().Size().Return(0)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/healthcheck", nil)
	fixture.Handlers.HandleHealthcheck(recorder, request)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Result().StatusCode)
}

func TestHealthcheckFailContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})
	fixture.Destinations.EXPECT().Size().AnyTimes().Return(1)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/healthcheck", nil)
	fixture.Handlers.HandleHealthcheck(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Result().StatusCode)

	fixture.HealthcheckContextCancel()

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest("GET", "/healthcheck", nil)
	fixture.Handlers.HandleHealthcheck(recorder, request)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Result().StatusCode)
}

func TestHealthcheckSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Destinations.EXPECT().Size().Return(3)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/healthcheck", nil)
	fixture.Handlers.HandleHealthcheck(recorder, request)

	assert.Equal(t, http.StatusNoContent, recorder.Result().StatusCode)
}

const metricsJson = "[{\"name\":\"metric-name\",\"type\":\"counter\",\"tags\":[\"tag1:value1\",\"tag2:value2\"],\"value\":[1,0,0,0,0,0,0,0]}]"

func TestProxyJson(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:false"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		"GET", "/import", strings.NewReader(metricsJson))
	handleJsonMetricsChannel := make(chan struct{})
	go func() {
		fixture.Handlers.HandleJsonMetrics(recorder, request)
		close(handleJsonMetricsChannel)
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- nil
	<-handleJsonMetricsChannel

	assert.Equal(t, &metricpb.Metric{
		Name: "metric-name",
		Tags: []string{"tag1:value1", "tag2:value2"},
		Type: metricpb.Type_Counter,
		Value: &metricpb.Metric_Counter{
			Counter: &metricpb.CounterValue{
				Value: 1,
			},
		},
		Scope: metricpb.Scope_Global,
	}, sendRequest.Metric)
	assert.Equal(t, http.StatusOK, recorder.Result().StatusCode)
}

func TestProxyJsonBadRequest(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_error_count",
		int64(1), []string{"protocol:http", "status:error_decode"}, 1.0)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/import", strings.NewReader("["))
	fixture.Handlers.HandleJsonMetrics(recorder, request)
}

const metricsJsonBadType = "[{\"name\":\"metric-name\",\"type\":\"unknown\"}]"

func TestProxyJsonConvertError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:json_convert"}, 1.0)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(
		"GET", "/import", strings.NewReader(metricsJsonBadType))
	fixture.Handlers.HandleJsonMetrics(recorder, request)
}

var metric = &metricpb.Metric{
	Name: "metric-name",
	Tags: []string{"tag1:value1", "tag2:value2"},
	Type: metricpb.Type_Counter,
	Value: &metricpb.Metric_Counter{
		Counter: &metricpb.CounterValue{
			Value: 1,
		},
	},
	Scope: metricpb.Scope_Global,
}

func TestProxyGrpcSingle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-single"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-single"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-single"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:false"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	sendMetricsChannel := make(chan error)
	go func() {
		_, err := fixture.Handlers.SendMetrics(
			context.Background(), &forwardrpc.MetricList{
				Metrics: []*metricpb.Metric{metric},
			})
		sendMetricsChannel <- err
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- nil
	err := <-sendMetricsChannel

	assert.Equal(t, metric, sendRequest.Metric)
	assert.NoError(t, err)
}

func TestProxyGrpcStream(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:false"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	sendMetricsChannel := make(chan error)
	go func() {
		sendMetricsChannel <- fixture.Handlers.SendMetricsV2(mockServer)
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- nil
	err := <-sendMetricsChannel

	assert.Equal(t, metric, sendRequest.Metric)
	assert.NoError(t, err)
}

func TestProxyGrpcStreamError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:false"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_error_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, errors.New("stream error"))
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	sendMetricsChannel := make(chan error)
	go func() {
		sendMetricsChannel <- fixture.Handlers.SendMetricsV2(mockServer)
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- nil
	err := <-sendMetricsChannel

	assert.Equal(t, metric, sendRequest.Metric)
	assert.Error(t, err)
}

func TestProxyGrpcStreamIgnoreTags(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{
		matcher.CreateTagMatcher(&matcher.TagMatcherConfig{
			Kind:  "prefix",
			Unset: false,
			Value: "tag1",
		}),
	})

	fixture.Statsd.EXPECT().Count(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
	fixture.Statsd.EXPECT().Timing(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	sendMetricsChannel := make(chan error)
	go func() {
		sendMetricsChannel <- fixture.Handlers.SendMetricsV2(mockServer)
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- nil
	err := <-sendMetricsChannel

	assert.Equal(t, metric, sendRequest.Metric)
	assert.NoError(t, err)
}

func TestNoDestination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:destination"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(nil, errors.New("no destination"))

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	err := fixture.Handlers.SendMetricsV2(mockServer)
	assert.NoError(t, err)
}

func TestForwardError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:forward"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	sendMetricsChannel := make(chan error)
	go func() {
		sendMetricsChannel <- fixture.Handlers.SendMetricsV2(mockServer)
	}()
	sendRequest := <-sendChannel
	sendRequest.ErrorChannel <- errors.New("forward error")
	err := <-sendMetricsChannel

	assert.Equal(t, metric, sendRequest.Metric)
	assert.NoError(t, err)
}

func TestChannelBufferFull(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Timing(
		"veneur_proxy.ingest.request_latency_ms",
		gomock.Any(), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.handle.metrics_count",
		int64(1), []string{"error:enqueue"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)
	mockServer.EXPECT().SendAndClose(&emptypb.Empty{}).Return(nil)

	err := fixture.Handlers.SendMetricsV2(mockServer)
	assert.NoError(t, err)
}
