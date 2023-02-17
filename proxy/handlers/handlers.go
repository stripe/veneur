package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14/forwardrpc"
	"github.com/stripe/veneur/v14/proxy/connect"
	"github.com/stripe/veneur/v14/proxy/destinations"
	"github.com/stripe/veneur/v14/proxy/json"
	"github.com/stripe/veneur/v14/samplers/metricpb"
	"github.com/stripe/veneur/v14/scopedstatsd"
	"github.com/stripe/veneur/v14/util/matcher"
	"google.golang.org/protobuf/types/known/emptypb"
)

type Handlers struct {
	Destinations       destinations.Destinations
	HealthcheckContext context.Context
	IgnoreTags         []matcher.TagMatcher
	Logger             *logrus.Entry
	Statsd             scopedstatsd.Client
}

func (proxy *Handlers) HandleHealthcheck(
	writer http.ResponseWriter, request *http.Request,
) {
	if proxy.Destinations.Size() == 0 || proxy.HealthcheckContext.Err() != nil {
		writer.WriteHeader(http.StatusServiceUnavailable)
	} else {
		writer.WriteHeader(http.StatusNoContent)
	}
}

// Receives and handles metrics via HTTP requests.
func (proxy *Handlers) HandleJsonMetrics(
	writer http.ResponseWriter, request *http.Request,
) {
	proxy.Statsd.Count(
		"veneur_proxy.ingest.request_count", 1,
		[]string{"protocol:http"}, 1.0)
	requestStart := time.Now()
	defer proxy.Statsd.Timing(
		"veneur_proxy.ingest.request_latency_ms", time.Since(requestStart),
		[]string{"protocol:http"}, 1.0)

	jsonMetrics, err := json.ParseRequest(request)
	if err != nil {
		proxy.Logger.WithError(err.Err).Debug(err.Message)
		http.Error(writer, fmt.Sprintf("%s: %v", err.Message, err.Err), err.Status)
		proxy.Statsd.Count(
			"veneur_proxy.ingest.request_error_count", 1,
			[]string{"protocol:http", fmt.Sprintf("status:%s", err.Tag)}, 1.0)
		return
	}

	proxy.Statsd.Count(
		"veneur_proxy.ingest.metrics_count",
		int64(len(jsonMetrics)), []string{"protocol:http"}, 1.0)
	convertErrorCount := 0
	for _, jsonMetric := range jsonMetrics {
		metric, err := json.ConvertJsonMetric(&jsonMetric)
		if err != nil {
			proxy.Logger.Debugf("error converting metric: %v", err)
			convertErrorCount += 1
			continue
		}
		proxy.handleMetric(metric)
	}
	if convertErrorCount > 0 {
		proxy.Statsd.Count(
			"veneur_proxy.handle.metrics_count",
			int64(convertErrorCount), []string{"error:json_convert"}, 1.0)
	}
}

// Receives and handles metrics via individual gRPC requests.
func (proxy *Handlers) SendMetrics(
	ctx context.Context, metricList *forwardrpc.MetricList,
) (*empty.Empty, error) {
	proxy.Statsd.Count(
		"veneur_proxy.ingest.request_count", 1,
		[]string{"protocol:grpc-single"}, 1.0)
	requestStart := time.Now()
	defer proxy.Statsd.Timing(
		"veneur_proxy.ingest.request_latency_ms", time.Since(requestStart),
		[]string{"protocol:grpc-single"}, 1.0)

	proxy.Statsd.Count(
		"veneur_proxy.ingest.metrics_count",
		int64(len(metricList.Metrics)), []string{"protocol:grpc-single"}, 1.0)
	for _, metric := range metricList.Metrics {
		proxy.handleMetric(metric)
	}

	return &emptypb.Empty{}, nil
}

// Receives and handles metrics via gRPC streaming.
func (proxy *Handlers) SendMetricsV2(
	server forwardrpc.Forward_SendMetricsV2Server,
) error {
	proxy.Statsd.Count(
		"veneur_proxy.ingest.request_count", 1,
		[]string{"protocol:grpc-stream"}, 1.0)
	requestStart := time.Now()
	defer proxy.Statsd.Timing(
		"veneur_proxy.ingest.request_latency_ms", time.Since(requestStart),
		[]string{"protocol:grpc-stream"}, 1.0)

	defer func() {
		err := server.SendAndClose(&emptypb.Empty{})
		if err != nil {
			proxy.Logger.WithError(err).Error("error closing stream")
		}
	}()
	for {
		metric, err := server.Recv()
		if err == io.EOF {
			return nil
		} else if err != nil {
			proxy.Logger.WithError(err).Error("error receiving metrics")
			proxy.Statsd.Count(
				"veneur_proxy.ingest.request_error_count", 1,
				[]string{"protocol:grpc-stream"}, 1.0)
			return err
		}
		proxy.Statsd.Count(
			"veneur_proxy.ingest.metrics_count",
			int64(1), []string{"protocol:grpc-stream"}, 1.0)
		proxy.handleMetric(metric)
	}
}

// Forwards metrics to downstream global Veneur instances.
func (proxy *Handlers) handleMetric(metric *metricpb.Metric) {
	tags := []string{}
tagLoop:
	for _, tag := range metric.Tags {
		for _, matcher := range proxy.IgnoreTags {
			if matcher.Match(tag) {
				continue tagLoop
			}
		}
		tags = append(tags, tag)
	}

	key := fmt.Sprintf("%s%s%s",
		metric.Name, strings.ToLower(metric.Type.String()), strings.Join(tags, ","))
	destination, err := proxy.Destinations.Get(key)
	if err != nil {
		proxy.Logger.WithError(err).Debug("failed to get destination")
		proxy.Statsd.Count(
			"veneur_proxy.handle.metrics_count",
			int64(1), []string{"error:destination"}, 1.0)
		return
	}

	defer func() {
		recoverError := recover()
		if recoverError != nil {
			err = fmt.Errorf("%s", recoverError)
			proxy.Logger.WithError(err).Debug("failed to forward metric")
		}
	}()

	sendRequest := connect.SendRequest{
		Metric:    metric,
		Timestamp: time.Now(),
	}

	// sendChannel is a buffered channel with size configured by
	// `send_buffer_size`. In normal operation, the following write should be
	// non-blocking; however if the buffer fills up, we increment the counter
	// with error:enqueue and perform a blocking write.
	sendChannel := destination.SendChannel()
	closedChannel := destination.ClosedChannel()
	select {
	case sendChannel <- sendRequest: // Attempt a non-blocking write.
		proxy.Statsd.Count(
			"veneur_proxy.handle.metrics_count",
			int64(1), []string{"error:false"}, 1.0)
		return
	default:
		proxy.Statsd.Count(
			"veneur_proxy.handle.enqueue",
			int64(1), []string{"destination:" + destination.Address()}, 1.0)
		select {
		case sendChannel <- sendRequest: // Perform a blocking write.
			proxy.Statsd.Count(
				"veneur_proxy.handle.metrics_count",
				int64(1), []string{"error:false"}, 1.0)
			return
		case <-closedChannel: // Do not block if the destination is closed.
			proxy.Statsd.Count(
				"veneur_proxy.handle.metrics_count",
				int64(1), []string{"error:dropped"}, 1.0)
			return
		}
	}
}
