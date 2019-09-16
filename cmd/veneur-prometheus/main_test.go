package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
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
		Help: "A typical counter vec with labels that will be added and some that won't.",
	},
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

	statsC := &memoryStatsC{}

	cfg := config{
		metricsHost:    ts.URL,
		statsHost:      "unit-test-no-host",
		httpClient:     newHTTPClient("", "", ""),
		statsClient:    statsC,
		ignoredLabels:  getIgnoredFromArg("ignore"),
		ignoredMetrics: getIgnoredFromArg("(.*)_ignore,promhttp_(.*)"),
	}

	collect(cfg)

	assert.True(t, statsC.Contains(stat{Name: "counter"}))
	assert.True(t, statsC.Contains(stat{Name: "counter_vec", Tags: []string{"a:A1", "b:B1"}}))
	assert.False(t, statsC.Contains(stat{Name: "counter_ignored"}))

	assert.True(t, statsC.Contains(stat{Name: "gauge"}))
	assert.True(t, statsC.Contains(stat{Name: "gauge_vec", Tags: []string{"c:C1", "d:D1"}}))
	assert.False(t, statsC.Contains(stat{Name: "gauge_ignored"}))

	assert.True(t, statsC.Contains(stat{Name: "summary.sum"}))
	assert.True(t, statsC.Contains(stat{Name: "summary.count"}))
	assert.True(t, statsC.Contains(stat{Name: "summary.50percentile"}))
	assert.True(t, statsC.Contains(stat{Name: "summary.90percentile"}))
	assert.True(t, statsC.Contains(stat{Name: "summary.99percentile"}))

	assert.True(t, statsC.Contains(stat{Name: "summary_vec.sum", Tags: []string{"e:E1", "f:F1"}}))
	assert.True(t, statsC.Contains(stat{Name: "summary_vec.count", Tags: []string{"e:E1", "f:F1"}}))
	assert.True(t, statsC.Contains(stat{Name: "summary_vec.50percentile", Tags: []string{"e:E1", "f:F1"}}))
	assert.True(t, statsC.Contains(stat{Name: "summary_vec.90percentile", Tags: []string{"e:E1", "f:F1"}}))
	assert.True(t, statsC.Contains(stat{Name: "summary_vec.99percentile", Tags: []string{"e:E1", "f:F1"}}))

	assert.False(t, statsC.Contains(stat{Name: "summary_ignored.sum"}))

	assert.True(t, statsC.Contains(stat{Name: "histogram.sum"}))
	assert.True(t, statsC.Contains(stat{Name: "histogram.count"}))
	assert.True(t, statsC.Contains(stat{Name: "histogram.le1.000000"}))
	assert.True(t, statsC.Contains(stat{Name: "histogram.le2.000000"}))
	assert.True(t, statsC.Contains(stat{Name: "histogram.le5.000000"}))

	assert.True(t, statsC.Contains(stat{Name: "histogram_vec.sum", Tags: []string{"g:G1", "h:H1"}}))
	assert.True(t, statsC.Contains(stat{Name: "histogram_vec.count", Tags: []string{"g:G1", "h:H1"}}))
	assert.True(t, statsC.Contains(stat{Name: "histogram_vec.le1.000000", Tags: []string{"g:G1", "h:H1"}}))
	assert.True(t, statsC.Contains(stat{Name: "histogram_vec.le2.000000", Tags: []string{"g:G1", "h:H1"}}))
	assert.True(t, statsC.Contains(stat{Name: "histogram_vec.le5.000000", Tags: []string{"g:G1", "h:H1"}}))

	assert.False(t, statsC.Contains(stat{Name: "histogram_ignored.sum"}))

	assert.True(t, statsC.Contains(stat{Name: "veneur.prometheus.metrics_flushed_total"}))
}

type memoryStatsC struct {
	stats []interface{}
}

func (m *memoryStatsC) Count(name string, value int64, tags []string, scale float64) error {
	m.stats = append(m.stats, count{stat{name, tags}, value})
	return nil
}

func (m *memoryStatsC) Gauge(name string, value float64, tags []string, scale float64) error {
	m.stats = append(m.stats, gauge{stat{name, tags}, value})
	return nil
}

func (m *memoryStatsC) Contains(s stat) bool {
	for _, t := range m.stats {
		var o stat
		switch v := t.(type) {
		case count:
			o = v.stat
		case gauge:
			o = v.stat
		default:
			panic(fmt.Sprintf("unknown stat type %T", v))
		}

		if s.Same(o) {
			return true
		}
	}

	return false
}

func TestGetTags(t *testing.T) {
	label1Name := "label1Name"
	label1Value := "label1Value"
	label1Pair := &dto.LabelPair{
		Name:  &label1Name,
		Value: &label1Value,
	}

	label2Name := "label2Name"
	label2Value := "label2Value"
	label2Pair := &dto.LabelPair{
		Name:  &label2Name,
		Value: &label2Value,
	}

	label3Name := "label3Name"
	label3Value := "label3Value"
	label3Pair := &dto.LabelPair{
		Name:  &label3Name,
		Value: &label3Value,
	}

	labels := []*dto.LabelPair{
		label1Pair, label2Pair, label3Pair,
	}

	ignoredLabels := []*regexp.Regexp{
		regexp.MustCompile(".*5.*"),
		regexp.MustCompile(".*abel1.*"),
	}

	tags := getTags(labels, ignoredLabels)
	expectedTags := []string{
		"label2Name:label2Value",
		"label3Name:label3Value",
	}

	assert.Equal(t, expectedTags, tags)
}

func TestShouldExportMetric(t *testing.T) {
	metric1Name := "metric1Name"

	mf := dto.MetricFamily{
		Name: &metric1Name,
	}

	ignoredMetrics1 := []*regexp.Regexp{
		regexp.MustCompile(".*0.*"),
		regexp.MustCompile(".*1.*"),
	}
	ignoredMetrics2 := []*regexp.Regexp{regexp.MustCompile(".*2.*")}

	assert.False(t, shouldExportMetric(mf, ignoredMetrics1))
	assert.True(t, shouldExportMetric(mf, ignoredMetrics2))
}

func TestGetHTTPClientHTTP(t *testing.T) {
	client := newHTTPClient("", "", "")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	res, err := client.Get(ts.URL)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)
}

func TestGetHTTPClientHTTPS(t *testing.T) {
	client := newHTTPClient("./testdata/client.pem", "./testdata/client.key", "./testdata/root.pem")

	caCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile("./testdata/root.pem")
	assert.NoError(t, err)
	caCertPool.AppendCertsFromPEM(caCert)

	serverCert, err := tls.LoadX509KeyPair("./testdata/server.pem", "./testdata/server.key")
	assert.NoError(t, err)

	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caCertPool,
	}
	ts.StartTLS()
	defer ts.Close()

	res, err := client.Get(ts.URL)
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, http.StatusOK)
}
