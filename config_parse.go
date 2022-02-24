package veneur

import (
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/sirupsen/logrus"

	"gopkg.in/yaml.v2"
)

var defaultConfig = Config{
	Aggregates:                     []string{"min", "max", "count"},
	DatadogFlushMaxPerBody:         25000,
	Interval:                       time.Duration(10 * time.Second),
	MetricMaxLength:                4096,
	PrometheusNetworkType:          "tcp",
	ReadBufferSizeBytes:            1048576 * 2, // 2 MiB
	SpanChannelCapacity:            100,
	SplunkHecBatchSize:             100,
	SplunkHecMaxConnectionLifetime: time.Duration(10 * time.Second), // same as Interval
}

var defaultProxyConfig = ProxyConfig{
	MaxIdleConnsPerHost:          100,
	TracingClientCapacity:        1024,
	TracingClientFlushInterval:   "500ms",
	TracingClientMetricsInterval: "1s",
}

// ReadProxyConfig unmarshals the proxy config file and slurps in its data.
func ReadProxyConfig(
	logger *logrus.Entry, path string,
) (c ProxyConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	c, err = readProxyConfig(f)
	c.applyDefaults(logger)
	return
}

func (c *ProxyConfig) applyDefaults(logger *logrus.Entry) {
	if c.MaxIdleConnsPerHost == 0 {
		// It's dangerous to leave this as the default. Since veneur-proxy is
		// designed for HA environments with lots of globalstats backends we
		// need higher defaults lest we have too small a connection pool and cause
		// a glut of TIME_WAIT connections, so set a default.
		logger.WithField(
			"new value", defaultProxyConfig.MaxIdleConnsPerHost,
		).Warn("max_idle_conns_per_host being unset may lead to unsafe operations, defaulting!")
		c.MaxIdleConnsPerHost = defaultProxyConfig.MaxIdleConnsPerHost
	}
	if c.TracingClientCapacity == 0 {
		c.TracingClientCapacity = defaultProxyConfig.TracingClientCapacity
	}
	if c.TracingClientFlushInterval == "" {
		c.TracingClientFlushInterval = defaultProxyConfig.TracingClientFlushInterval
	}
	if c.TracingClientMetricsInterval == "" {
		c.TracingClientMetricsInterval = defaultProxyConfig.TracingClientMetricsInterval
	}
}

func readProxyConfig(r io.Reader) (ProxyConfig, error) {
	var c ProxyConfig
	bts, err := ioutil.ReadAll(r)
	if err != nil {
		return c, err
	}
	unmarshalErr := unmarshalSemiStrictly(bts, &c)
	if unmarshalErr != nil {
		if _, ok := err.(*UnknownConfigKeys); !ok {
			return c, unmarshalErr
		}
	}

	err = envconfig.Process("veneur_proxy", &c)
	if err != nil {
		return c, err
	}

	return c, unmarshalErr
}

// UnknownConfigKeys represents a failure to strictly parse a
// configuration YAML file has failed, indicating that the file
// contains unknown keys.
type UnknownConfigKeys struct {
	err error
}

func (e *UnknownConfigKeys) Error() string {
	return e.err.Error()
}

// ReadConfig unmarshals the config file and slurps in its
// data. ReadConfig can return an error of type *UnknownConfigKeys,
// which means that the file is usable, but contains unknown fields.
func ReadConfig(logger *logrus.Entry, path string) (c Config, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()

	c, err = readConfig(f)
	c.applyDefaults(logger)
	return
}

func unmarshalSemiStrictly(bts []byte, into interface{}) error {
	strictErr := yaml.UnmarshalStrict(bts, into)
	if strictErr == nil {
		return nil
	}

	looseErr := yaml.Unmarshal(bts, into)
	if looseErr != nil {
		return looseErr
	}
	return &UnknownConfigKeys{strictErr}
}

func readConfig(r io.Reader) (Config, error) {
	var c Config
	// Unfortunately the YAML package does not
	// support reader inputs
	// TODO(aditya) convert this when the
	// upstream PR lands
	bts, err := ioutil.ReadAll(r)
	if err != nil {
		return c, err
	}
	unmarshalErr := unmarshalSemiStrictly(bts, &c)
	if unmarshalErr != nil {
		if _, ok := err.(*UnknownConfigKeys); !ok {
			return c, unmarshalErr
		}
	}

	err = envconfig.Process("veneur", &c)
	if err != nil {
		return c, err
	}

	// pass back an error about any unknown fields:
	return c, unmarshalErr
}

func (c *Config) applyDefaults(logger *logrus.Entry) {
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
	if c.PrometheusNetworkType == "" {
		c.PrometheusNetworkType = defaultConfig.PrometheusNetworkType
	}
	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultConfig.ReadBufferSizeBytes
	}
	if c.SsfBufferSize != 0 {
		logger.Warn(
			"ssf_buffer_size configuration option has been replaced by datadog_span_buffer_size and will be removed in the next version")
		if c.DatadogSpanBufferSize == 0 {
			c.DatadogSpanBufferSize = c.SsfBufferSize
		}
	}
	if c.FlushMaxPerBody != 0 {
		logger.Warn(
			"flush_max_per_body configuration option has been replaced by datadog_flush_max_per_body and will be removed in the next version")
		if c.DatadogFlushMaxPerBody == 0 {
			c.DatadogFlushMaxPerBody = c.FlushMaxPerBody
		}
	}
	if c.TraceLightstepNumClients != 0 {
		logger.Warn(
			"trace_lightstep_num_clients configuration option has been replaced by lightstep_num_clients and will be removed in the next version")
		if c.LightstepNumClients == 0 {
			c.LightstepNumClients = c.TraceLightstepNumClients
		}
	}
	if c.TraceLightstepCollectorHost.Value != nil {
		logger.Warn(
			"trace_lightstep_collector_host configuration option has been replaced by lightstep_collector_host and will be removed in the next version")
		if c.LightstepCollectorHost.Value == nil {
			c.LightstepCollectorHost = c.TraceLightstepCollectorHost
		}
	}
	if c.TraceLightstepAccessToken.Value != "" {
		logger.Warn(
			"trace_lightstep_access_token configuration option has been replaced by lightstep_access_token and will be removed in the next version")
		if c.LightstepAccessToken.Value == "" {
			c.LightstepAccessToken = c.TraceLightstepAccessToken
		}
	}
	if c.TraceLightstepMaximumSpans != 0 {
		logger.Warn(
			"trace_lightstep_maximum_spans configuration option has been replaced by lightstep_maximum_spans and will be removed in the next version")
		if c.LightstepMaximumSpans == 0 {
			c.LightstepMaximumSpans = c.TraceLightstepMaximumSpans
		}
	}
	if c.TraceLightstepReconnectPeriod != 0 {
		logger.Warn(
			"trace_lightstep_reconnect_period configuration option has been replaced by lightstep_reconnect_period and will be removed in the next version")
		if c.LightstepReconnectPeriod == 0 {
			c.LightstepReconnectPeriod = c.TraceLightstepReconnectPeriod
		}
	}

	if c.DatadogFlushMaxPerBody == 0 {
		c.DatadogFlushMaxPerBody = defaultConfig.DatadogFlushMaxPerBody
	}

	if c.SpanChannelCapacity == 0 {
		c.SpanChannelCapacity = defaultConfig.SpanChannelCapacity
	}

	if c.SplunkHecBatchSize == 0 {
		c.SplunkHecBatchSize = defaultConfig.SplunkHecBatchSize
	}

	if c.SplunkHecMaxConnectionLifetime == 0 {
		c.SplunkHecMaxConnectionLifetime = defaultConfig.SplunkHecMaxConnectionLifetime
	}
}
