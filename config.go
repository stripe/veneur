package veneur

type Config struct {
	Aggregates                   []string  `yaml:"aggregates"`
	AwsAccessKeyID               string    `yaml:"aws_access_key_id"`
	AwsRegion                    string    `yaml:"aws_region"`
	AwsS3Bucket                  string    `yaml:"aws_s3_bucket"`
	AwsSecretAccessKey           string    `yaml:"aws_secret_access_key"`
	BlockProfileRate             int       `yaml:"block_profile_rate"`
	DatadogAPIHostname           string    `yaml:"datadog_api_hostname"`
	DatadogAPIKey                string    `yaml:"datadog_api_key"`
	DatadogFlushMaxPerBody       int       `yaml:"datadog_flush_max_per_body"`
	DatadogSpanBufferSize        int       `yaml:"datadog_span_buffer_size"`
	DatadogTraceAPIAddress       string    `yaml:"datadog_trace_api_address"`
	Debug                        bool      `yaml:"debug"`
	EnableProfiling              bool      `yaml:"enable_profiling"`
	FalconerAddress              string    `yaml:"falconer_address"`
	FlushFile                    string    `yaml:"flush_file"`
	FlushMaxPerBody              int       `yaml:"flush_max_per_body"`
	ForwardAddress               string    `yaml:"forward_address"`
	ForwardUseGrpc               bool      `yaml:"forward_use_grpc"`
	GrpcAddress                  string    `yaml:"grpc_address"`
	Hostname                     string    `yaml:"hostname"`
	HTTPAddress                  string    `yaml:"http_address"`
	IndicatorSpanTimerName       string    `yaml:"indicator_span_timer_name"`
	Interval                     string    `yaml:"interval"`
	KafkaBroker                  string    `yaml:"kafka_broker"`
	KafkaCheckTopic              string    `yaml:"kafka_check_topic"`
	KafkaEventTopic              string    `yaml:"kafka_event_topic"`
	KafkaMetricBufferBytes       int       `yaml:"kafka_metric_buffer_bytes"`
	KafkaMetricBufferFrequency   string    `yaml:"kafka_metric_buffer_frequency"`
	KafkaMetricBufferMessages    int       `yaml:"kafka_metric_buffer_messages"`
	KafkaMetricRequireAcks       string    `yaml:"kafka_metric_require_acks"`
	KafkaMetricTopic             string    `yaml:"kafka_metric_topic"`
	KafkaPartitioner             string    `yaml:"kafka_partitioner"`
	KafkaRetryMax                int       `yaml:"kafka_retry_max"`
	KafkaSpanBufferBytes         int       `yaml:"kafka_span_buffer_bytes"`
	KafkaSpanBufferFrequency     string    `yaml:"kafka_span_buffer_frequency"`
	KafkaSpanBufferMesages       int       `yaml:"kafka_span_buffer_mesages"`
	KafkaSpanRequireAcks         string    `yaml:"kafka_span_require_acks"`
	KafkaSpanSampleRatePercent   int       `yaml:"kafka_span_sample_rate_percent"`
	KafkaSpanSampleTag           string    `yaml:"kafka_span_sample_tag"`
	KafkaSpanSerializationFormat string    `yaml:"kafka_span_serialization_format"`
	KafkaSpanTopic               string    `yaml:"kafka_span_topic"`
	LightstepAccessToken         string    `yaml:"lightstep_access_token"`
	LightstepCollectorHost       string    `yaml:"lightstep_collector_host"`
	LightstepMaximumSpans        int       `yaml:"lightstep_maximum_spans"`
	LightstepNumClients          int       `yaml:"lightstep_num_clients"`
	LightstepReconnectPeriod     string    `yaml:"lightstep_reconnect_period"`
	MetricMaxLength              int       `yaml:"metric_max_length"`
	MutexProfileFraction         int       `yaml:"mutex_profile_fraction"`
	NumReaders                   int       `yaml:"num_readers"`
	NumSpanWorkers               int       `yaml:"num_span_workers"`
	NumWorkers                   int       `yaml:"num_workers"`
	OmitEmptyHostname            bool      `yaml:"omit_empty_hostname"`
	Percentiles                  []float64 `yaml:"percentiles"`
	ReadBufferSizeBytes          int       `yaml:"read_buffer_size_bytes"`
	SentryDsn                    string    `yaml:"sentry_dsn"`
	SignalfxAPIKey               string    `yaml:"signalfx_api_key"`
	SignalfxEndpointBase         string    `yaml:"signalfx_endpoint_base"`
	SignalfxHostnameTag          string    `yaml:"signalfx_hostname_tag"`
	SignalfxPerTagAPIKeys        []struct {
		APIKey string `yaml:"api_key"`
		Name   string `yaml:"name"`
	} `yaml:"signalfx_per_tag_api_keys"`
	SignalfxVaryKeyBy             string   `yaml:"signalfx_vary_key_by"`
	SpanChannelCapacity           int      `yaml:"span_channel_capacity"`
	SsfBufferSize                 int      `yaml:"ssf_buffer_size"`
	SsfListenAddresses            []string `yaml:"ssf_listen_addresses"`
	StatsAddress                  string   `yaml:"stats_address"`
	StatsdListenAddresses         []string `yaml:"statsd_listen_addresses"`
	SynchronizeWithInterval       bool     `yaml:"synchronize_with_interval"`
	Tags                          []string `yaml:"tags"`
	TagsExclude                   []string `yaml:"tags_exclude"`
	TLSAuthorityCertificate       string   `yaml:"tls_authority_certificate"`
	TLSCertificate                string   `yaml:"tls_certificate"`
	TLSKey                        string   `yaml:"tls_key"`
	TraceLightstepAccessToken     string   `yaml:"trace_lightstep_access_token"`
	TraceLightstepCollectorHost   string   `yaml:"trace_lightstep_collector_host"`
	TraceLightstepMaximumSpans    int      `yaml:"trace_lightstep_maximum_spans"`
	TraceLightstepNumClients      int      `yaml:"trace_lightstep_num_clients"`
	TraceLightstepReconnectPeriod string   `yaml:"trace_lightstep_reconnect_period"`
	TraceMaxLengthBytes           int      `yaml:"trace_max_length_bytes"`
}
