package main

import (
	"io"
	"net/http"
	"regexp"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
)

type prometheusResults struct {
	mf          dto.MetricFamily
	decodeError error
	clientError error
}

func queryPrometheus(httpClient *http.Client, host string, ignoredMetrics []*regexp.Regexp) <-chan prometheusResults {
	metrics := make(chan prometheusResults)

	go func() {
		defer close(metrics)

		resp, err := httpClient.Get(host)
		if err != nil {
			metrics <- prometheusResults{clientError: err}
			logrus.
				WithError(err).
				WithField("prometheus_host", host).
				Warn("unable to connect with prometheus host")

			return
		}

		d := expfmt.NewDecoder(resp.Body, expfmt.FmtText)
		for {
			var mf dto.MetricFamily
			err := d.Decode(&mf)
			if err == io.EOF {
				// We've hit the end, break out!
				return
			} else if err != nil {
				metrics <- prometheusResults{decodeError: err}
				logrus.
					WithError(err).
					WithField("prometheus_host", host).
					Warn("decode error")
				return
			}

			if !shouldExportMetric(mf, ignoredMetrics) {
				continue
			}

			metrics <- prometheusResults{mf: mf}
		}
	}()

	return metrics
}

func shouldExportMetric(mf dto.MetricFamily, ignoredMetrics []*regexp.Regexp) bool {
	for _, ignoredMetric := range ignoredMetrics {
		metricName := mf.GetName()

		if ignoredMetric.MatchString(metricName) {
			return false
		}
	}

	return true
}
