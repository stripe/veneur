package veneur

import (
	"os"
	"time"

	"github.com/stripe/veneur/v14/util"
	"github.com/stripe/veneur/v14/util/matcher"
	"github.com/stripe/veneur/v14/util/tls"
)

type Config struct {
	Aggregates                  []string            `yaml:"aggregates"`
	BlockProfileRate            int                 `yaml:"block_profile_rate"`
	CountUniqueTimeseries       bool                `yaml:"count_unique_timeseries"`
	Debug                       bool                `yaml:"debug"`
	EnableProfiling             bool                `yaml:"enable_profiling"`
	ExtendTags                  []string            `yaml:"extend_tags"`
	Features                    Features            `yaml:"features"`
	FlushOnShutdown             bool                `yaml:"flush_on_shutdown"`
	FlushWatchdogMissedFlushes  int                 `yaml:"flush_watchdog_missed_flushes"`
	ForwardAddress              string              `yaml:"forward_address"`
	ForwardOnly                 bool                `yaml:"forward_only"`
	GrpcAddress                 string              `yaml:"grpc_address"`
	GrpcListenAddresses         []util.Url          `yaml:"grpc_listen_addresses"`
	Hostname                    string              `yaml:"hostname"`
	HTTP                        HttpConfig          `yaml:"http"`
	HTTPAddress                 string              `yaml:"http_address"`
	HTTPQuit                    bool                `yaml:"http_quit"`
	IndicatorSpanTimerName      string              `yaml:"indicator_span_timer_name"`
	Interval                    time.Duration       `yaml:"interval"`
	MetricMaxLength             int                 `yaml:"metric_max_length"`
	MetricSinkRouting           []SinkRoutingConfig `yaml:"metric_sink_routing"`
	MetricSinks                 []SinkConfig        `yaml:"metric_sinks"`
	MutexProfileFraction        int                 `yaml:"mutex_profile_fraction"`
	NumReaders                  int                 `yaml:"num_readers"`
	NumSpanWorkers              int                 `yaml:"num_span_workers"`
	NumWorkers                  int                 `yaml:"num_workers"`
	ObjectiveSpanTimerName      string              `yaml:"objective_span_timer_name"`
	OmitEmptyHostname           bool                `yaml:"omit_empty_hostname"`
	Percentiles                 []float64           `yaml:"percentiles"`
	ReadBufferSizeBytes         int                 `yaml:"read_buffer_size_bytes"`
	SentryDsn                   util.StringSecret   `yaml:"sentry_dsn"`
	Sources                     []SourceConfig      `yaml:"sources"`
	SpanChannelCapacity         int                 `yaml:"span_channel_capacity"`
	SpanSinks                   []SinkConfig        `yaml:"span_sinks"`
	SsfListenAddresses          []util.Url          `yaml:"ssf_listen_addresses"`
	StatsAddress                string              `yaml:"stats_address"`
	StatsdListenAddresses       []util.Url          `yaml:"statsd_listen_addresses"`
	SynchronizeWithInterval     bool                `yaml:"synchronize_with_interval"`
	TagsExclude                 []string            `yaml:"tags_exclude"`
	TLSAuthorityCertificate     string              `yaml:"tls_authority_certificate"`
	TLSCertificate              string              `yaml:"tls_certificate"`
	TLSKey                      util.StringSecret   `yaml:"tls_key"`
	Tls                         *tls.Tls            `yaml:"tls"`
	TraceMaxLengthBytes         int                 `yaml:"trace_max_length_bytes"`
	VeneurMetricsAdditionalTags []string            `yaml:"veneur_metrics_additional_tags"`
	VeneurMetricsScopes         struct {
		Counter   string `yaml:"counter"`
		Gauge     string `yaml:"gauge"`
		Histogram string `yaml:"histogram"`
		Set       string `yaml:"set"`
		Status    string `yaml:"status"`
	} `yaml:"veneur_metrics_scopes"`
}

type Features struct {
	DiagnosticsMetricsEnabled bool `yaml:"diagnostics_metrics_enabled"`
	EnableMetricSinkRouting   bool `yaml:"enable_metric_sink_routing"`
}

type HttpConfig struct {
	// Enables /config/json and /config/yaml endpoints for displaying the current
	// configuration. Entries of type util.StringSecret will be redacted unless
	// the -print-secrets flag is set.
	Config bool `yaml:"config"`
}

type SinkRoutingConfig struct {
	Name  string            `yaml:"name"`
	Match []matcher.Matcher `yaml:"match"`
	Sinks SinkRoutingSinks  `yaml:"sinks"`
}

type SinkRoutingSinks struct {
	Matched    []string `yaml:"matched"`
	NotMatched []string `yaml:"not_matched"`
}
type SourceConfig struct {
	Kind   string      `yaml:"kind"`
	Name   string      `yaml:"name"`
	Config interface{} `yaml:"config"`
	Tags   []string    `yaml:"tags"`
}

type SinkConfig struct {
	Kind          string               `yaml:"kind"`
	Name          string               `yaml:"name"`
	Config        interface{}          `yaml:"config"`
	MaxNameLength int                  `yaml:"max_name_length"`
	MaxTagLength  int                  `yaml:"max_tag_length"`
	MaxTags       int                  `yaml:"max_tags"`
	StripTags     []matcher.TagMatcher `yaml:"strip_tags"`
	AddTags       map[string]string    `yaml:"add_tags"`
}

var defaultConfig = Config{
	Aggregates:          []string{"min", "max", "count"},
	Interval:            10 * time.Second,
	MetricMaxLength:     4096,
	ReadBufferSizeBytes: 1048576 * 2, // 2 MiB
	SpanChannelCapacity: 100,
}

func (c *Config) ApplyDefaults() {
	if len(c.Aggregates) == 0 {
		c.Aggregates = defaultConfig.Aggregates
	}
	if c.Hostname == "" && !c.OmitEmptyHostname {
		c.Hostname, _ = os.Hostname()
	}
	if c.Interval == 0 {
		c.Interval = defaultConfig.Interval
	}
	if c.MetricMaxLength == 0 {
		c.MetricMaxLength = defaultConfig.MetricMaxLength
	}
	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultConfig.ReadBufferSizeBytes
	}

	if c.SpanChannelCapacity == 0 {
		c.SpanChannelCapacity = defaultConfig.SpanChannelCapacity
	}
}
