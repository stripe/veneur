package main

import (
	"fmt"
	"math"
	"regexp"

	dto "github.com/prometheus/client_model/go"
)

func translatePrometheus(ignoredLabels []*regexp.Regexp, prometheus <-chan prometheusResults) <-chan []inMemoryStat {
	inmemory := make(chan []inMemoryStat)
	go sendTranslated(prometheus, ignoredLabels, inmemory)

	return inmemory
}

func sendTranslated(prometheus <-chan prometheusResults, translate translator, s sendor) {

	count := int64(0)
	unknown := int64(0)

	for result := range prometheus {
		var stats []inMemoryStat

		if result.clientError != nil {
			count++
			s.send(newCount("veneur.prometheus.connect_errors_total", nil, 1))
			continue
		}

		if result.decodeError != nil {
			count++
			s.send(newCount("veneur.prometheus.decode_errors_total", nil, 1))
			continue
		}

		mf := result.mf
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			stats = translate.PrometheusCounter(mf)
		case dto.MetricType_GAUGE:
			stats = translate.PrometheusGauge(mf)
		case dto.MetricType_SUMMARY:
			stats = translate.PrometheusSummary(mf)
		case dto.MetricType_HISTOGRAM:
			stats = translate.PrometheusHistogram(mf)
		default:
			unknown++
		}

		count += int64(len(stats))
		if stats != nil {
			s.send(stats...)
		}
	}

	s.send(
		newCount("veneur.prometheus.unknown_metric_type_total", nil, unknown),
		newCount("veneur.prometheus.metrics_flushed_total", nil, count+2),
	)

	s.Close()
}

type sendor chan<- []inMemoryStat

func (s sendor) send(stats ...inMemoryStat) {
	s <- stats
}

func (s sendor) Close() {
	close(s)
}

type translator []*regexp.Regexp

func (t translator) PrometheusCounter(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, counter := range mf.GetMetric() {
		tags := t.Tags(counter.GetLabel())
		stats = append(stats, newCount(mf.GetName(), tags, int64(counter.GetCounter().GetValue())))
	}
	return stats
}

func (t translator) PrometheusGauge(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, gauge := range mf.GetMetric() {
		tags := t.Tags(gauge.GetLabel())
		stats = append(stats, newGauge(mf.GetName(), tags, float64(gauge.GetGauge().GetValue())))
	}
	return stats
}

func (t translator) PrometheusSummary(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, summary := range mf.GetMetric() {
		tags := t.Tags(summary.GetLabel())
		name := mf.GetName()
		data := summary.GetSummary()

		stats = append(stats, newGauge(fmt.Sprintf("%s.sum", name), tags, data.GetSampleSum()))
		stats = append(stats, newCount(fmt.Sprintf("%s.count", name), tags, int64(data.GetSampleCount())))

		for _, quantile := range data.GetQuantile() {
			v := quantile.GetValue()
			if !math.IsNaN(v) {
				stats = append(stats,
					newGauge(
						fmt.Sprintf("%s.%dpercentile", name, int(quantile.GetQuantile()*100)),
						tags,
						v))
			}
		}
	}

	return stats
}

func (t translator) PrometheusHistogram(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, histo := range mf.GetMetric() {
		tags := t.Tags(histo.GetLabel())
		name := mf.GetName()
		data := histo.GetHistogram()

		stats = append(stats, newGauge(fmt.Sprintf("%s.sum", name), tags, data.GetSampleSum()))
		stats = append(stats, newCount(fmt.Sprintf("%s.count", name), tags, int64(data.GetSampleCount())))

		for _, bucket := range data.GetBucket() {
			b := bucket.GetUpperBound()
			if !math.IsNaN(b) {
				stats = append(stats,
					newCount(
						fmt.Sprintf("%s.le%f", name, b),
						tags,
						int64(bucket.GetCumulativeCount())))
			}
		}
	}

	return stats
}

func (t translator) Tags(labels []*dto.LabelPair) []string {
	var tags []string

	for _, pair := range labels {
		labelName := pair.GetName()
		labelValue := pair.GetValue()
		include := true

		for _, ignoredLabel := range t {
			if ignoredLabel.MatchString(labelName) {
				include = false
				break
			}
		}

		if include {
			tags = append(tags, fmt.Sprintf("%s:%s", labelName, labelValue))
		}
	}

	return tags
}
