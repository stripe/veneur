package openmetrics

import (
	"context"
	"io"
	"net/http"
	"regexp"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
)

type PrometheusResults struct {
	MetricFamily dto.MetricFamily
	Error        error
}

func Query(
	ctx context.Context, httpClient *http.Client, host string,
	ignoredMetrics []*regexp.Regexp,
) (<-chan PrometheusResults, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", host, nil)
	if err != nil {
		return nil, err
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	decoder := expfmt.NewDecoder(response.Body, expfmt.FmtText)

	metrics := make(chan PrometheusResults)
	go func() {
		defer close(metrics)

		for {
			var metricFamily dto.MetricFamily
			err := decoder.Decode(&metricFamily)
			if err == io.EOF {
				return
			} else if err != nil {
				metrics <- PrometheusResults{Error: err}
				logrus.
					WithError(err).
					WithField("prometheus_host", host).
					Warn("decode error")
				return
			}

			if !shouldExportMetric(metricFamily, ignoredMetrics) {
				continue
			}

			metrics <- PrometheusResults{MetricFamily: metricFamily}
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
