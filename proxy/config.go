package proxy

import (
	"time"

	"github.com/stripe/veneur/v14/util/matcher"
)

type Config struct {
	Debug                  bool          `yaml:"debug"`
	RuntimeMetricsInterval time.Duration `yaml:"runtime_metrics_interval"`
	DialTimeout            time.Duration `yaml:"dial_timeout"`
	DiscoveryInterval      time.Duration `yaml:"discovery_interval"`
	ForwardAddresses       []string      `yaml:"forward_addresses"`
	ForwardService         string        `yaml:"forward_service"`
	GrpcAddress            string        `yaml:"grpc_address"`
	Http                   struct {
		EnableConfig    bool `yaml:"enable_config"`
		EnableProfiling bool `yaml:"enable_profiling"`
	} `yaml:"http"`
	HttpAddress     string               `yaml:"http_address"`
	IgnoreTags      []matcher.TagMatcher `yaml:"ignore_tags"`
	SentryDsn       string               `yaml:"sentry_dsn"`
	ShutdownTimeout time.Duration        `yaml:"shutdown_timeout"`
	StatsAddress    string               `yaml:"stats_address"`
}
