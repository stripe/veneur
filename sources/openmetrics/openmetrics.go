package openmetrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sources"
	"github.com/stripe/veneur/v14/util"
)

type OpenMetricsSourceConfig struct {
	HistogramBucketTag string        `yaml:"histogram_bucket_tag"`
	IgnoredMetrics     util.Regexp   `yaml:"ignored_metrics"`
	ScrapeInterval     time.Duration `yaml:"scrape_interval"`
	ScrapeTarget       util.Url      `yaml:"scrape_target"`
	ScrapeTimeout      time.Duration `yaml:"scrape_timeout"`
	SummaryQuantileTag string        `yaml:"summary_quantile_tag"`
}

// TODO(arnavdugar): Make public fields private once veneur-prometheus is
// removed.
type OpenMetricsSource struct {
	cancelFunc         context.CancelFunc
	context            context.Context
	histogramBucketTag string
	HttpClient         *http.Client
	IgnoredMetrics     *regexp.Regexp
	logger             *logrus.Entry
	name               string
	scrapeInterval     time.Duration
	ScrapeTarget       *url.URL
	ScrapeTimeout      time.Duration
	server             *veneur.Server
	summaryQuantileTag string
}

type QueryResults struct {
	MetricFamily dto.MetricFamily
	Error        error
}

type convertResults struct {
	Metric *samplers.UDPMetric
	Error  error
}

func ParseConfig(
	name string, config interface{},
) (veneur.ParsedSourceConfig, error) {
	sourceConfig := OpenMetricsSourceConfig{}
	err := util.DecodeConfig(name, config, &sourceConfig)
	if err != nil {
		return nil, err
	}

	if sourceConfig.HistogramBucketTag == "" {
		sourceConfig.HistogramBucketTag = "le"
	}
	if sourceConfig.SummaryQuantileTag == "" {
		sourceConfig.SummaryQuantileTag = "quantile"
	}
	if sourceConfig.ScrapeTimeout == 0 {
		sourceConfig.ScrapeTimeout = sourceConfig.ScrapeInterval
	} else if sourceConfig.ScrapeTimeout > sourceConfig.ScrapeInterval {
		return nil, errors.New(
			"scrape timeout cannot be larger than scrape interval")
	}

	return sourceConfig, nil
}

func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	sourceConfig veneur.ParsedSourceConfig,
) (sources.Source, error) {
	openMetricsSourceConfig, ok := sourceConfig.(OpenMetricsSourceConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	return OpenMetricsSource{
		cancelFunc:         cancelFunc,
		context:            ctx,
		histogramBucketTag: openMetricsSourceConfig.HistogramBucketTag,
		HttpClient:         server.HTTPClient,
		IgnoredMetrics:     openMetricsSourceConfig.IgnoredMetrics.Value,
		logger:             logger,
		name:               name,
		scrapeInterval:     openMetricsSourceConfig.ScrapeInterval,
		ScrapeTarget:       openMetricsSourceConfig.ScrapeTarget.Value,
		ScrapeTimeout:      openMetricsSourceConfig.ScrapeTimeout,
		server:             server,
		summaryQuantileTag: openMetricsSourceConfig.SummaryQuantileTag,
	}, nil
}

func (source OpenMetricsSource) Name() string {
	return source.name
}

func (source OpenMetricsSource) Start() error {
	ticker := time.NewTicker(source.scrapeInterval)
intervalLoop:
	for {
		select {
		case t := <-ticker.C:
			results, err := func() (<-chan QueryResults, error) {
				ctx, cancel :=
					context.WithDeadline(source.context, t.Add(source.ScrapeTimeout))
				defer cancel()
				return source.Query(ctx)
			}()

			if err != nil {
				source.logger.WithError(err).Warn("failed to query metrics")
				continue
			}
			udpMetrics := source.Convert(results)
			for metric := range udpMetrics {
				if metric.Error != nil {
					source.logger.WithError(err).Warn("failed to ingest metrics")
					continue intervalLoop
				}
				source.server.IngestMetric(metric.Metric)
			}
		case <-source.context.Done():
			break intervalLoop
		}
	}
	ticker.Stop()
	return nil
}

func (source OpenMetricsSource) Stop() {
	if source.cancelFunc == nil {
		return
	}
	source.cancelFunc()
}

func (source *OpenMetricsSource) Query(
	ctx context.Context,
) (<-chan QueryResults, error) {
	request, err :=
		http.NewRequestWithContext(ctx, "GET", source.ScrapeTarget.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := source.HttpClient.Do(request)
	if err != nil {
		return nil, err
	}
	decoder := expfmt.NewDecoder(response.Body, expfmt.FmtText)

	metrics := make(chan QueryResults)
	go func() {
		defer close(metrics)

		for {
			var metricFamily dto.MetricFamily
			err := decoder.Decode(&metricFamily)
			if err == io.EOF {
				return
			} else if err != nil {
				metrics <- QueryResults{Error: err}
				logrus.
					WithError(err).
					WithField("host", source.ScrapeTarget).
					Warn("decode error")
				return
			}

			if source.IgnoredMetrics != nil &&
				source.IgnoredMetrics.MatchString(*metricFamily.Name) {
				continue
			}

			metrics <- QueryResults{MetricFamily: metricFamily}
		}
	}()
	return metrics, nil
}

func (source *OpenMetricsSource) Convert(
	prometheusResults <-chan QueryResults,
) <-chan *convertResults {
	udpMetrics := make(chan *convertResults)
	go func() {
		defer close(udpMetrics)

		for result := range prometheusResults {
			if result.Error != nil {
				udpMetrics <- &convertResults{
					Error: result.Error,
				}
				break
			}

			metricFamily := result.MetricFamily
			switch *metricFamily.Type {
			case dto.MetricType_COUNTER:
				source.convertCounter(&metricFamily, udpMetrics)
			case dto.MetricType_GAUGE:
				source.convertGauge(&metricFamily, udpMetrics)
			case dto.MetricType_SUMMARY:
				source.convertSummary(&metricFamily, udpMetrics)
			case dto.MetricType_HISTOGRAM:
				source.convertHistogram(&metricFamily, udpMetrics)
			case dto.MetricType_UNTYPED:
				source.convertUntyped(&metricFamily, udpMetrics)
			default:
				continue
			}
		}
	}()
	return udpMetrics
}

func (source *OpenMetricsSource) convertCounter(
	metricFamily *dto.MetricFamily, metrics chan *convertResults,
) {
	for _, metric := range metricFamily.Metric {
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: *metricFamily.Name,
					Type: "counter",
				},
				Tags:      getTags(metric.Label),
				Timestamp: *metric.TimestampMs,
				Value:     metric.Counter.Value,
			},
		}
	}
}

func (source *OpenMetricsSource) convertGauge(
	metricFamily *dto.MetricFamily, metrics chan *convertResults,
) {
	for _, metric := range metricFamily.Metric {
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: *metricFamily.Name,
					Type: "gauge",
				},
				Tags:      getTags(metric.Label),
				Timestamp: *metric.TimestampMs,
				Value:     metric.Gauge.Value,
			},
		}
	}
}

func (source *OpenMetricsSource) convertSummary(
	metricFamily *dto.MetricFamily, metrics chan *convertResults,
) {
	for _, metric := range metricFamily.Metric {
		tags := getTags(metric.Label)
		for _, quantile := range metric.Summary.Quantile {
			summaryTags := make([]string, len(tags))
			copy(summaryTags, tags)
			summaryTags = append(tags, fmt.Sprintf(
				"%s:%f", source.summaryQuantileTag, *quantile.Quantile))

			metrics <- &convertResults{
				Metric: &samplers.UDPMetric{
					MetricKey: samplers.MetricKey{
						Name: *metricFamily.Name,
						Type: "gauge",
					},
					Tags:      summaryTags,
					Timestamp: *metric.TimestampMs,
					Value:     quantile.Value,
				},
			}
		}
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: fmt.Sprintf("%s.count", *metricFamily.Name),
					Type: "counter",
				},
				Tags:      tags,
				Timestamp: *metric.TimestampMs,
				Value:     metric.Summary.SampleCount,
			},
		}
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: fmt.Sprintf("%s.sum", *metricFamily.Name),
					Type: "counter",
				},
				Tags:      tags,
				Timestamp: *metric.TimestampMs,
				Value:     metric.Summary.SampleSum,
			},
		}
	}
}

func (source *OpenMetricsSource) convertHistogram(
	metricFamily *dto.MetricFamily, metrics chan *convertResults,
) {
	for _, metric := range metricFamily.Metric {
		tags := getTags(metric.Label)
		for _, bucket := range metric.Histogram.Bucket {
			bucketTags := make([]string, len(tags))
			copy(bucketTags, tags)
			bucketTags = append(tags, fmt.Sprintf(
				"%s:%f", source.histogramBucketTag, *bucket.UpperBound))

			metrics <- &convertResults{
				Metric: &samplers.UDPMetric{
					MetricKey: samplers.MetricKey{
						Name: *metricFamily.Name,
						Type: "counter",
					},
					Tags:      bucketTags,
					Timestamp: *metric.TimestampMs,
					Value:     bucket.CumulativeCount,
				},
			}
		}
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: fmt.Sprintf("%s.count", *metricFamily.Name),
					Type: "counter",
				},
				Tags:      tags,
				Timestamp: *metric.TimestampMs,
				Value:     metric.Histogram.SampleCount,
			},
		}
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: fmt.Sprintf("%s.sum", *metricFamily.Name),
					Type: "counter",
				},
				Tags:      tags,
				Timestamp: *metric.TimestampMs,
				Value:     metric.Histogram.SampleSum,
			},
		}
	}
}

func (source *OpenMetricsSource) convertUntyped(
	metricFamily *dto.MetricFamily, metrics chan *convertResults,
) {
	for _, metric := range metricFamily.Metric {
		metrics <- &convertResults{
			Metric: &samplers.UDPMetric{
				MetricKey: samplers.MetricKey{
					Name: *metricFamily.Name,
					Type: "gauge",
				},
				Tags:      getTags(metric.Label),
				Timestamp: *metric.TimestampMs,
				Value:     metric.Untyped.Value,
			},
		}
	}
}

func getTags(labels []*dto.LabelPair) []string {
	tags := make([]string, len(labels))
	for index, label := range labels {
		tags[index] = fmt.Sprintf("%s:%s", *label.Name, *label.Value)
	}
	sort.Strings(tags)
	return tags
}
