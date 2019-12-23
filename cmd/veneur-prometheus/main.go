package main

import (
	"flag"
	"time"

	"github.com/DataDog/datadog-go/statsd"
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
	socket = flag.String("socket", "", "The path to a unix socket to use for transport. Useful for certains styles of proxy.")
)

func main() {
	flag.Parse()

	if *debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	i, err := time.ParseDuration(*interval)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error":    err,
			"interval": *interval,
		}).Fatal("failed to parse interval")
	}

	statsClient, _ := statsd.New(*statsHost)

	if *prefix != "" {
		statsClient.Namespace = *prefix
	}

	cfg, err := prometheusConfigFromArguments()
	if err != nil {
		logrus.WithError(err).Fatal("unable to build prometheus config")
	}

	cache := new(countCache)
	ticker := time.NewTicker(i)
	for _ = range ticker.C {
		statsdStats := collect(cfg, cache)
		sendToStatsd(statsClient, *statsHost, statsdStats)
	}
}

func collect(cfg prometheusConfig, cache *countCache) <-chan []statsdStat {
	logrus.WithFields(logrus.Fields{
		"metrics_host":    cfg.metricsHost,
		"ignored_labels":  cfg.ignoredLabels,
		"ignored_metrics": cfg.ignoredMetrics,
	}).Debug("beginning collection")

	prometheus := queryPrometheus(cfg.httpClient, cfg.metricsHost, cfg.ignoredMetrics)
	return translatePrometheus(cfg.ignoredLabels, cache, prometheus)
}

func sendToStatsd(client *statsd.Client, host string, stats <-chan []statsdStat) {
	logrus.WithField("stats_host", host).Debug("beginning stats send")

	for batch := range stats {
		for _, s := range batch {
			err := s.Send(client)

			if err != nil {
				logrus.
					WithError(err).
					WithField("stats_host", host).
					Warn("failed sending stats")
			}
		}
	}
}
