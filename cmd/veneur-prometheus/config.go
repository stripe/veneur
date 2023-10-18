package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type prometheusConfig struct {
	metricsHost    *url.URL
	httpClient     *http.Client
	ignoredLabels  *regexp.Regexp
	ignoredMetrics *regexp.Regexp
	addedLabels    map[string]string
	renameLabels   map[string]string
	interval       time.Duration
}

func prometheusConfigFromArguments() (*prometheusConfig, error) {
	client, err := newHTTPClient(*socket, *cert, *key, *caCert)

	metricsHostUrl, err := url.Parse(*metricsHost)
	if err != nil {
		return nil, err
	}

	parsedInterval, err := time.ParseDuration(*interval)
	if err != nil {
		return nil, err
	}

	return &prometheusConfig{
		metricsHost:    metricsHostUrl,
		httpClient:     client,
		ignoredLabels:  getIgnoredFromArg(*ignoredLabelsStr),
		ignoredMetrics: getIgnoredFromArg(*ignoredMetricsStr),
		addedLabels:    getAddedFromArg(*addLabelsStr),
		renameLabels:   getRenamedFromArg(*renameLabelsStr),
		interval:       parsedInterval,
	}, err
}

func getIgnoredFromArg(arg string) *regexp.Regexp {
	if arg == "" {
		return nil
	}
	return regexp.MustCompile(strings.Replace(arg, ",", "|", -1))
}

func getRenamedFromArg(arg string) map[string]string {
	renamed := make(map[string]string)
	for _, renameStr := range strings.Split(arg, ",") {
		if len(renameStr) <= 0 {
			continue
		}

		parts := strings.SplitN(renameStr, "=", 2)
		if len(parts) == 2 {
			renamed[parts[0]] = parts[1]
		}
	}

	return renamed
}

func getAddedFromArg(arg string) map[string]string {
	added := make(map[string]string)
	for _, addStr := range strings.Split(arg, ",") {
		if len(addStr) <= 0 {
			continue
		}

		parts := strings.SplitN(addStr, "=", 2)
		if len(parts) == 2 {
			added[parts[0]] = parts[1]
		}
	}

	return added
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
			return nil, fmt.Errorf("error reading client cert and key: %s", err)
		}
		clientCerts = append(clientCerts, clientCert)
	}

	if caCertPath != "" {
		caCertPool = x509.NewCertPool()
		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("error reading ca cert: %s", err)
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
