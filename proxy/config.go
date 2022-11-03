package proxy

import (
	"time"

	"github.com/stripe/veneur/v14/util/matcher"
)

type Config struct {
	Debug             bool          `yaml:"debug"`
	DialTimeout       time.Duration `yaml:"dial_timeout"`
	DiscoveryInterval time.Duration `yaml:"discovery_interval"`
	ForwardAddresses  []string      `yaml:"forward_addresses"`
	ForwardService    string        `yaml:"forward_service"`
	GrpcServer        struct {
		ConnectionTimeout     time.Duration `yaml:"connection_timeout"`
		MaxConnectionIdle     time.Duration `yaml:"max_connection_idle"`
		MaxConnectionAge      time.Duration `yaml:"max_connection_age"`
		MaxConnectionAgeGrace time.Duration `yaml:"max_connection_age_grace"`
		PingTimeout           time.Duration `yaml:"ping_timeout"`
		KeepaliveTimeout      time.Duration `yaml:"keepalive_timeout"`
	} `yaml:"grpc_server"`
	GrpcAddress string `yaml:"grpc_address"`
	Http        struct {
		EnableConfig    bool `yaml:"enable_config"`
		EnableProfiling bool `yaml:"enable_profiling"`
	} `yaml:"http"`
	HttpAddress            string               `yaml:"http_address"`
	IgnoreTags             []matcher.TagMatcher `yaml:"ignore_tags"`
	RuntimeMetricsInterval time.Duration        `yaml:"runtime_metrics_interval"`
	SendBufferSize         uint                 `yaml:"send_buffer_size"`
	SentryDsn              string               `yaml:"sentry_dsn"`
	ShutdownTimeout        time.Duration        `yaml:"shutdown_timeout"`
	Statsd                 struct {
		Address             string        `yaml:"address"`
		AggregationInterval time.Duration `yaml:"aggregation_interval"`
		ChannelBufferSize   int           `yaml:"channel_buffer_size"`
		MessagesPerPayload  int           `yaml:"messages_per_payload"`
	} `yaml:"statsd"`
}
