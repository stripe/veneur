package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestPrometheusCounterIsEverIncreasing(t *testing.T) {

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	ts, err := testPrometheusEndpoint(counter)
	require.NoError(t, err)
	defer ts.Close()

	count, err := getCount(ts.URL, "counter")
	require.NoError(t, err)
	assert.Equal(t, 0.0, count)

	counter.Inc()

	count, err = getCount(ts.URL, "counter")
	require.NoError(t, err)
	assert.Equal(t, 1.0, count)

	counter.Inc()

	count, err = getCount(ts.URL, "counter")
	require.NoError(t, err)
	assert.Equal(t, 2.0, count)
}

func testPrometheusEndpoint(collectors ...prometheus.Collector) (*httptest.Server, error) {
	registry := prometheus.NewRegistry()
	for _, collector := range collectors {
		err := registry.Register(collector)
		if err != nil {
			return nil, err
		}
	}

	ts := httptest.NewServer(
		promhttp.InstrumentMetricHandler(registry, promhttp.HandlerFor(registry, promhttp.HandlerOpts{})))

	return ts, nil
}

func getCount(url string, name string) (float64, error) {
	res, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	d := expfmt.NewDecoder(res.Body, expfmt.FmtText)
	var mf dto.MetricFamily
	for {
		err := d.Decode(&mf)
		if err == io.EOF {
			return 0, fmt.Errorf("did not find count")
		} else if err != nil {
			return 0, err
		}

		if mf.GetName() == name {
			for _, counter := range mf.GetMetric() {
				return counter.GetCounter().GetValue(), nil
			}
		}
	}
}
