package veneur

type Config struct {
	Aggregates               []string  `yaml:"aggregates"`
	APIHostname              string    `yaml:"api_hostname"`
	AwsAccessKeyID           string    `yaml:"aws_access_key_id"`
	AwsRegion                string    `yaml:"aws_region"`
	AwsS3Bucket              string    `yaml:"aws_s3_bucket"`
	AwsSecretAccessKey       string    `yaml:"aws_secret_access_key"`
	ConsulForwardServiceName string    `yaml:"consul_forward_service_name"`
	ConsulRefreshInterval    string    `yaml:"consul_refresh_interval"`
	ConsulTraceServiceName   string    `yaml:"consul_trace_service_name"`
	Debug                    bool      `yaml:"debug"`
	EnableProfiling          bool      `yaml:"enable_profiling"`
	FlushMaxPerBody          int       `yaml:"flush_max_per_body"`
	ForwardAddress           string    `yaml:"forward_address"`
	Hostname                 string    `yaml:"hostname"`
	HTTPAddress              string    `yaml:"http_address"`
	InfluxAddress            string    `yaml:"influx_address"`
	InfluxConsistency        string    `yaml:"influx_consistency"`
	InfluxDBName             string    `yaml:"influx_db_name"`
	Interval                 string    `yaml:"interval"`
	Key                      string    `yaml:"key"`
	MetricMaxLength          int       `yaml:"metric_max_length"`
	NumReaders               int       `yaml:"num_readers"`
	NumWorkers               int       `yaml:"num_workers"`
	OmitEmptyHostname        bool      `yaml:"omit_empty_hostname"`
	Percentiles              []float64 `yaml:"percentiles"`
	ReadBufferSizeBytes      int       `yaml:"read_buffer_size_bytes"`
	SentryDsn                string    `yaml:"sentry_dsn"`
	StatsAddress             string    `yaml:"stats_address"`
	Tags                     []string  `yaml:"tags"`
	TraceAddress             string    `yaml:"trace_address"`
	TraceAPIAddress          string    `yaml:"trace_api_address"`
	TraceMaxLengthBytes      int       `yaml:"trace_max_length_bytes"`
	UdpAddress               string    `yaml:"udp_address"`
}
