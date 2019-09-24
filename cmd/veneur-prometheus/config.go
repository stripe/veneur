package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-go/statsd"
	"github.com/sirupsen/logrus"
)

type config struct {
	metricsHost    string
	statsHost      string
	httpClient     *http.Client
	statsClient    statsC
	ignoredLabels  []*regexp.Regexp
	ignoredMetrics []*regexp.Regexp
}

func configFromArgs() config {
	statsClient, _ := statsd.New(*statsHost)

	if *prefix != "" {
		statsClient.Namespace = *prefix
	}

	return config{
		metricsHost:    *metricsHost,
		statsHost:      *statsHost,
		httpClient:     newHTTPClient(*cert, *key, *caCert),
		statsClient:    statsClient,
		ignoredLabels:  getIgnoredFromArg(*ignoredLabelsStr),
		ignoredMetrics: getIgnoredFromArg(*ignoredMetricsStr),
	}
}

func getIgnoredFromArg(arg string) []*regexp.Regexp {
	ignore := []*regexp.Regexp{}
	for _, ignoreStr := range strings.Split(arg, ",") {
		if len(ignoreStr) > 0 {
			ignore = append(ignore, regexp.MustCompile(ignoreStr))
		}
	}

	return ignore
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
