package main

import (
	"flag"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/sinks/debug"
	"github.com/stripe/veneur/v14/sources/openmetrics"
)

var (
	configFile    = flag.String("f", "config.yaml", "Config file.")
	sampleCounter = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "demo",
		Name:      "sample_counter",
		Help:      "Sample counter.",
	})
	sampleGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "demo",
		Name:      "sample_gauge",
		Help:      "Sample gauge.",
	})
	sampleHistogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: "demo",
		Name:      "sample_histogram",
		Help:      "Sample histogram.",
		Buckets:   []float64{2.0, 5.0},
	})
	sampleSummary = prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace: "demo",
		Name:      "sample_summary",
		Help:      "Sample summary.",
		Objectives: map[float64]float64{
			0.5: 0.01,
			0.9: 0.01,
		},
	})
)

func main() {
	flag.Parse()
	logger := logrus.StandardLogger()

	prometheus.MustRegister(
		sampleCounter,
		sampleGauge,
		sampleHistogram,
		sampleSummary,
	)

	config, err := veneur.ReadConfig(logrus.NewEntry(logger), *configFile)
	if err != nil {
		logger.Fatal(err.Error())
	}

	server, err := veneur.NewFromConfig(veneur.ServerConfig{
		Config: config,
		Logger: logger,
		SourceTypes: veneur.SourceTypes{
			"openmetrics": {
				Create:      openmetrics.Create,
				ParseConfig: openmetrics.ParseConfig,
			},
		},
		MetricSinkTypes: veneur.MetricSinkTypes{
			"debug": {
				Create:      debug.CreateMetricSink,
				ParseConfig: debug.ParseMetricConfig,
			},
		},
	})
	if err != nil {
		logger.Fatal(err.Error())
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2112", nil)
	}()

	go func() {
		ticker := time.NewTicker(config.Interval)
		gaugeValue := 1
		for range ticker.C {
			sampleCounter.Inc()
			sampleGauge.Set(float64(gaugeValue))
			sampleHistogram.Observe(float64(gaugeValue))
			sampleSummary.Observe(rand.NormFloat64() * 10)

			gaugeValue = (gaugeValue * 5) % 7
		}
	}()

	server.Start()
	server.Serve()
}
