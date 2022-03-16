package prometheus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"text/template"

	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"

	"github.com/sirupsen/logrus"
)

// Max package size for TCP (and hardcoded for UDP in Statsd Exporter) is
// 65535 bytes. Assuming a generous 50 bytes for metric name, 10 bytes for
// value, and 200 bytes for tags, we can have ~200 packets per batch.
var batchSize = 200

// Use DogstatsD serialization format.
// https://github.com/prometheus/statsd_exporter#tagging-extensions.
const serializationFormat = "{{.Name}}:{{.Value}}|{{.Type}}|#{{.Tags}}\n"

type PrometheusMetricSinkConfig struct {
	RepeaterAddress string `yaml:"repeater_address"`
	NetworkType     string `yaml:"network_type"`
}

// In exporting to Prometheus as a metric sink, we are tentatively choosing to
// use https://github.com/prometheus/statsd_exporter.
type PrometheusMetricSink struct {
	name            string
	repeaterAddress string
	networkType     string
	logger          *logrus.Entry
	traceClient     *trace.Client
}

func ParseMetricConfig(name string, config interface{}) (veneur.MetricSinkConfig, error) {
	prometheusConfig := PrometheusMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &prometheusConfig)
	if err != nil {
		return nil, err
	}
	return prometheusConfig, nil
}

func CreateMetricSink(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	prometheusConfig, ok := sinkConfig.(PrometheusMetricSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	if prometheusConfig.NetworkType != "tcp" && prometheusConfig.NetworkType != "udp" {
		return nil, fmt.Errorf("Statsd Exporter only listens to TCP/UDP, but '%s' was requested", prometheusConfig.NetworkType)
	}

	if _, err := url.ParseRequestURI(prometheusConfig.RepeaterAddress); err != nil {
		return nil, err
	}

	return &PrometheusMetricSink{
		name:            name,
		repeaterAddress: prometheusConfig.RepeaterAddress,
		networkType:     prometheusConfig.NetworkType,
		logger:          logger,
		traceClient:     server.TraceClient,
	}, nil
}

func (sink *PrometheusMetricSink) Name() string {
	return sink.name
}

func (sink *PrometheusMetricSink) Start(traceClient *trace.Client) error {
	sink.traceClient = traceClient
	return nil
}

// Flush sends metrics to the Statsd Exporter in batches.
func (sink *PrometheusMetricSink) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(sink.traceClient)

	if len(interMetrics) == 0 {
		sink.logger.Info("Nothing to flush, skipping.")
		return nil
	}

	conn, err := net.Dial(sink.networkType, sink.repeaterAddress)
	if err != nil {
		return err
	}
	defer conn.Close()

	// Prometheus Statsd Exporter allows us to batch by using \n.
	// https://github.com/prometheus/statsd_exporter/blob/master/pkg/listener/listener.go.
	for i := 0; i < len(interMetrics); i += batchSize {
		end := i + batchSize
		if end > len(interMetrics) {
			end = len(interMetrics)
		}

		body := sink.serializeMetrics(interMetrics[i:end])
		if body == "" {
			continue
		}

		conn.Write([]byte(body))
	}

	sink.logger.Info(sinks.FlushSuccessMessage)

	return nil
}

// FlushOtherSamples sends events to SignalFx. This is a no-op for Prometheus
// sinks as Prometheus does not support other samples.
func (sink *PrometheusMetricSink) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
}

// serializeMetrics seralizes metrics according to the defined
// serializationFormat.
func (sink *PrometheusMetricSink) serializeMetrics(metrics []samplers.InterMetric) string {
	t := template.Must(template.New("statsd_metric").Parse(serializationFormat))

	statsdMetrics := []string{}
	for _, metric := range metrics {
		if !sinks.IsAcceptableMetric(metric, sink) {
			continue
		}

		var sm bytes.Buffer
		t.Execute(&sm, map[string]interface{}{
			"Name":  metric.Name,
			"Value": metric.Value,
			"Type":  metricTypeEnc(metric),
			"Tags":  strings.Join(metric.Tags, ","),
		})

		statsdMetrics = append(statsdMetrics, sm.String())
	}

	return strings.Join(statsdMetrics, "")
}

// metricTypeEnc returns "g" for gauge and status metrics, "c" for counters.
func metricTypeEnc(metric samplers.InterMetric) string {
	switch metric.Type {
	case samplers.GaugeMetric, samplers.StatusMetric:
		return "g"
	case samplers.CounterMetric:
		return "c"
	}
	return ""
}

func MigrateConfig(conf *veneur.Config) {
	if conf.PrometheusRepeaterAddress == "" {
		return
	}

	conf.MetricSinks = append(conf.MetricSinks, veneur.SinkConfig{
		Kind: "prometheus",
		Name: "prometheus",
		Config: PrometheusMetricSinkConfig{
			RepeaterAddress: conf.PrometheusRepeaterAddress,
			NetworkType:     conf.PrometheusNetworkType,
		},
	})
}
