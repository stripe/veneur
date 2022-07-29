package handlers_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
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
)

type TestHandlers struct {
	Destination  *connect.MockDestination
	Destinations *destinations.MockDestinations
	Discoverer   *discovery.MockDiscoverer
	Handlers     *handlers.Handlers
	Statsd       *scopedstatsd.MockClient
}

func CreateTestHandlers(
	ctrl *gomock.Controller, ignoreTags []matcher.TagMatcher,
) *TestHandlers {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)

	mockStatsd := scopedstatsd.NewMockClient(ctrl)
	mockDestinations := destinations.NewMockDestinations(ctrl)
	mockDiscoverer := discovery.NewMockDiscoverer(ctrl)
	mockDestination := connect.NewMockDestination(ctrl)

	return &TestHandlers{
		Destination:  mockDestination,
		Destinations: mockDestinations,
		Discoverer:   mockDiscoverer,
		Handlers: &handlers.Handlers{
			Destinations: mockDestinations,
			Logger:       logrus.NewEntry(logger),
			Statsd:       mockStatsd,
			IgnoreTags:   ignoreTags,
		},
		Statsd: mockStatsd,
	}
}

func TestHealthcheckFail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Destinations.EXPECT().Size().Return(0)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/healthcheck", nil)
	fixture.Handlers.HandleHealthcheck(recorder, request)

	assert.Equal(t, http.StatusNotFound, recorder.Result().StatusCode)
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

var metricsJson = []byte("[{\"name\":\"metric-name\",\"type\":\"counter\",\"tags\":[\"tag1:value1\",\"tag2:value2\"],\"value\":[1,0,0,0,0,0,0,0]}]")

func TestProxyJson(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:http"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"error:false", "protocol:http"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/import", bytes.NewReader(metricsJson))
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

func TestProxyGrpcSingle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fixture := CreateTestHandlers(ctrl, []matcher.TagMatcher{})

	metric := &metricpb.Metric{
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

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-single"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"error:false", "protocol:grpc-single"}, 1.0)

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

	metric := &metricpb.Metric{
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

	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.request_count",
		int64(1), []string{"protocol:grpc-stream"}, 1.0)
	fixture.Statsd.EXPECT().Count(
		"veneur_proxy.ingest.metrics_count",
		int64(1), []string{"error:false", "protocol:grpc-stream"}, 1.0)

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag1:value1,tag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)

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

	metric := &metricpb.Metric{
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

	fixture.Statsd.EXPECT().Count(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	fixture.Destinations.EXPECT().
		Get("metric-namecountertag2:value2").
		Return(fixture.Destination, nil)
	sendChannel := make(chan connect.SendRequest)
	fixture.Destination.EXPECT().SendChannel().Return(sendChannel)

	mockServer := forwardrpc.NewMockForward_SendMetricsV2Server(ctrl)
	mockServer.EXPECT().Recv().Times(1).Return(metric, nil)
	mockServer.EXPECT().Recv().Times(1).Return(nil, io.EOF)

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
