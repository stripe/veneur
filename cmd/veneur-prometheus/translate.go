package main

import (
	"fmt"
	"math"
	"regexp"

	dto "github.com/prometheus/client_model/go"
	"github.com/stripe/veneur/v14/sources/openmetrics"
)

var (
	decodeError = newStatsdCount("veneur.prometheus.decode_errors_total", nil, 1)

	unknownPrometheusTypeID = statID{"veneur.prometheus.unknown_metric_type_total", nil}
	flushedMetricsID        = statID{"veneur.prometheus.metrics_flushed_total", nil}
)

func translatePrometheus(
	cfg *prometheusConfig, cache *countCache,
	prometheus <-chan openmetrics.QueryResults,
) <-chan []statsdStat {
	statsd := make(chan []statsdStat)
	s := sender{statsd, cache}
	t := translator{
		ignored: cfg.ignoredLabels,
		added:   cfg.addedLabels,
		renamed: cfg.renameLabels,
	}
	go sendTranslated(prometheus, t, s)

	return statsd
}

func sendTranslated(
	prometheus <-chan openmetrics.QueryResults, translate translator, s sender,
) {
	count := int64(0)
	unknown := int64(0)

	for result := range prometheus {
		var stats []inMemoryStat

		if result.Error != nil {
			count++
			s.statsd(decodeError)
			continue
		}

		mf := result.MetricFamily
		switch mf.GetType() {
		case dto.MetricType_COUNTER:
			stats = translate.PrometheusCounter(mf)
		case dto.MetricType_GAUGE:
			stats = translate.PrometheusGauge(mf)
		case dto.MetricType_SUMMARY:
			stats = translate.PrometheusSummary(mf)
		case dto.MetricType_HISTOGRAM:
			stats = translate.PrometheusHistogram(mf)
		case dto.MetricType_UNTYPED:
			stats = translate.PrometheusUntyped(mf)
		default:
			unknown++
		}

		count += int64(len(stats))
		if stats != nil {
			s.send(stats...)
		}
	}

	s.statsd(
		statsdCount{unknownPrometheusTypeID, unknown},
		statsdCount{flushedMetricsID, count + 2},
	)

	s.Close()
}

type sender struct {
	ch    chan<- []statsdStat
	cache *countCache
}

func (s sender) statsd(stats ...statsdStat) {
	s.ch <- stats
}

func (s sender) send(stats ...inMemoryStat) {
	var statsd []statsdStat
	for _, inmemory := range stats {
		statsd = append(statsd, inmemory.Translate(s.cache))
	}

	s.statsd(statsd...)
}

func (s sender) Close() {
	close(s.ch)
	s.cache.Done()
}

type translator struct {
	ignored *regexp.Regexp
	renamed map[string]string
	added   map[string]string
}

func (t translator) PrometheusCounter(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, counter := range mf.GetMetric() {
		tags := t.Tags(counter.GetLabel())
		stats = append(stats, newPrometheusCount(mf.GetName(), tags, int64(counter.GetCounter().GetValue())))
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

func (t translator) PrometheusUntyped(mf dto.MetricFamily) []inMemoryStat {
	var stats []inMemoryStat
	for _, untyped := range mf.GetMetric() {
		tags := t.Tags(untyped.GetLabel())
		stats = append(stats, newGauge(mf.GetName(), tags, float64(untyped.GetUntyped().GetValue())))
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
		stats = append(stats, newPrometheusCount(fmt.Sprintf("%s.count", name), tags, int64(data.GetSampleCount())))

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
		stats = append(stats, newPrometheusCount(fmt.Sprintf("%s.count", name), tags, int64(data.GetSampleCount())))

		for _, bucket := range data.GetBucket() {
			b := bucket.GetUpperBound()
			if !math.IsNaN(b) {
				stats = append(stats,
					newPrometheusCount(
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

		if t.ignored != nil && t.ignored.MatchString(labelName) {
			continue
		}

		if newName, found := t.renamed[labelName]; found {
			labelName = newName
		}

		tags = append(tags, fmt.Sprintf("%s:%s", labelName, labelValue))
	}

	for name, value := range t.added {
		tags = append(tags, fmt.Sprintf("%s:%s", name, value))
	}

	return tags
}
