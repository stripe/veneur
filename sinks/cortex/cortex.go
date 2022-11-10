package cortex

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/prompb"
	"github.com/sirupsen/logrus"
	"github.com/stripe/veneur/v14"
	"github.com/stripe/veneur/v14/samplers"
	"github.com/stripe/veneur/v14/sinks"
	"github.com/stripe/veneur/v14/ssf"
	"github.com/stripe/veneur/v14/trace"
	"github.com/stripe/veneur/v14/util"
)

const (
	// MaxConns is the number of simultaneous connections we allow to one host
	MaxConns = 100
	// DefaultRemoteTimeout after which a write request will time out
	DefaultRemoteTimeout = time.Duration(30 * time.Second)
	// DefaultAuthorizationType if a credential is supplied
	DefaultAuthorizationType = "Bearer"
)

type counterMapKey struct {
	name string
	tags string
}

// CortexMetricSink writes metrics to one or more configured Cortex instances
// using the prometheus remote-write API. For specifications, see
// https://github.com/prometheus/compliance/tree/main/remote_write
type CortexMetricSink struct {
	URL                        string
	RemoteTimeout              time.Duration
	ProxyURL                   string
	Client                     *http.Client
	logger                     *logrus.Entry
	name                       string
	traceClient                *trace.Client
	addHeaders                 map[string]string
	basicAuth                  *BasicAuthType
	batchWriteSize             int
	counters                   map[counterMapKey]float64
	convertCountersToMonotonic bool
	excludedTags               map[string]struct{}
	defaultBucket              string
	bucketByKeys               []string
	bucketBy                   string
	includeBucketSize          bool
}

var _ sinks.MetricSink = (*CortexMetricSink)(nil)

type BasicAuthType struct {
	Username util.StringSecret `yaml:"username"`
	Password util.StringSecret `yaml:"password"`
}

// TODO: implement queue config options, at least
// max_samples_per_send, max_shards, and capacity
// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write
type CortexMetricSinkConfig struct {
	URL                        string            `yaml:"url"`
	RemoteTimeout              time.Duration     `yaml:"remote_timeout"`
	ProxyURL                   string            `yaml:"proxy_url"`
	BatchWriteSize             int               `yaml:"batch_write_size"`
	Headers                    map[string]string `yaml:"headers"`
	BasicAuth                  BasicAuthType     `yaml:"basic_auth"`
	ConvertCountersToMonotonic bool              `yaml:"convert_counters_to_monotonic"`
	Authorization              struct {
		Type       string            `yaml:"type"`
		Credential util.StringSecret `yaml:"credentials"`
	} `yaml:"authorization"`
	DefaultBucket    string   `yaml:"default_bucket"`
	BucketByKeys     []string `yaml:"bucket_by_keys"`
	HeadersPerBucket struct {
		BucketBy          string `yaml:"bucket_name"`
		IncludeBucketSize bool   `yaml:"include_bucket_size"`
	} `yaml:"headers_per_bucket"`
}

// Create creates a new Cortex sink.
// @see veneur.MetricSinkTypes
func Create(
	server *veneur.Server, name string, logger *logrus.Entry,
	config veneur.Config, sinkConfig veneur.MetricSinkConfig,
) (sinks.MetricSink, error) {
	conf, ok := sinkConfig.(CortexMetricSinkConfig)
	if !ok {
		return nil, errors.New("invalid sink config type")
	}

	if conf.BucketByKeys != nil && conf.DefaultBucket == "" {
		return nil, errors.New("cannot create buckets without specifying the default bucket")
	}

	headers := make(map[string]string)
	if conf.Authorization.Type != "" {
		headers["Authorization"] = conf.Authorization.Type + " " + conf.Authorization.Credential.Value
	}
	var basicAuth *BasicAuthType
	if conf.BasicAuth.Username.Value != "" {
		basicAuth = &conf.BasicAuth
	}

	return NewCortexMetricSink(conf.URL, conf.RemoteTimeout, conf.ProxyURL, logger, name, headers, basicAuth, conf.BatchWriteSize, conf.ConvertCountersToMonotonic)
}

// ParseConfig extracts Cortex specific fields from the global veneur config
func ParseConfig(
	name string, config interface{},
) (veneur.MetricSinkConfig, error) {
	cortexConfig := CortexMetricSinkConfig{}
	err := util.DecodeConfig(name, config, &cortexConfig)
	if err != nil {
		return nil, err
	}
	if cortexConfig.RemoteTimeout == 0 {
		cortexConfig.RemoteTimeout = DefaultRemoteTimeout
	}
	if cortexConfig.BasicAuth.Username.Value != "" {
		if cortexConfig.BasicAuth.Password.Value == "" {
			return nil, errors.New("cortex sink needs both username and password")
		}
		if cortexConfig.Authorization.Credential.Value != "" {
			return nil, errors.New("cortex sink needs exactly one of basic_auth and authorization")
		}
	}
	if cortexConfig.Authorization.Credential.Value != "" && cortexConfig.Authorization.Type == "" {
		cortexConfig.Authorization.Type = DefaultAuthorizationType
	}
	return cortexConfig, nil
}

// NewCortexMetricSink creates and returns a new instance of the sink
func NewCortexMetricSink(URL string, timeout time.Duration, proxyURL string, logger *logrus.Entry, name string, headers map[string]string, basicAuth *BasicAuthType, batchWriteSize int, convertCountersToMonotonic bool) (*CortexMetricSink, error) {
	sink := &CortexMetricSink{
		URL:                        URL,
		RemoteTimeout:              timeout,
		ProxyURL:                   proxyURL,
		logger:                     logger,
		name:                       name,
		addHeaders:                 headers,
		basicAuth:                  basicAuth,
		batchWriteSize:             batchWriteSize,
		counters:                   map[counterMapKey]float64{},
		convertCountersToMonotonic: convertCountersToMonotonic,
		excludedTags:               map[string]struct{}{},
	}
	sink.logger = sink.logger.WithFields(logrus.Fields{
		"sink_name": sink.Name(),
		"sink_kind": sink.Kind(),
	})
	return sink, nil
}

// Name returns the sink name
func (s *CortexMetricSink) Name() string {
	return s.name
}

// Kind returns the sink kind
func (s *CortexMetricSink) Kind() string {
	return "cortex"
}

// Start sets up the HTTP client for writing to Cortex
func (s *CortexMetricSink) Start(tc *trace.Client) error {
	s.logger.Infof("Starting sink for %s", s.URL)
	// Default concurrent connections is 2
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = MaxConns
	t.MaxConnsPerHost = MaxConns
	t.MaxIdleConnsPerHost = MaxConns

	// Configure proxy URL, if supplied
	if len(s.ProxyURL) > 0 {
		p, err := url.Parse(s.ProxyURL)
		if err != nil {
			return errors.Wrap(err, "malformed cortex_proxy_url")
		}
		t.Proxy = http.ProxyURL(p)
	}

	s.Client = &http.Client{
		Timeout:   s.RemoteTimeout,
		Transport: t,
	}

	s.traceClient = tc
	return nil
}

// Flush sends a batch of metrics to the configured remote-write endpoint
func (s *CortexMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) (sinks.MetricFlushResult, error) {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)
	metricKeyTags := map[string]string{"sink_name": s.Name(), "sink_type": s.Kind()}
	flushedMetrics := 0
	droppedMetrics := 0
	defer func() {
		span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(flushedMetrics), metricKeyTags))
		span.Add(ssf.Count(sinks.MetricKeyTotalMetricsDropped, float32(droppedMetrics), metricKeyTags))
	}()

	if len(metrics) == 0 {
		return sinks.MetricFlushResult{}, nil
	}

	if s.batchWriteSize == 0 || len(metrics) <= s.batchWriteSize {
		err := s.writeMetrics(ctx, metrics)
		if err == nil {
			flushedMetrics = len(metrics)
		} else {
			s.logger.Error(err)
			droppedMetrics = len(metrics)
		}

		return sinks.MetricFlushResult{MetricsFlushed: flushedMetrics, MetricsDropped: droppedMetrics}, err
	}

	doIfNotDone := func(fn func() error) error {
		select {
		case <-ctx.Done():
			return errors.New("context finished before completing metrics flush")
		default:
			return fn()
		}
	}

	var batch []samplers.InterMetric
	for i, metric := range metrics {
		err := doIfNotDone(func() error {
			batch = append(batch, metric)
			if i > 0 && i%s.batchWriteSize == 0 {
				err := s.writeMetrics(ctx, batch)
				if err != nil {
					return err
				}

				flushedMetrics += len(batch)
				batch = []samplers.InterMetric{}
			}

			return nil
		})

		if err != nil {
			s.logger.Error(err)
			droppedMetrics += len(batch)
			return sinks.MetricFlushResult{MetricsFlushed: flushedMetrics, MetricsDropped: droppedMetrics}, err
		}
	}

	var err error
	if len(batch) > 0 {
		err = doIfNotDone(func() error {
			return s.writeMetrics(ctx, batch)
		})

		if err == nil {
			flushedMetrics += len(batch)
		} else {
			s.logger.Error(err)
			droppedMetrics += len(batch)
		}
	}

	return sinks.MetricFlushResult{MetricsFlushed: flushedMetrics, MetricsDropped: droppedMetrics}, err
}

func (s *CortexMetricSink) writeMetrics(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)

	wr, updatedCounters := s.makeWriteRequest(metrics)

	data, err := proto.Marshal(wr)
	if err != nil {
		return errors.Wrapf(err, "cortex_err=\"failed to write batch: failed to marshal proto\"")
	}

	var buf bytes.Buffer
	encoded := snappy.Encode(nil, data)
	buf.Write(encoded)

	req, err := http.NewRequestWithContext(ctx, "POST", s.URL, &buf)
	if err != nil {
		return errors.Wrapf(err, "cortex_err=\"failed to write batch: failed to create http request\"")
	}

	// This set of headers is prescribed by the remote-write standard
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", "veneur/cortex")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")
	for key, value := range s.addHeaders {
		req.Header.Set(key, value)
	}
	if s.basicAuth != nil {
		req.SetBasicAuth(s.basicAuth.Username.Value, s.basicAuth.Password.Value)
	}

	ts := time.Now()
	r, err := s.Client.Do(req)
	if err != nil {
		span.Error(err)
		return errors.Wrapf(err, "cortex_err=\"failed to write batch: misc http client error\" duration_secs=%.2f", time.Since(ts).Seconds())
	}
	// Resource leak can occur if body isn't closed explicitly
	defer r.Body.Close()

	// TODO: retry on 4xx/5xx (per remote-write spec)
	// Draft PR: https://github.com/stripe/veneur/pull/925
	if r.StatusCode >= 300 {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.Wrapf(err, "cortex_err=\"failed to write batch: downstream returned error response with unreadable body\" response_code=%d", r.StatusCode)
		}
		return fmt.Errorf("cortex_err=\"failed to write batch: error response\", response_code=%d response_body=\"%s\"", r.StatusCode, b)
	}

	for key, val := range updatedCounters {
		s.counters[key] = val
	}

	return nil
}

// FlushOtherSamples would forward non-metric sanples like spans. Prometheus
// cannot receive them, so this is a no-op.
func (s *CortexMetricSink) FlushOtherSamples(context.Context, []ssf.SSFSample) {
	// TODO convert samples to metrics and send them
	// as in FlushOtherSamples in the signalfx sink
}

// makeWriteRequest converts a list of samples from a flush into a single
// prometheus remote-write compatible protobuf object
func (s *CortexMetricSink) makeWriteRequest(metrics []samplers.InterMetric) (*prompb.WriteRequest, map[counterMapKey]float64) {
	ts := make([]*prompb.TimeSeries, len(metrics))
	updatedCounters := map[counterMapKey]float64{}
	for i, metric := range metrics {
		if metric.Type == samplers.CounterMetric && s.convertCountersToMonotonic {
			newMetric, counterKey := s.convertToMonotonicCounter(metric)
			metric = newMetric
			updatedCounters[counterKey] = metric.Value
		}

		ts[i] = metricToTimeSeries(metric, s.excludedTags)
	}

	return &prompb.WriteRequest{
		Timeseries: ts,
	}, updatedCounters
}

func (s *CortexMetricSink) convertToMonotonicCounter(metric samplers.InterMetric) (samplers.InterMetric, counterMapKey) {
	sort.Strings(metric.Tags)
	key := counterMapKey{
		name: metric.Name,
		tags: strings.Join(metric.Tags, "|"),
	}

	metric.Value += s.counters[key]
	return metric, key
}

// SetExcludedTags sets the excluded tag names. Any tags with the
// provided key (name) will be excluded.
func (s *CortexMetricSink) SetExcludedTags(excludes []string) {
	tagsSet := map[string]struct{}{}
	for _, tag := range excludes {
		tagsSet[tag] = struct{}{}
	}
	s.excludedTags = tagsSet
}

// metricToTimeSeries converts a sample to a prometheus timeseries.
// This is not a 1:1 conversion! We constrain metric and label names to the
// legal set of characters (see https://prometheus.io/docs/practices/naming)
// and we drop tags which are not in "key:value" format
// (see https://prometheus.io/docs/concepts/data_model/)
// we also take "last value wins" approach to duplicate labels inside a metric
func metricToTimeSeries(metric samplers.InterMetric, excludedTags map[string]struct{}) *prompb.TimeSeries {
	var ts prompb.TimeSeries
	ts.Labels = []*prompb.Label{{
		Name: "__name__", Value: sanitise(metric.Name),
	}}

	sanitisedTags := map[string]string{}

	for _, tag := range metric.Tags {
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) < 2 {
			continue // drop illegal tag
		}

		sk := sanitise(kv[0])
		sanitisedTags[sk] = kv[1]
	}

	for k := range excludedTags {
		sk := sanitise(k)
		delete(sanitisedTags, sk)
	}

	for k, v := range sanitisedTags {
		ts.Labels = append(ts.Labels, &prompb.Label{Name: k, Value: v})
	}

	// Prom format has the ability to carry batched samples, in this instance we
	// send a single sample per write. Probably worth exploring this as an area
	// for optimisation if we find the write path becomes contended
	ts.Samples = []prompb.Sample{{
		Value: metric.Value, Timestamp: metric.Timestamp * 1000,
	}}

	return &ts
}

// sanitise replaces all characters which are not in the set [a-zA-Z0-9_:]
// with underscores, and additionally prefixes the whole with an underscore if
// the first character is numeric
func sanitise(input string) string {
	output := make([]rune, len(input))
	prefix := false

	// this relatively verbose byte comparison is ~4x quicker than using a single
	// compiled regexp.ReplaceAll, and we'll be calling it a lot
	for i, r := range input {
		switch {
		case r >= 'a' && r <= 'z':
			output[i] = r
		case r >= 'A' && r <= 'Z':
			output[i] = r
		case r >= '0' && r <= '9':
			if i == 0 {
				prefix = true
			}
			output[i] = r
		case r == '_':
			output[i] = r
		case r == ':':
			output[i] = r
		default:
			output[i] = '_'
		}
	}
	if prefix {
		return "_" + string(output)
	}
	return string(output)
}
