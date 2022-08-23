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
	Destinations destinations.Destinations
	IgnoreTags   []matcher.TagMatcher
	Logger       *logrus.Entry
	Shutdown     bool
	Statsd       scopedstatsd.Client
}

func (proxy *Handlers) HandleHealthcheck(
	writer http.ResponseWriter, request *http.Request,
) {
	if proxy.Shutdown {
		writer.WriteHeader(http.StatusNotFound)
	} else if proxy.Destinations.Size() == 0 {
		writer.WriteHeader(http.StatusNotFound)
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

	errorCount := 0
	for _, jsonMetric := range jsonMetrics {
		metric, err := json.ConvertJsonMetric(&jsonMetric)
		if err != nil {
			proxy.Logger.Debugf("error converting metric: %v", err)
			errorCount += 1
			continue
		}
		err = proxy.handleMetric(metric)
		if err != nil {
			errorCount += 1
		}
	}

	proxy.Statsd.Count(
		"veneur_proxy.ingest.metrics_count", int64(len(jsonMetrics)-errorCount),
		[]string{"error:false", "protocol:http"}, 1.0)
	if errorCount > 0 {
		proxy.Statsd.Count(
			"veneur_proxy.ingest.metrics_count", int64(errorCount),
			[]string{"error:true", "protocol:http"}, 1.0)
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

	errorCount := 0
	for _, metric := range metricList.Metrics {
		err := proxy.handleMetric(metric)
		if err != nil {
			errorCount += 1
		}
	}

	proxy.Statsd.Count(
		"veneur_proxy.ingest.metrics_count",
		int64(len(metricList.Metrics)-errorCount),
		[]string{"error:false", "protocol:grpc-single"}, 1.0)
	if errorCount > 0 {
		proxy.Statsd.Count(
			"veneur_proxy.ingest.metrics_count", int64(errorCount),
			[]string{"error:true", "protocol:grpc-single"}, 1.0)
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
			proxy.Logger.WithError(err).Debug("error receiving metrics")
			proxy.Statsd.Count(
				"veneur_proxy.ingest.request_error_count", 1,
				[]string{"protocol:grpc-stream"}, 1.0)
			return err
		}
		err = proxy.handleMetric(metric)
		if err != nil {
			proxy.Statsd.Count(
				"veneur_proxy.ingest.metrics_count",
				int64(1), []string{"error:true", "protocol:grpc-stream"}, 1.0)
		} else {
			proxy.Statsd.Count(
				"veneur_proxy.ingest.metrics_count",
				int64(1), []string{"error:false", "protocol:grpc-stream"}, 1.0)
		}
	}
}

// Forwards metrics to downstream global Veneur instances.
func (proxy *Handlers) handleMetric(metric *metricpb.Metric) error {
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
			"veneur_proxy.forward.metrics_count", 1,
			[]string{"error:true"}, 1.0)
		return err
	}

	defer func() {
		err := recover()
		if err != nil {
			proxy.Logger.
				WithError(fmt.Errorf("%s", err)).
				Debug("failed to forward metric")
			proxy.Statsd.Count(
				"veneur_proxy.forward.metrics_count", 1,
				[]string{"error:true"}, 1.0)
		}
	}()

	errorChannel := make(chan error)
	destination.SendChannel() <- connect.SendRequest{
		Metric:       metric,
		ErrorChannel: errorChannel,
	}
	return <-errorChannel
}
