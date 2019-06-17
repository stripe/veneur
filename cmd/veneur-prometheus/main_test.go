package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

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
