package veneur

type ProxyConfig struct {
	ConsulForwardGrpcServiceName string `yaml:"consul_forward_grpc_service_name"`
	ConsulForwardServiceName     string `yaml:"consul_forward_service_name"`
	ConsulRefreshInterval        string `yaml:"consul_refresh_interval"`
	ConsulTraceServiceName       string `yaml:"consul_trace_service_name"`
	Debug                        bool   `yaml:"debug"`
	EnableProfiling              bool   `yaml:"enable_profiling"`
	ForwardAddress               string `yaml:"forward_address"`
	ForwardTimeout               string `yaml:"forward_timeout"`
	GrpcAddress                  string `yaml:"grpc_address"`
	GrpcForwardAddress           string `yaml:"grpc_forward_address"`
	HTTPAddress                  string `yaml:"http_address"`
	RuntimeMetricsInterval       string `yaml:"runtime_metrics_interval"`
	SentryDsn                    string `yaml:"sentry_dsn"`
	SsfDestinationAddress        string `yaml:"ssf_destination_address"`
	StatsAddress                 string `yaml:"stats_address"`
	TraceAddress                 string `yaml:"trace_address"`
	TraceAPIAddress              string `yaml:"trace_api_address"`
}
