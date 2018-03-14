package main

import (
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	glob "github.com/ryanuber/go-glob"
	"github.com/sirupsen/logrus"
)

var (
	debug             = flag.Bool("d", false, "Enable debug mode")
	metricsHost       = flag.String("h", "http://localhost:9090/metrics", "The full URL — like 'http://localhost:9090/metrics' to query for Prometheus metrics.")
	interval          = flag.String("i", "10s", "The interval at which to query. Value must be parseable by time.ParseDuration (https://golang.org/pkg/time/#ParseDuration).")
	blockedLabelsStr  = flag.String("l", "", "A comma-seperated list of label name globs to not export")
	blockedMetricsStr = flag.String("m", "", "A comma-seperated list of metric name globs to not export")
	prefix            = flag.String("p", "", "A prefix to append to any metrics emitted. Do not include a trailing period.")
	statsHost         = flag.String("s", "127.0.0.1:8126", "The host and port — like '127.0.0.1:8126' — to send our metrics to.")
)

func main() {
	flag.Parse()

	c, _ := statsd.New(*statsHost)

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	i, err := time.ParseDuration(*interval)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to parse interval '%s'", *interval)
	}

	blockedLabels := strings.Split(*blockedLabelsStr, ",")
	blockedMetrics := strings.Split(*blockedMetricsStr, ",")

	ticker := time.NewTicker(i)
	for _ = range ticker.C {
		collect(c, blockedLabels, blockedMetrics)
	}

	if *prefix != "" {
		c.Namespace = *prefix
	}
}

func collect(c *statsd.Client, blockedLabels []string, blockedMetrics []string) {
	logrus.WithFields(logrus.Fields{
		"stats_host":   *statsHost,
		"metrics_host": *metricsHost,
	}).Debug("Beginning collection")

	resp, _ := http.Get(*metricsHost)
	d := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
	var mf dto.MetricFamily
	for {
		err := d.Decode(&mf)
		if err == io.EOF {
			// We've hit the end, break out!
			break
		} else if err != nil {
			c.Count("veneur.prometheus.decode_errors_total", 1, nil, 1.0)
			logrus.WithError(err).Warn("Failed to decode a metric")
			break
		}

		if !shouldExportMetric(mf, blockedMetrics) {
			continue
		}

		var metricCount int64
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			for _, counter := range mf.GetMetric() {
				tags := getTags(counter.GetLabel(), blockedLabels)
				c.Count(mf.GetName(), int64(counter.GetCounter().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_GAUGE:
			for _, gauge := range mf.GetMetric() {
				tags := getTags(gauge.GetLabel(), blockedLabels)
				c.Gauge(mf.GetName(), float64(gauge.GetGauge().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_HISTOGRAM, dto.MetricType_SUMMARY:
			for _, histo := range mf.GetMetric() {
				tags := getTags(histo.GetLabel(), blockedLabels)
				hname := mf.GetName()
				summ := histo.GetSummary()
				c.Gauge(fmt.Sprintf("%s.sum", hname), summ.GetSampleSum(), tags, 1.0)
				c.Gauge(fmt.Sprintf("%s.count", hname), float64(summ.GetSampleCount()), tags, 1.0)
				for _, quantile := range summ.GetQuantile() {
					v := quantile.GetValue()
					if !math.IsNaN(v) {
						c.Gauge(fmt.Sprintf("%s.%dpercentile", hname, int(quantile.GetQuantile()*100)), v, tags, 1.0)
						metricCount++
					}
				}
			}
		default:
			c.Count("veneur.prometheus.unknown_metric_type_total", 1, nil, 1.0)
		}
		c.Count("veneur.prometheus.metrics_flushed_total", metricCount, nil, 1.0)
	}
}

func getTags(labels []*dto.LabelPair, blockedLabels []string) []string {
	var tags []string

	for _, pair := range labels {
		labelName := pair.GetName()
		labelValue := pair.GetValue()
		include := true

		for _, blockedLabel := range blockedLabels {
			if glob.Glob(blockedLabel, labelName) {
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

func shouldExportMetric(mf dto.MetricFamily, blockedMetrics []string) bool {
	for _, blockedMetric := range blockedMetrics {
		metricName := mf.GetName()

		if glob.Glob(blockedMetric, metricName) {
			return false
		}
	}

	return true
}
