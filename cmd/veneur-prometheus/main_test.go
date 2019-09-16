package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
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

func TestPrometheusCounterIsEverIncreasing(t *testing.T) {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
		Help: "A typical counter.",
	})

	err := prometheus.Register(counter)
	require.NoError(t, err)

	ts := httptest.NewServer(promhttp.Handler())
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
