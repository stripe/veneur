package main

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectGetsAllMetricsItShould(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "counter_vec",
		Help: "A typical counter vec with labels that will be added and some that won't."},
		[]string{"a", "b", "ignore_me"},
	)

	counterIgnored := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter_ignored",
		Help: "A typical counter that will be regexed out.",
	})

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gauge",
		Help: "A typical gauge",
	})

	gaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gauge_vec",
		Help: "A typical guage vec with labels that will be added and some that won't.",
	},
		[]string{"c", "d", "ignore"},
	)

	gaugeIgnored := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gauge_ignored",
		Help: "A typical gauge that will be regexed out.",
	})

	summary := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "summary",
		Help:       "A typical summary.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})

	summaryIgnored := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "summary_ignored",
		Help:       "A typical summary that will be ignored.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})

	summaryVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       "summary_vec",
		Help:       "A typical summary vec with labels that will be added and some that won't.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
		[]string{"e", "f", "ignore"},
	)

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram",
		Help:    "A typical histogram.",
		Buckets: []float64{1, 2, 5},
	})

	histogramIgnored := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram_ignored",
		Help:    "A typical histogram that will be ignored.",
		Buckets: []float64{1, 2, 5},
	})

	histogramVec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "histogram_vec",
		Help:    "A typical histogram vec with labels that will be added and some that won't.",
		Buckets: []float64{1, 2, 5},
	},
		[]string{"g", "h", "ignore"},
	)

	ts, err := testPrometheusEndpoint(
		counter,
		counterVec,
		counterIgnored,
		gauge,
		gaugeVec,
		gaugeIgnored,
		summary,
		summaryVec,
		summaryIgnored,
		histogram,
		histogramVec,
		histogramIgnored,
	)
	require.NoError(t, err)
	defer ts.Close()

	//give all the results a count in case a zero filter happens
	counter.Inc()
	counterVec.WithLabelValues("A1", "B1", "Ignored1").Inc()
	counterIgnored.Inc()

	gauge.Set(1)
	gaugeVec.WithLabelValues("C1", "D1", "Ignored2").Set(1)
	gaugeIgnored.Set(1)

	summary.Observe(30)
	summaryVec.WithLabelValues("E1", "F1", "Ignored3").Observe(30)
	summaryIgnored.Observe(30)

	histogram.Observe(45)
	histogramVec.WithLabelValues("G1", "H1", "Ignored4").Observe(45)
	histogramIgnored.Observe(45)

	cfg := prometheusConfig{
		metricsHost:    ts.URL,
		httpClient:     newHTTPClient("", "", ""),
		ignoredLabels:  getIgnoredFromArg("ignore"),
		ignoredMetrics: getIgnoredFromArg("(.*)_ignore,promhttp_(.*)"),
	}

	counts, gauges := splitStats(collect(cfg))

	assert.True(t, countReceived(counts, "counter"))
	assert.True(t, countReceived(counts, "counter_vec", "a:A1", "b:B1"))
	assert.False(t, countReceived(counts, "counter_ignored"))

	assert.True(t, gaugeReceived(gauges, "gauge"))
	assert.True(t, gaugeReceived(gauges, "gauge_vec", "c:C1", "d:D1"))
	assert.False(t, gaugeReceived(gauges, "gauge_ignored"))

	assert.True(t, gaugeReceived(gauges, "summary.sum"))
	assert.True(t, countReceived(counts, "summary.count"))
	assert.True(t, gaugeReceived(gauges, "summary.50percentile"))
	assert.True(t, gaugeReceived(gauges, "summary.90percentile"))
	assert.True(t, gaugeReceived(gauges, "summary.99percentile"))

	assert.True(t, gaugeReceived(gauges, "summary_vec.sum", "e:E1", "f:F1"))
	assert.True(t, countReceived(counts, "summary_vec.count", "e:E1", "f:F1"))
	assert.True(t, gaugeReceived(gauges, "summary_vec.50percentile", "e:E1", "f:F1"))
	assert.True(t, gaugeReceived(gauges, "summary_vec.90percentile", "e:E1", "f:F1"))
	assert.True(t, gaugeReceived(gauges, "summary_vec.99percentile", "e:E1", "f:F1"))

	assert.False(t, gaugeReceived(gauges, "summary_ignored.sum"))

	assert.True(t, gaugeReceived(gauges, "histogram.sum"))
	assert.True(t, countReceived(counts, "histogram.count"))
	assert.True(t, countReceived(counts, "histogram.le1.000000"))
	assert.True(t, countReceived(counts, "histogram.le2.000000"))
	assert.True(t, countReceived(counts, "histogram.le5.000000"))

	assert.True(t, gaugeReceived(gauges, "histogram_vec.sum", "g:G1", "h:H1"))
	assert.True(t, countReceived(counts, "histogram_vec.count", "g:G1", "h:H1"))
	assert.True(t, countReceived(counts, "histogram_vec.le1.000000", "g:G1", "h:H1"))
	assert.True(t, countReceived(counts, "histogram_vec.le2.000000", "g:G1", "h:H1"))
	assert.True(t, countReceived(counts, "histogram_vec.le5.000000", "g:G1", "h:H1"))

	assert.False(t, gaugeReceived(gauges, "histogram_ignored.sum"))

	assert.True(t, countReceived(counts, "veneur.prometheus.metrics_flushed_total"))
}

func splitStats(stats <-chan []inMemoryStat) ([]count, []gauge) {
	var c []count
	var g []gauge
	for batch := range stats {
		for _, s := range batch {
			switch i := s.(type) {
			case count:
				c = append(c, i)
			case gauge:
				g = append(g, i)

			default:
				panic(fmt.Sprintf("unknown inmemory stat type: %T", s))
			}
		}
	}

	return c, g
}

func gaugeReceived(gauges []gauge, name string, tags ...string) bool {
	b := statID{name, tags}
	for _, a := range gauges {
		if same(a.statID, b) {
			return true
		}
	}

	return false
}

func countReceived(counts []count, name string, tags ...string) bool {
	b := statID{name, tags}
	for _, a := range counts {
		if same(a.statID, b) {
			return true
		}
	}

	return false
}
