package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"
)

type prometheusConfig struct {
	metricsHost    string
	httpClient     *http.Client
	ignoredLabels  []*regexp.Regexp
	ignoredMetrics []*regexp.Regexp
}

func prometheusConfigFromArguments() prometheusConfig {

	return prometheusConfig{
		metricsHost:    *metricsHost,
		httpClient:     newHTTPClient(*cert, *key, *caCert),
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
