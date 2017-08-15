package veneur

type Config struct {
	Aggregates                    []string  `yaml:"aggregates"`
	APIHostname                   string    `yaml:"api_hostname"`
	AwsAccessKeyID                string    `yaml:"aws_access_key_id"`
	AwsRegion                     string    `yaml:"aws_region"`
	AwsS3Bucket                   string    `yaml:"aws_s3_bucket"`
	AwsSecretAccessKey            string    `yaml:"aws_secret_access_key"`
	DatadogAPIHostname            string    `yaml:"datadog_api_hostname"`
	DatadogAPIKey                 string    `yaml:"datadog_api_key"`
	DatadogTraceAPIAddress        string    `yaml:"datadog_trace_api_address"`
	Debug                         bool      `yaml:"debug"`
	EnableProfiling               bool      `yaml:"enable_profiling"`
	FlushFile                     string    `yaml:"flush_file"`
	FlushMaxPerBody               int       `yaml:"flush_max_per_body"`
	ForwardAddress                string    `yaml:"forward_address"`
	Hostname                      string    `yaml:"hostname"`
	HTTPAddress                   string    `yaml:"http_address"`
	InfluxAddress                 string    `yaml:"influx_address"`
	InfluxConsistency             string    `yaml:"influx_consistency"`
	InfluxDBName                  string    `yaml:"influx_db_name"`
	Interval                      string    `yaml:"interval"`
	Key                           string    `yaml:"key"`
	MetricMaxLength               int       `yaml:"metric_max_length"`
	NumReaders                    int       `yaml:"num_readers"`
	NumWorkers                    int       `yaml:"num_workers"`
	OmitEmptyHostname             bool      `yaml:"omit_empty_hostname"`
	Percentiles                   []float64 `yaml:"percentiles"`
	ReadBufferSizeBytes           int       `yaml:"read_buffer_size_bytes"`
	SentryDsn                     string    `yaml:"sentry_dsn"`
	SsfAddress                    string    `yaml:"ssf_address"`
	SsfBufferSize                 int       `yaml:"ssf_buffer_size"`
	StatsAddress                  string    `yaml:"stats_address"`
	Tags                          []string  `yaml:"tags"`
	TcpAddress                    string    `yaml:"tcp_address"`
	TLSAuthorityCertificate       string    `yaml:"tls_authority_certificate"`
	TLSCertificate                string    `yaml:"tls_certificate"`
	TLSKey                        string    `yaml:"tls_key"`
	TraceAddress                  string    `yaml:"trace_address"`
	TraceAPIAddress               string    `yaml:"trace_api_address"`
	TraceLightstepAccessToken     string    `yaml:"trace_lightstep_access_token"`
	TraceLightstepCollectorHost   string    `yaml:"trace_lightstep_collector_host"`
	TraceLightstepMaximumSpans    int       `yaml:"trace_lightstep_maximum_spans"`
	TraceLightstepReconnectPeriod string    `yaml:"trace_lightstep_reconnect_period"`
	TraceMaxLengthBytes           int       `yaml:"trace_max_length_bytes"`
	UdpAddress                    string    `yaml:"udp_address"`
}
