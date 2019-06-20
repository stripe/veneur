package main

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/DataDog/datadog-go/statsd"
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

	statsClient, _ := statsd.New(*statsHost)

	if *prefix != "" {
		statsClient.Namespace = *prefix
	}

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	i, err := time.ParseDuration(*interval)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to parse interval '%s'", *interval)
	}

	ignoredLabels := []*regexp.Regexp{}
	ignoredMetrics := []*regexp.Regexp{}

	for _, ignoredLabelStr := range strings.Split(*ignoredLabelsStr, ",") {
		if len(ignoredLabelStr) > 0 {
			ignoredLabels = append(ignoredLabels, regexp.MustCompile(ignoredLabelStr))
		}
	}
	for _, ignoredMetricStr := range strings.Split(*ignoredMetricsStr, ",") {
		if len(ignoredMetricStr) > 0 {
			ignoredMetrics = append(ignoredMetrics, regexp.MustCompile(ignoredMetricStr))
		}
	}

	httpClient := newHTTPClient(*cert, *key, *caCert)

	ticker := time.NewTicker(i)
	for _ = range ticker.C {
		collect(httpClient, statsClient, ignoredLabels, ignoredMetrics)
	}
}

func collect(httpClient *http.Client, statsClient *statsd.Client, ignoredLabels []*regexp.Regexp, ignoredMetrics []*regexp.Regexp) {
	logrus.WithFields(logrus.Fields{
		"stats_host":      *statsHost,
		"metrics_host":    *metricsHost,
		"ignored_labels":  ignoredLabels,
		"ignored_metrics": ignoredMetrics,
	}).Debug("Beginning collection")

	resp, err := httpClient.Get(*metricsHost)
	if err != nil {
		logrus.WithError(err).WithField("metrics_host", *metricsHost).Warn(fmt.Sprintf("Failed to collect metrics"))
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
			statsClient.Count("veneur.prometheus.decode_errors_total", 1, nil, 1.0)
			logrus.WithError(err).Warn("Failed to decode a metric")
			break
		}

		if !shouldExportMetric(mf, ignoredMetrics) {
			continue
		}

		var metricCount int64
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			for _, counter := range mf.GetMetric() {
				tags := getTags(counter.GetLabel(), ignoredLabels)
				statsClient.Count(mf.GetName(), int64(counter.GetCounter().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_GAUGE:
			for _, gauge := range mf.GetMetric() {
				tags := getTags(gauge.GetLabel(), ignoredLabels)
				statsClient.Gauge(mf.GetName(), float64(gauge.GetGauge().GetValue()), tags, 1.0)
				metricCount++
			}
		case dto.MetricType_SUMMARY:
			for _, summary := range mf.GetMetric() {
				tags := getTags(summary.GetLabel(), ignoredLabels)
				name := mf.GetName()
				data := summary.GetSummary()
				statsClient.Gauge(fmt.Sprintf("%s.sum", name), data.GetSampleSum(), tags, 1.0)
				statsClient.Count(fmt.Sprintf("%s.count", name), int64(data.GetSampleCount()), tags, 1.0)
				metricCount += 2 // One for sum, one for count, one for each percentile bucket

				for _, quantile := range data.GetQuantile() {
					v := quantile.GetValue()
					if !math.IsNaN(v) {
						statsClient.Gauge(fmt.Sprintf("%s.%dpercentile", name, int(quantile.GetQuantile()*100)), v, tags, 1.0)
						metricCount++
					}
				}
			}
		case dto.MetricType_HISTOGRAM:
			for _, histo := range mf.GetMetric() {
				tags := getTags(histo.GetLabel(), ignoredLabels)
				name := mf.GetName()
				data := histo.GetHistogram()
				statsClient.Gauge(fmt.Sprintf("%s.sum", name), data.GetSampleSum(), tags, 1.0)
				statsClient.Count(fmt.Sprintf("%s.count", name), int64(data.GetSampleCount()), tags, 1.0)
				metricCount += 2 // One for sum, one for count, one for each histo bucket

				for _, bucket := range data.GetBucket() {
					b := bucket.GetUpperBound()
					if !math.IsNaN(b) {
						statsClient.Count(fmt.Sprintf("%s.le%f", name, b), int64(bucket.GetCumulativeCount()), tags, 1.0)
						metricCount++
					}
				}
			}
		default:
			statsClient.Count("veneur.prometheus.unknown_metric_type_total", 1, nil, 1.0)
		}
		statsClient.Count("veneur.prometheus.metrics_flushed_total", metricCount, nil, 1.0)
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

func newHTTPClient(certPath, keyPath, caCertPath string) *http.Client {
	var caCertPool *x509.CertPool
	var clientCerts []tls.Certificate

	if certPath != "" {
		clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			logrus.WithError(err).Fatalf("error reading client cert and key")
		}
		clientCerts = append(clientCerts, clientCert)
	}

	if caCertPath != "" {
		caCertPool = x509.NewCertPool()
		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			logrus.WithError(err).Fatalf("error reading ca cert")
		}
		caCertPool.AppendCertsFromPEM(caCert)
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: clientCerts,
				RootCAs:      caCertPool,
			},
		},
	}

	return client
}
