package cortex

import (
	"bytes"
	"context"
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
)

// CortexMetricSink writes metrics to one or more configured Cortex instances
// using the prometheus remote-write API. For specifications, see
// https://github.com/prometheus/compliance/tree/main/remote_write
type CortexMetricSink struct {
	URL           string
	RemoteTimeout time.Duration
	ProxyURL      string
	client        *http.Client
	logger        *logrus.Entry
	name          string
	tags          map[string]string
	traceClient   *trace.Client
}

// TODO: implement queue config options, at least
// max_samples_per_send, max_shards, and capacity
// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write
type CortexMetricSinkConfig struct {
	URL           string        `yaml:"url"`
	RemoteTimeout time.Duration `yaml:"remote_timeout"`
	ProxyURL      string        `yaml:"proxy_url"`
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

	// TagsAsMap is a set of configurable common tags applied to every metric
	tags := server.TagsAsMap
	// Host has to be supplied especially
	tags["host"] = config.Hostname

	return NewCortexMetricSink(conf.URL, conf.RemoteTimeout, conf.ProxyURL, logger, name, tags)
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
	return cortexConfig, nil
}

// NewCortexMetricSink creates and returns a new instance of the sink
func NewCortexMetricSink(URL string, timeout time.Duration, proxyURL string, logger *logrus.Entry, name string, tags map[string]string) (*CortexMetricSink, error) {
	return &CortexMetricSink{
		URL:           URL,
		RemoteTimeout: timeout,
		ProxyURL:      proxyURL,
		tags:          tags,
		logger:        logger.WithFields(logrus.Fields{"sink_type": "cortex"}),
		name:          name,
	}, nil
}

// Name returns the string cortex
func (s *CortexMetricSink) Name() string {
	return s.name
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

	s.client = &http.Client{
		Timeout:   time.Duration(s.RemoteTimeout * time.Second),
		Transport: t,
	}

	s.traceClient = tc
	return nil
}

// Flush sends a batch of metrics to the configured remote-write endpoint
func (s *CortexMetricSink) Flush(ctx context.Context, metrics []samplers.InterMetric) error {
	span, _ := trace.StartSpanFromContext(ctx, "")
	defer span.ClientFinish(s.traceClient)

	wr := makeWriteRequest(metrics, s.tags)
	data, err := proto.Marshal(wr)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	encoded := snappy.Encode(nil, data)
	buf.Write(encoded)

	req, err := http.NewRequestWithContext(ctx, "POST", s.URL, &buf)
	if err != nil {
		return err
	}

	// This set of headers is prescribed by the remote-write standard
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("User-Agent", "veneur/cortex")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	r, err := s.client.Do(req)
	if err != nil {
		span.Error(err)
		s.logger.WithError(err).Info("Failed to post request")
		return err
	}
	// Resource leak can occur if body isn't closed explicitly
	defer r.Body.Close()

	// TODO: retry on 400/500 (per remote-write spec)
	if r.StatusCode >= 300 {
		b, err := ioutil.ReadAll(r.Body)
		if err != nil {
			return errors.Wrap(err, "could not read response body")
		}
		// Cortex responds with terse and informative messages
		s.logger.Infof("Flush failed with HTTP %d: %s", r.StatusCode, b)
	}

	// Emit standard sink metrics
	tags := map[string]string{"sink": s.name, "sink_type": "cortex"}
	// We don't send sinks.MetricKeyTotalMetricsSkipped at present, as it would always be 0
	span.Add(ssf.Count(sinks.MetricKeyTotalMetricsFlushed, float32(len(metrics)), tags))

	s.logger.Info("Flush complete")

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
func makeWriteRequest(metrics []samplers.InterMetric, tags map[string]string) *prompb.WriteRequest {
	ts := make([]*prompb.TimeSeries, len(metrics))
	for i, metric := range metrics {
		ts[i] = metricToTimeSeries(metric, tags)
	}

	return &prompb.WriteRequest{
		Timeseries: ts,
	}
}

// metricToTimeSeries converts a sample to a prometheus timeseries.
// This is not a 1:1 conversion! We constrain metric and label names to the
// legal set of characters (see https://prometheus.io/docs/practices/naming)
// and we drop tags which are not in "key:value" format
// (see https://prometheus.io/docs/concepts/data_model/)
// we also take "last value wins" approach to duplicate labels inside a metric
// cortex does not dupe tags
func metricToTimeSeries(metric samplers.InterMetric, tags map[string]string) *prompb.TimeSeries {
	var ts prompb.TimeSeries
	ts.Labels = []*prompb.Label{{
		Name: "__name__", Value: sanitise(metric.Name),
	}}

	sanitizedTags := map[string]string{}
	var keys []string

	for _, tag := range metric.Tags {
		kv := strings.SplitN(tag, ":", 2)
		if len(kv) < 2 {
			continue // drop illegal tag
		}

		sk := sanitise(kv[0])
		sanitizedTags[sk] = kv[1]
		keys = append(keys, sk)
	}

	for k, v := range tags {
		sk := sanitise(k)
		sanitizedTags[sk] = v
		keys = append(keys, sk)
	}

	sort.Strings(keys)

	for _, k := range keys {
		ts.Labels = append(ts.Labels, &prompb.Label{Name: k, Value: sanitizedTags[k]})
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
