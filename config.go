package veneur

import (
	"time"

	"github.com/stripe/veneur/v14/util"
	"github.com/stripe/veneur/v14/util/matcher"
)

type Config struct {
	Aggregates                             []string          `yaml:"aggregates"`
	AwsAccessKeyID                         util.StringSecret `yaml:"aws_access_key_id"`
	AwsRegion                              string            `yaml:"aws_region"`
	AwsS3Bucket                            string            `yaml:"aws_s3_bucket"`
	AwsSecretAccessKey                     util.StringSecret `yaml:"aws_secret_access_key"`
	BlockProfileRate                       int               `yaml:"block_profile_rate"`
	CountUniqueTimeseries                  bool              `yaml:"count_unique_timeseries"`
	DatadogAPIHostname                     string            `yaml:"datadog_api_hostname"`
	DatadogAPIKey                          util.StringSecret `yaml:"datadog_api_key"`
	DatadogExcludeTagsPrefixByPrefixMetric []struct {
		MetricPrefix string   `yaml:"metric_prefix"`
		Tags         []string `yaml:"tags"`
	} `yaml:"datadog_exclude_tags_prefix_by_prefix_metric"`
	DatadogFlushMaxPerBody       int      `yaml:"datadog_flush_max_per_body"`
	DatadogMetricNamePrefixDrops []string `yaml:"datadog_metric_name_prefix_drops"`
	DatadogSpanBufferSize        int      `yaml:"datadog_span_buffer_size"`
	DatadogTraceAPIAddress       string   `yaml:"datadog_trace_api_address"`
	Debug                        bool     `yaml:"debug"`
	DebugFlushedMetrics          bool     `yaml:"debug_flushed_metrics"`
	DebugIngestedSpans           bool     `yaml:"debug_ingested_spans"`
	EnableProfiling              bool     `yaml:"enable_profiling"`
	ExtendTags                   []string `yaml:"extend_tags"`
	FalconerAddress              string   `yaml:"falconer_address"`
	Features                     struct {
		EnableMetricSinkRouting bool   `yaml:"enable_metric_sink_routing"`
		MigrateMetricSinks      bool   `yaml:"migrate_metric_sinks"`
		MigrateSpanSinks        bool   `yaml:"migrate_span_sinks"`
		ProxyProtocol           string `yaml:"proxy_protocol"`
	} `yaml:"features"`
	FlushFile                                 string              `yaml:"flush_file"`
	FlushMaxPerBody                           int                 `yaml:"flush_max_per_body"`
	FlushOnShutdown                           bool                `yaml:"flush_on_shutdown"`
	FlushWatchdogMissedFlushes                int                 `yaml:"flush_watchdog_missed_flushes"`
	ForwardAddress                            string              `yaml:"forward_address"`
	ForwardUseGrpc                            bool                `yaml:"forward_use_grpc"`
	GrpcAddress                               string              `yaml:"grpc_address"`
	GrpcListenAddresses                       []util.Url          `yaml:"grpc_listen_addresses"`
	Hostname                                  string              `yaml:"hostname"`
	HTTP                                      HttpConfig          `yaml:"http"`
	HTTPAddress                               string              `yaml:"http_address"`
	HTTPQuit                                  bool                `yaml:"http_quit"`
	IndicatorSpanTimerName                    string              `yaml:"indicator_span_timer_name"`
	Interval                                  time.Duration       `yaml:"interval"`
	KafkaBroker                               string              `yaml:"kafka_broker"`
	KafkaCheckTopic                           string              `yaml:"kafka_check_topic"`
	KafkaEventTopic                           string              `yaml:"kafka_event_topic"`
	KafkaMetricBufferBytes                    int                 `yaml:"kafka_metric_buffer_bytes"`
	KafkaMetricBufferFrequency                time.Duration       `yaml:"kafka_metric_buffer_frequency"`
	KafkaMetricBufferMessages                 int                 `yaml:"kafka_metric_buffer_messages"`
	KafkaMetricRequireAcks                    string              `yaml:"kafka_metric_require_acks"`
	KafkaMetricTopic                          string              `yaml:"kafka_metric_topic"`
	KafkaPartitioner                          string              `yaml:"kafka_partitioner"`
	KafkaRetryMax                             int                 `yaml:"kafka_retry_max"`
	KafkaSpanBufferBytes                      int                 `yaml:"kafka_span_buffer_bytes"`
	KafkaSpanBufferFrequency                  time.Duration       `yaml:"kafka_span_buffer_frequency"`
	KafkaSpanBufferMesages                    int                 `yaml:"kafka_span_buffer_mesages"`
	KafkaSpanRequireAcks                      string              `yaml:"kafka_span_require_acks"`
	KafkaSpanSampleRatePercent                float64             `yaml:"kafka_span_sample_rate_percent"`
	KafkaSpanSampleTag                        string              `yaml:"kafka_span_sample_tag"`
	KafkaSpanSerializationFormat              string              `yaml:"kafka_span_serialization_format"`
	KafkaSpanTopic                            string              `yaml:"kafka_span_topic"`
	LightstepAccessToken                      util.StringSecret   `yaml:"lightstep_access_token"`
	LightstepCollectorHost                    util.Url            `yaml:"lightstep_collector_host"`
	LightstepMaximumSpans                     int                 `yaml:"lightstep_maximum_spans"`
	LightstepNumClients                       int                 `yaml:"lightstep_num_clients"`
	LightstepReconnectPeriod                  time.Duration       `yaml:"lightstep_reconnect_period"`
	MetricMaxLength                           int                 `yaml:"metric_max_length"`
	MetricSinkRouting                         []SinkRoutingConfig `yaml:"metric_sink_routing"`
	MetricSinks                               []SinkConfig        `yaml:"metric_sinks"`
	MutexProfileFraction                      int                 `yaml:"mutex_profile_fraction"`
	NewrelicAccountID                         int                 `yaml:"newrelic_account_id"`
	NewrelicCommonTags                        []string            `yaml:"newrelic_common_tags"`
	NewrelicEventType                         string              `yaml:"newrelic_event_type"`
	NewrelicInsertKey                         util.StringSecret   `yaml:"newrelic_insert_key"`
	NewrelicRegion                            string              `yaml:"newrelic_region"`
	NewrelicServiceCheckEventType             string              `yaml:"newrelic_service_check_event_type"`
	NewrelicTraceObserverURL                  string              `yaml:"newrelic_trace_observer_url"`
	NumReaders                                int                 `yaml:"num_readers"`
	NumSpanWorkers                            int                 `yaml:"num_span_workers"`
	NumWorkers                                int                 `yaml:"num_workers"`
	ObjectiveSpanTimerName                    string              `yaml:"objective_span_timer_name"`
	OmitEmptyHostname                         bool                `yaml:"omit_empty_hostname"`
	Percentiles                               []float64           `yaml:"percentiles"`
	PrometheusNetworkType                     string              `yaml:"prometheus_network_type"`
	PrometheusRepeaterAddress                 string              `yaml:"prometheus_repeater_address"`
	PrometheusRemoteBearerToken               string              `yaml:"prometheus_remote_bearer_token"`
	PrometheusRemoteFlushMaxConcurrency       int                 `yaml:"prometheus_remote_flush_max_concurrency"`
	PrometheusRemoteFlushMaxPerBody           int                 `yaml:"prometheus_remote_flush_max_per_body"`
	PrometheusRemoteFlushInterval             int                 `yaml:"prometheus_remote_flush_interval"`
	PrometheusRemoteFlushTimeout              int                 `yaml:"prometheus_remote_flush_timeout"`
	PrometheusRemoteBufferQueueSize           int                 `yaml:"prometheus_remote_buffer_queue_size"`
	PrometheusRemoteWriteAddress              string              `yaml:"prometheus_remote_write_address"`
	ReadBufferSizeBytes                       int                 `yaml:"read_buffer_size_bytes"`
	SentryDsn                                 util.StringSecret   `yaml:"sentry_dsn"`
	SignalfxAPIKey                            util.StringSecret   `yaml:"signalfx_api_key"`
	SignalfxDynamicPerTagAPIKeysEnable        bool                `yaml:"signalfx_dynamic_per_tag_api_keys_enable"`
	SignalfxDynamicPerTagAPIKeysRefreshPeriod time.Duration       `yaml:"signalfx_dynamic_per_tag_api_keys_refresh_period"`
	SignalfxEndpointAPI                       string              `yaml:"signalfx_endpoint_api"`
	SignalfxEndpointBase                      string              `yaml:"signalfx_endpoint_base"`
	SignalfxFlushMaxPerBody                   int                 `yaml:"signalfx_flush_max_per_body"`
	SignalfxHostnameTag                       string              `yaml:"signalfx_hostname_tag"`
	SignalfxMetricNamePrefixDrops             []string            `yaml:"signalfx_metric_name_prefix_drops"`
	SignalfxMetricTagPrefixDrops              []string            `yaml:"signalfx_metric_tag_prefix_drops"`
	SignalfxPerTagAPIKeys                     []struct {
		APIKey util.StringSecret `yaml:"api_key"`
		Name   string            `yaml:"name"`
	} `yaml:"signalfx_per_tag_api_keys"`
	SignalfxVaryKeyBy                      string            `yaml:"signalfx_vary_key_by"`
	SignalfxVaryKeyByFavorCommonDimensions bool              `yaml:"signalfx_vary_key_by_favor_common_dimensions"`
	Sources                                []SourceConfig    `yaml:"sources"`
	SpanChannelCapacity                    int               `yaml:"span_channel_capacity"`
	SpanSinks                              []SinkConfig      `yaml:"span_sinks"`
	SplunkHecAddress                       string            `yaml:"splunk_hec_address"`
	SplunkHecBatchSize                     int               `yaml:"splunk_hec_batch_size"`
	SplunkHecConnectionLifetimeJitter      time.Duration     `yaml:"splunk_hec_connection_lifetime_jitter"`
	SplunkHecIngestTimeout                 time.Duration     `yaml:"splunk_hec_ingest_timeout"`
	SplunkHecMaxConnectionLifetime         time.Duration     `yaml:"splunk_hec_max_connection_lifetime"`
	SplunkHecSendTimeout                   time.Duration     `yaml:"splunk_hec_send_timeout"`
	SplunkHecSubmissionWorkers             int               `yaml:"splunk_hec_submission_workers"`
	SplunkHecTLSValidateHostname           string            `yaml:"splunk_hec_tls_validate_hostname"`
	SplunkHecToken                         string            `yaml:"splunk_hec_token"`
	SplunkSpanSampleRate                   int               `yaml:"splunk_span_sample_rate"`
	SsfBufferSize                          int               `yaml:"ssf_buffer_size"`
	SsfListenAddresses                     []util.Url        `yaml:"ssf_listen_addresses"`
	StatsAddress                           string            `yaml:"stats_address"`
	StatsdListenAddresses                  []util.Url        `yaml:"statsd_listen_addresses"`
	SynchronizeWithInterval                bool              `yaml:"synchronize_with_interval"`
	Tags                                   []string          `yaml:"tags"`
	TagsExclude                            []string          `yaml:"tags_exclude"`
	TLSAuthorityCertificate                string            `yaml:"tls_authority_certificate"`
	TLSCertificate                         string            `yaml:"tls_certificate"`
	TLSKey                                 util.StringSecret `yaml:"tls_key"`
	TraceLightstepAccessToken              util.StringSecret `yaml:"trace_lightstep_access_token"`
	TraceLightstepCollectorHost            util.Url          `yaml:"trace_lightstep_collector_host"`
	TraceLightstepMaximumSpans             int               `yaml:"trace_lightstep_maximum_spans"`
	TraceLightstepNumClients               int               `yaml:"trace_lightstep_num_clients"`
	TraceLightstepReconnectPeriod          time.Duration     `yaml:"trace_lightstep_reconnect_period"`
	TraceMaxLengthBytes                    int               `yaml:"trace_max_length_bytes"`
	VeneurMetricsAdditionalTags            []string          `yaml:"veneur_metrics_additional_tags"`
	VeneurMetricsScopes                    struct {
		Counter   string `yaml:"counter"`
		Gauge     string `yaml:"gauge"`
		Histogram string `yaml:"histogram"`
		Set       string `yaml:"set"`
		Status    string `yaml:"status"`
	} `yaml:"veneur_metrics_scopes"`
	XrayAddress          string   `yaml:"xray_address"`
	XrayAnnotationTags   []string `yaml:"xray_annotation_tags"`
	XraySamplePercentage float64  `yaml:"xray_sample_percentage"`
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
	Kind      string               `yaml:"kind"`
	Name      string               `yaml:"name"`
	Config    interface{}          `yaml:"config"`
	StripTags []matcher.TagMatcher `yaml:"strip_tags"`
}
