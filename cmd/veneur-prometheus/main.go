package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"regexp"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
)

var (
	debug             = flag.Bool("d", false, "Enable debug mode")
	metricsHost       = flag.String("h", "http://localhost:9090/metrics", "The full URL — like 'http://localhost:9090/metrics' to query for Prometheus metrics.")
	interval          = flag.String("i", "10s", "The interval at which to query. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	ignoredLabelsStr  = flag.String("ignored-labels", "", "A comma-seperated list of label name regexes to not export")
	ignoredMetricsStr = flag.String("ignored-metrics", "", "A comma-seperated list of metric name regexes to not export")
	prefix            = flag.String("p", "", "A prefix to append to any metrics emitted. Include a trailing period. (e.g. \"myservice.\")")
	statsHost         = flag.String("s", "127.0.0.1:8126", "The host and port — like '127.0.0.1:8126' — to send our metrics to.")

	// mTLS params for collecting metrics
	cert   = flag.String("cert", "", "The path to a client cert to present to the server. Only used if using mTLS.")
	key    = flag.String("key", "", "The path to a private key to use for mTLS. Only used if using mTLS.")
	caCert = flag.String("cacert", "", "The path to a CA cert used to validate the server certificate. Only used if using mTLS.")
)

func main() {
	flag.Parse()

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	i, err := time.ParseDuration(*interval)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to parse interval '%s'", *interval)
	}

	cfg := configFromArgs()
	ticker := time.NewTicker(i)
	for _ = range ticker.C {
		collect(cfg)
	}
}

type statsC interface {
	Count(string, int64, []string, float64) error
	Gauge(string, float64, []string, float64) error
}

func collect(cfg config) {
	logrus.WithFields(logrus.Fields{
		"stats_host":      cfg.statsHost,
		"metrics_host":    cfg.metricsHost,
		"ignored_labels":  cfg.ignoredLabels,
		"ignored_metrics": cfg.ignoredMetrics,
	}).Debug("Beginning collection")

	resp, err := cfg.httpClient.Get(cfg.metricsHost)
	if err != nil {
		logrus.
			WithError(err).
			WithField("metrics_host", cfg.metricsHost).
			Warn(fmt.Sprintf("Failed to collect metrics"))
		return
	}

	d := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
	var mf dto.MetricFamily
	for {
		err := d.Decode(&mf)
		if err == io.EOF {
			// We've hit the end, break out!
			break
		} else if err != nil {
			cfg.statsClient.Count("veneur.prometheus.decode_errors_total", 1, nil, 1.0)
			logrus.WithError(err).Warn("Failed to decode a metric")
			break
		}

		if !shouldExportMetric(mf, cfg.ignoredMetrics) {
			continue
		}

		var metricCount int64
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			for _, counter := range mf.GetMetric() {
				tags := getTags(counter.GetLabel(), cfg.ignoredLabels)
				cfg.statsClient.Count(mf.GetName(), int64(counter.GetCounter().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_GAUGE:
			for _, gauge := range mf.GetMetric() {
				tags := getTags(gauge.GetLabel(), cfg.ignoredLabels)
				cfg.statsClient.Gauge(mf.GetName(), float64(gauge.GetGauge().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_SUMMARY:
			for _, summary := range mf.GetMetric() {
				tags := getTags(summary.GetLabel(), cfg.ignoredLabels)
				name := mf.GetName()
				data := summary.GetSummary()
				cfg.statsClient.Gauge(fmt.Sprintf("%s.sum", name), data.GetSampleSum(), tags, 1.0)
				cfg.statsClient.Count(fmt.Sprintf("%s.count", name), int64(data.GetSampleCount()), tags, 1.0)
				metricCount += 2 // One for sum, one for count, one for each percentile bucket

				for _, quantile := range data.GetQuantile() {
					v := quantile.GetValue()
					if !math.IsNaN(v) {
						cfg.statsClient.Gauge(fmt.Sprintf("%s.%dpercentile", name, int(quantile.GetQuantile()*100)), v, tags, 1.0)
						metricCount++
					}
				}
			}
		case dto.MetricType_HISTOGRAM:
			for _, histo := range mf.GetMetric() {
				tags := getTags(histo.GetLabel(), cfg.ignoredLabels)
				name := mf.GetName()
				data := histo.GetHistogram()
				cfg.statsClient.Gauge(fmt.Sprintf("%s.sum", name), data.GetSampleSum(), tags, 1.0)
				cfg.statsClient.Count(fmt.Sprintf("%s.count", name), int64(data.GetSampleCount()), tags, 1.0)
				metricCount += 2 // One for sum, one for count, one for each histo bucket

				for _, bucket := range data.GetBucket() {
					b := bucket.GetUpperBound()
					if !math.IsNaN(b) {
						cfg.statsClient.Count(fmt.Sprintf("%s.le%f", name, b), int64(bucket.GetCumulativeCount()), tags, 1.0)
						metricCount++
					}
				}
			}
		default:
			cfg.statsClient.Count("veneur.prometheus.unknown_metric_type_total", 1, nil, 1.0)
		}
		cfg.statsClient.Count("veneur.prometheus.metrics_flushed_total", metricCount, nil, 1.0)
	}
}

func getTags(labels []*dto.LabelPair, ignoredLabels []*regexp.Regexp) []string {
	var tags []string

	for _, pair := range labels {
		labelName := pair.GetName()
		labelValue := pair.GetValue()
		include := true

		for _, ignoredLabel := range ignoredLabels {
			if ignoredLabel.MatchString(labelName) {
				include = false
				break
			}
		}

		if include {
			tags = append(tags, fmt.Sprintf("%s:%s", labelName, labelValue))
		}
	}

	return tags
}

func shouldExportMetric(mf dto.MetricFamily, ignoredMetrics []*regexp.Regexp) bool {
	for _, ignoredMetric := range ignoredMetrics {
		metricName := mf.GetName()

		if ignoredMetric.MatchString(metricName) {
			return false
		}
	}

	return true
}
