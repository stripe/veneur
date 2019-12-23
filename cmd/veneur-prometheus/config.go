package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"regexp"
	"strings"
)

type prometheusConfig struct {
	metricsHost    string
	httpClient     *http.Client
	ignoredLabels  []*regexp.Regexp
	ignoredMetrics []*regexp.Regexp
}

func prometheusConfigFromArguments() (prometheusConfig, error) {
	client, err := newHTTPClient(*socket, *cert, *key, *caCert)

	return prometheusConfig{
		metricsHost:    *metricsHost,
		httpClient:     client,
		ignoredLabels:  getIgnoredFromArg(*ignoredLabelsStr),
		ignoredMetrics: getIgnoredFromArg(*ignoredMetricsStr),
	}, err
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

func newHTTPClient(socket, certPath, keyPath, caCertPath string) (*http.Client, error) {

	var transport http.RoundTripper
	var err error
	if socket != "" {
		transport = NewUnixTransport(socket)
	} else {
		transport, err = httpTransport(certPath, keyPath, caCertPath)
	}

	if err != nil {
		return nil, err
	}

	return &http.Client{Transport: transport}, nil
}

func httpTransport(certPath, keyPath, caCertPath string) (http.RoundTripper, error) {
	var caCertPool *x509.CertPool
	var clientCerts []tls.Certificate

	if certPath != "" {
		clientCert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("error reading client cert and key: %w", err)
		}
		clientCerts = append(clientCerts, clientCert)
	}

	if caCertPath != "" {
		caCertPool = x509.NewCertPool()
		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("error reading ca cert: %w", err)
		}
		caCertPool.AppendCertsFromPEM(caCert)
	}

	return &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: clientCerts,
			RootCAs:      caCertPool,
		},
	}, nil
}
