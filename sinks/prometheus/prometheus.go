package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"text/template"

	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"

	"github.com/sirupsen/logrus"
)

// Max package size for TCP (and hardcoded for UDP in Statsd Exporter) is
// 65535 bytes. Assuming a generous 50 bytes for metric name, 10 bytes for
// value, and 200 bytes for tags, we can have ~200 packets per batch.
var batchSize = 200

// Use DogstatsD serialization format.
// https://github.com/prometheus/statsd_exporter#tagging-extensions.
const serializationFormat = "{{.Name}}:{{.Value}}|{{.Type}}|#{{.Tags}}\n"

// StatsdRepeater is the metric sink implementation for Prometheus.
// In exporting to Prometheus as a metric sink, we are tentatively choosing to
// use https://github.com/prometheus/statsd_exporter.
type StatsdRepeater struct {
	addr        string
	network     string
	logger      *logrus.Logger
	traceClient *trace.Client
}

// NewStatsdRepeater returns a new StatsdRepeater, validating addr and network.
func NewStatsdRepeater(addr string, network string, logger *logrus.Logger) (*StatsdRepeater, error) {
	// TODO(yanke): what about UNIX sockets?
	if network != "tcp" && network != "udp" {
		return nil, fmt.Errorf("Statsd Exporter only listens to TCP/UDP, but %s was requested", network)
	}
	if _, err := url.ParseRequestURI(addr); err != nil {
		return nil, err
	}

	return &StatsdRepeater{
		addr:    addr,
		network: network,
		logger:  logger,
	}, nil
}

// Name returns the name of this sink.
func (s *StatsdRepeater) Name() string {
	return "prometheus"
}

// Start begins the sink.
func (s *StatsdRepeater) Start(cl *trace.Client) error {
	s.traceClient = cl
	return nil
}

// Flush sends metrics to the Statsd Exporter in batches.
func (s *StatsdRepeater) Flush(ctx context.Context, interMetrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)

	if len(interMetrics) == 0 {
		s.logger.Info("Nothing to flush, skipping.")
		return nil
	}

	conn, err := net.Dial(s.network, s.addr)
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

		body := s.serializeMetrics(interMetrics[i:end])
		if body == "" {
			continue
		}

		conn.Write([]byte(body))
	}

	return nil
}

// FlushOtherSamples sends events to SignalFx. This is a no-op for Prometheus
// sinks as Prometheus does not support other samples.
func (s *StatsdRepeater) FlushOtherSamples(ctx context.Context, samples []ssf.SSFSample) {
}

// serializeMetrics seralizes metrics according to the defined
// serializationFormat.
func (s *StatsdRepeater) serializeMetrics(metrics []samplers.InterMetric) string {
	t := template.Must(template.New("statsd_metric").Parse(serializationFormat))

	statsdMetrics := []string{}
	for _, metric := range metrics {
		if !sinks.IsAcceptableMetric(metric, s) {
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
