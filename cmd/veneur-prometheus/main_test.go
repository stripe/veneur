package main

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUntyped(t *testing.T) {
	untypedVal := float64(8989)

	untyped := prometheus.NewUntypedFunc(prometheus.UntypedOpts{
		Name: "untyped",
	}, func() float64 { return untypedVal })

	ts, err := testPrometheusEndpoint(
		untyped,
	)
	require.NoError(t, err)
	defer ts.Close()

	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	cache := new(countCache)
	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges := splitStats(stats)
	unknown, _ := countValue(counts, "veneur.prometheus.unknown_metric_type_total")
	assert.Equal(t, 0, unknown)

	got, _ := gaugeValue(gauges, "untyped")
	assert.Equal(t, float64(8989), got)
}

func TestVeneurGeneratedCountsAreCorrect(t *testing.T) {
	counter1 := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter1",
		Help: "A typical counter.",
	})

	counter2 := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter2",
		Help: "A typical counter.",
	})

	counter3 := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter3",
		Help: "A typical counter.",
	})

	ts, err := testPrometheusEndpoint(
		counter1,
		counter2,
		counter3,
	)
	require.NoError(t, err)
	defer ts.Close()

	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	cache := new(countCache)
	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ := splitStats(stats)
	flushed, _ := countValue(counts, "veneur.prometheus.metrics_flushed_total")
	assert.Equal(t, 5, flushed)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	flushed, _ = countValue(counts, "veneur.prometheus.metrics_flushed_total")
	assert.Equal(t, 5, flushed)

	counter2.Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	flushed, _ = countValue(counts, "veneur.prometheus.metrics_flushed_total")
	assert.Equal(t, 5, flushed)
}

func TestCountsOnBridgeRestarts(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	ts, err := testPrometheusEndpoint(
		counter,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	splitStats(stats)

	counter.Inc()
	counter.Inc()
	counter.Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ := splitStats(stats)
	count, _ := countValue(counts, "counter")
	assert.Equal(t, 3, count)

	counter.Inc()
	counter.Inc()
	counter.Inc()

	//cache gets cleared on restarts
	cache = new(countCache)
	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)

	count, _ = countValue(counts, "counter")
	assert.Equal(t, 0, count) //missed it because of our restart

	counter.Inc()
	counter.Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	count, _ = countValue(counts, "counter")
	assert.Equal(t, 2, count) //back on track
}

func TestCountsOnMonitoredServerRestarts(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	ts, err := testPrometheusEndpoint(
		counter,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	splitStats(stats)

	counter.Inc()
	counter.Inc()
	counter.Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ := splitStats(stats)
	count, _ := countValue(counts, "counter")
	assert.Equal(t, 3, count)

	counter.Inc()
	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	count, _ = countValue(counts, "counter")
	assert.Equal(t, 1, count)

	//gonna lose these in the restart
	counter.Inc()
	counter.Inc()
	counter.Inc()
	counter.Inc()

	counter = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	ts, err = testPrometheusEndpoint(
		counter,
	)
	require.NoError(t, err)
	defer ts.Close()

	//these will show up as they are post restart
	counter.Inc()
	counter.Inc()

	cfg = testConfig(t, ts.URL)
	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	count, _ = countValue(counts, "counter")
	assert.Equal(t, 2, count)
}

func TestHistogramsNewBucketsAreTranslatedToDiffs(t *testing.T) {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram",
		Help:    "A typical histogram.",
		Buckets: []float64{1, 2, 5},
	})

	ts, err := testPrometheusEndpoint(
		histogram,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := ignoreConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	splitStats(stats)

	histogram.Observe(1.5)
	histogram.Observe(2.3)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges := splitStats(stats)

	sum, _ := gaugeValue(gauges, "histogram.sum")
	count, _ := countValue(counts, "histogram.count")
	bucket1, _ := countValue(counts, "histogram.le1.000000")
	bucket2, _ := countValue(counts, "histogram.le2.000000")
	bucket5, _ := countValue(counts, "histogram.le5.000000")
	bucketInf, _ := countValue(counts, "histogram.le+Inf")

	assert.Equal(t, 3.8, sum)
	assert.Equal(t, 2, count)
	assert.Equal(t, 0, bucket1)
	assert.Equal(t, 1, bucket2)
	assert.Equal(t, 2, bucket5)
	assert.Equal(t, 2, bucketInf)

	histogram = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram",
		Help:    "A typical histogram.",
		Buckets: []float64{1, 1.75, 2, 6},
	})

	ts, err = testPrometheusEndpoint(
		histogram,
	)
	require.NoError(t, err)
	defer ts.Close()
	metricsHostUrl, err := url.Parse(ts.URL)
	require.NoError(t, err)
	cfg.metricsHost = metricsHostUrl

	histogram.Observe(1.3)
	histogram.Observe(2.3)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges = splitStats(stats)

	sum, _ = gaugeValue(gauges, "histogram.sum")
	count, _ = countValue(counts, "histogram.count")
	bucket1, _ = countValue(counts, "histogram.le1.000000")
	bucket175, _ := countValue(counts, "histogram.le1.750000")
	bucket2, _ = countValue(counts, "histogram.le2.000000")
	_, hasbucket5 := countValue(counts, "histogram.le5.000000")
	bucket6, _ := countValue(counts, "histogram.le6.000000")
	bucketInf, _ = countValue(counts, "histogram.le+Inf")

	//prom reports some goofy numbers at times
	assert.True(t, sum < 4) //note this is 'wrong' in that we lost the real sum

	assert.Equal(t, 0, bucket1)
	assert.Equal(t, 1, bucket175)
	assert.Equal(t, 2, bucket6, fmt.Sprintf("%v", counts))

	//these counts are the same as the previous observation, even though a restart
	//edge case where we lose data we lost it
	assert.Equal(t, 0, count)
	assert.Equal(t, 0, bucket2)
	assert.Equal(t, 0, bucketInf, fmt.Sprintf("%v", counts))

	assert.False(t, hasbucket5)
}

func TestHistogramsAreTranslatedToDiffs(t *testing.T) {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram",
		Help:    "A typical histogram.",
		Buckets: []float64{1, 2, 5},
	})

	histogramVec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "histogram_vec",
		Help:    "A typical histogram vec with labels",
		Buckets: []float64{1, 2, 5},
	},
		[]string{"g", "h"},
	)

	ts, err := testPrometheusEndpoint(
		histogram,
		histogramVec,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := ignoreConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges := splitStats(stats)

	assert.True(t, gaugeReceived(gauges, "histogram.sum"))
	assert.True(t, countReceived(counts, "histogram.count"))
	assert.True(t, countReceived(counts, "histogram.le1.000000"))
	assert.True(t, countReceived(counts, "histogram.le2.000000"))
	assert.True(t, countReceived(counts, "histogram.le5.000000"))
	assert.True(t, countReceived(counts, "histogram.le+Inf"))

	assert.False(t, gaugeReceived(gauges, "histogram_vec.sum", "g:G1", "h:H1"))
	assert.False(t, countReceived(counts, "histogram_vec.count", "g:G1", "h:H1"))
	assert.False(t, countReceived(counts, "histogram_vec.le1.000000", "g:G1", "h:H1"))
	assert.False(t, countReceived(counts, "histogram_vec.le2.000000", "g:G1", "h:H1"))
	assert.False(t, countReceived(counts, "histogram_vec.le5.000000", "g:G1", "h:H1"))
	assert.False(t, countReceived(counts, "histogram_vec.le+Inf", "g:G1", "h:H1"))

	sum, _ := gaugeValue(gauges, "histogram.sum")
	count, _ := countValue(counts, "histogram.count")
	bucket1, _ := countValue(counts, "histogram.le1.000000")
	bucket2, _ := countValue(counts, "histogram.le2.000000")
	bucket5, _ := countValue(counts, "histogram.le5.000000")
	bucketInf, _ := countValue(counts, "histogram.le+Inf")

	assert.Equal(t, 0.0, sum)
	assert.Equal(t, 0, count)
	assert.Equal(t, 0, bucket1)
	assert.Equal(t, 0, bucket2)
	assert.Equal(t, 0, bucket5)
	assert.Equal(t, 0, bucketInf)

	histogram.Observe(3)
	histogramVec.WithLabelValues("G1", "H1").Observe(1.5)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges = splitStats(stats)

	sum, _ = gaugeValue(gauges, "histogram.sum")
	count, _ = countValue(counts, "histogram.count")
	bucket1, _ = countValue(counts, "histogram.le1.000000")
	bucket2, _ = countValue(counts, "histogram.le2.000000")
	bucket5, _ = countValue(counts, "histogram.le5.000000")
	bucketInf, _ = countValue(counts, "histogram.le+Inf")

	assert.Equal(t, 3.0, sum)
	assert.Equal(t, 1, count)
	assert.Equal(t, 0, bucket1) //this stayed zero
	assert.Equal(t, 0, bucket2) //this staed zero
	assert.Equal(t, 1, bucket5)
	assert.Equal(t, 1, bucketInf)

	//sum shows up because it is a gauge. don't have diffs for the rest
	sum, _ = gaugeValue(gauges, "histogram_vec.sum", "g:G1", "h:H1")
	assert.Equal(t, 1.5, sum)

	sum, _ = gaugeValue(gauges, "histogram_vec.sum", "g:G1", "h:H1")
	count, _ = countValue(counts, "histogram_vec.count", "g:G1", "h:H1")
	bucket1, _ = countValue(counts, "histogram_vec.le1.000000", "g:G1", "h:H1")
	bucket2, _ = countValue(counts, "histogram_vec.le2.000000", "g:G1", "h:H1")
	bucket5, _ = countValue(counts, "histogram_vec.le5.000000", "g:G1", "h:H1")
	bucketInf, _ = countValue(counts, "histogram_vec.le+Inf", "g:G1", "h:H1")

	assert.Equal(t, 1.5, sum)
	assert.Equal(t, 1, count)
	assert.Equal(t, 0, bucket1)
	assert.Equal(t, 1, bucket2)
	assert.Equal(t, 1, bucket5)
	assert.Equal(t, 1, bucketInf)

	histogram.Observe(0.5)
	histogram.Observe(4.5)
	histogramVec.WithLabelValues("G1", "H1").Observe(6)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges = splitStats(stats)

	sum, _ = gaugeValue(gauges, "histogram.sum")
	count, _ = countValue(counts, "histogram.count")
	bucket1, _ = countValue(counts, "histogram.le1.000000")
	bucket2, _ = countValue(counts, "histogram.le2.000000")
	bucket5, _ = countValue(counts, "histogram.le5.000000")
	bucketInf, _ = countValue(counts, "histogram.le+Inf")

	assert.Equal(t, 8.0, sum)
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, bucket1)
	assert.Equal(t, 1, bucket2)
	assert.Equal(t, 2, bucket5)
	assert.Equal(t, 2, bucketInf)

	sum, _ = gaugeValue(gauges, "histogram_vec.sum", "g:G1", "h:H1")
	count, _ = countValue(counts, "histogram_vec.count", "g:G1", "h:H1")
	bucket1, _ = countValue(counts, "histogram_vec.le1.000000", "g:G1", "h:H1")
	bucket2, _ = countValue(counts, "histogram_vec.le2.000000", "g:G1", "h:H1")
	bucket5, _ = countValue(counts, "histogram_vec.le5.000000", "g:G1", "h:H1")
	bucketInf, _ = countValue(counts, "histogram_vec.le+Inf", "g:G1", "h:H1")

	assert.Equal(t, 7.5, sum)
	assert.Equal(t, 1, count)
	assert.Equal(t, 0, bucket1)
	assert.Equal(t, 0, bucket2)
	assert.Equal(t, 0, bucket5)
	assert.Equal(t, 1, bucketInf)
}

func TestSummariesCountAreTranslatedToDiffs(t *testing.T) {
	summary := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "summary",
		Help:       "A typical summary.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})

	summaryVec := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       "summary_vec",
		Help:       "A typical summary vec with labels",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	},
		[]string{"e", "f"},
	)

	ts, err := testPrometheusEndpoint(
		summary,
		summaryVec,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges := splitStats(stats)

	count, _ := countValue(counts, "summary.count")
	sum, _ := gaugeValue(gauges, "summary.sum")
	assert.Equal(t, 0, count)
	assert.Equal(t, 0.0, sum)

	//doesnt show up until there are values
	assert.False(t, gaugeReceived(gauges, "summary.50percentile"))

	//nothing shows for vecs until they have some values assigned
	assert.False(t, gaugeReceived(gauges, "summary_vec.50percentile", "e:E1", "f:F1"))
	assert.False(t, gaugeReceived(gauges, "summary_vec.sum", "e:E1", "f:F1"))
	assert.False(t, countReceived(counts, "summary_vec.count", "e:E1", "f:F1"))

	summary.Observe(30)
	summaryVec.WithLabelValues("E1", "F1").Observe(30)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges = splitStats(stats)

	count, _ = countValue(counts, "summary.count")
	sum, _ = gaugeValue(gauges, "summary.sum")
	perc50, _ := gaugeValue(gauges, "summary.50percentile")
	perc90, _ := gaugeValue(gauges, "summary.90percentile")
	assert.Equal(t, 1, count)
	assert.Equal(t, 30.0, sum)
	assert.Equal(t, 30.0, perc50)
	assert.Equal(t, 30.0, perc90)

	count, _ = countValue(counts, "summary_vec.count", "e:E1", "f:F1")
	sum, _ = gaugeValue(gauges, "summary_vec.sum", "e:E1", "f:F1")
	perc50, _ = gaugeValue(gauges, "summary_vec.50percentile", "e:E1", "f:F1")
	perc90, _ = gaugeValue(gauges, "summary_vec.90percentile", "e:E1", "f:F1")

	assert.Equal(t, 1, count)
	assert.Equal(t, 30.0, sum)
	assert.Equal(t, 30.0, perc50)
	assert.Equal(t, 30.0, perc90)

	summary.Observe(60)
	summary.Observe(60)

	summaryVec.WithLabelValues("E1", "F1").Observe(60)
	summaryVec.WithLabelValues("E1", "F1").Observe(60)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges = splitStats(stats)

	count, _ = countValue(counts, "summary.count")
	sum, _ = gaugeValue(gauges, "summary.sum")
	perc50, _ = gaugeValue(gauges, "summary.50percentile")
	perc90, _ = gaugeValue(gauges, "summary.90percentile")

	assert.Equal(t, 2, count)
	assert.Equal(t, 150.0, sum)
	assert.Equal(t, 60.0, perc50)
	assert.Equal(t, 60.0, perc90)

	count, _ = countValue(counts, "summary_vec.count", "e:E1", "f:F1")
	sum, _ = gaugeValue(gauges, "summary_vec.sum", "e:E1", "f:F1")
	perc50, _ = gaugeValue(gauges, "summary_vec.50percentile", "e:E1", "f:F1")
	perc90, _ = gaugeValue(gauges, "summary_vec.90percentile", "e:E1", "f:F1")

	assert.Equal(t, 2, count)
	assert.Equal(t, 150.0, sum)
	assert.Equal(t, 60.0, perc50)
	assert.Equal(t, 60.0, perc90)
}

func TestGaugesAreNOTTranslatedToDiffs(t *testing.T) {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gauge",
		Help: "A typical gauge",
	})

	gaugeVec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "gauge_vec",
		Help: "A typical guage vec with labels",
	},
		[]string{"c", "d"},
	)

	ts, err := testPrometheusEndpoint(
		gauge,
		gaugeVec,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	_, gauges := splitStats(stats)

	//vecs dont show up until they have a call
	assert.False(t, gaugeReceived(gauges, "gauge_vec", "c:C1", "d:D1"))
	val, _ := gaugeValue(gauges, "gauge")
	assert.Equal(t, 0.0, val)

	gauge.Set(3.4)
	gaugeVec.WithLabelValues("C1", "D1").Set(29.9)
	gaugeVec.WithLabelValues("C1", "D1").Set(27.8)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	_, gauges = splitStats(stats)
	val1, _ := gaugeValue(gauges, "gauge")
	val2, _ := gaugeValue(gauges, "gauge_vec", "c:C1", "d:D1")

	assert.Equal(t, 3.4, val1)
	assert.Equal(t, 27.8, val2)
}

func TestCountsAreTranslatedToDiffs(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "counter_vec",
		Help: "A typical counter vec with labels"},
		[]string{"a", "b"},
	)

	ts, err := testPrometheusEndpoint(
		counter,
		counterVec,
	)
	require.NoError(t, err)
	defer ts.Close()

	cache := new(countCache)
	cfg := testConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ := splitStats(stats)
	//won't show up in prom endpoint until its inc'd
	assert.False(t, countReceived(counts, "counter_vec", "a:A1", "b:B1"))

	counter.Inc()
	counterVec.WithLabelValues("A1", "B1").Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)
	count1, _ := countValue(counts, "counter")
	count2, _ := countValue(counts, "counter_vec", "a:A1", "b:B1")
	assert.Equal(t, 1, count1) //0 from init, 1 from inc, diff is 1
	assert.Equal(t, 1, count2) //new metric can take all of its count

	counter.Inc()
	counterVec.WithLabelValues("A1", "B1").Inc()
	counterVec.WithLabelValues("A2", "B1").Inc()

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)

	count1, _ = countValue(counts, "counter")
	count2, _ = countValue(counts, "counter_vec", "a:A1", "b:B1")
	count3, _ := countValue(counts, "counter_vec", "a:A2", "b:B1")
	assert.Equal(t, 1, count1) //0 from init, 1 from inc, diff is 1
	assert.Equal(t, 1, count2)
	//a new metric, but we've had observations before so we can assume its all new
	assert.Equal(t, 1, count3)

	counter.Add(5)
	counterVec.WithLabelValues("A1", "B1").Add(5)
	counterVec.WithLabelValues("A2", "B1").Add(5)

	stats, err = collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, _ = splitStats(stats)

	//all got 5 between last
	count1, _ = countValue(counts, "counter")
	count2, _ = countValue(counts, "counter_vec", "a:A1", "b:B1")
	count3, _ = countValue(counts, "counter_vec", "a:A2", "b:B1")
	assert.Equal(t, 5, count1)
	assert.Equal(t, 5, count2)
	assert.Equal(t, 5, count3)
}

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

	cache := new(countCache)
	cfg := ignoreConfig(t, ts.URL)
	statsClient, err := statsd.New("localhost:8125", statsd.WithoutTelemetry())
	assert.NoError(t, err)

	stats, err := collect(context.Background(), cfg, statsClient, cache)
	assert.NoError(t, err)
	counts, gauges := splitStats(stats)

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

func splitStats(stats <-chan []statsdStat) ([]statsdCount, []gauge) {
	var c []statsdCount
	var g []gauge
	for batch := range stats {
		for _, s := range batch {
			switch i := s.(type) {
			case statsdCount:
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

func gaugeValue(gauges []gauge, name string, tags ...string) (float64, bool) {
	b := statID{name, tags}
	for _, a := range gauges {
		if same(a.statID, b) {
			return a.Value, true
		}
	}

	return 0, false
}

func gaugeReceived(gauges []gauge, name string, tags ...string) bool {
	_, found := gaugeValue(gauges, name, tags...)
	return found
}

func countValue(counts []statsdCount, name string, tags ...string) (int, bool) {
	b := statID{name, tags}
	for _, a := range counts {
		if same(a.statID, b) {
			//our tests won't have int64 in them but prod might
			return int(a.Value), true
		}
	}

	return 0, false
}

func countReceived(counts []statsdCount, name string, tags ...string) bool {
	_, found := countValue(counts, name, tags...)
	return found
}

func testConfig(t *testing.T, metricsHost string) *prometheusConfig {
	c, err := newHTTPClient("", "", "", "")
	require.NoError(t, err)

	metricsHostUrl, err := url.Parse(metricsHost)
	require.NoError(t, err)

	return &prometheusConfig{
		metricsHost:    metricsHostUrl,
		httpClient:     c,
		ignoredLabels:  getIgnoredFromArg(""),
		ignoredMetrics: getIgnoredFromArg("promhttp_(.*)"),
	}
}

func ignoreConfig(t *testing.T, metricsHost string) *prometheusConfig {
	c, err := newHTTPClient("", "", "", "")
	require.NoError(t, err)

	metricsHostUrl, err := url.Parse(metricsHost)
	require.NoError(t, err)

	return &prometheusConfig{
		metricsHost:    metricsHostUrl,
		httpClient:     c,
		ignoredLabels:  getIgnoredFromArg("ignore"),
		ignoredMetrics: getIgnoredFromArg("(.*)_ignore,promhttp_(.*)"),
	}
}
