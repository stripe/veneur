package main

import (
	"context"
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
}

func QueryPrometheus(
	ctx context.Context, httpClient *http.Client, host string,
	ignoredMetrics []*regexp.Regexp,
) (<-chan prometheusResults, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", host, nil)
	if err != nil {
		return nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	decoder := expfmt.NewDecoder(response.Body, expfmt.FmtText)

	metrics := make(chan prometheusResults)
	go func() {
		defer close(metrics)

		for {
			var mf dto.MetricFamily
			err := decoder.Decode(&mf)
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

	return metrics, nil
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
