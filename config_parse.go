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
	Aggregates:          []string{"min", "max", "count"},
	Interval:            10 * time.Second,
	MetricMaxLength:     4096,
	ReadBufferSizeBytes: 1048576 * 2, // 2 MiB
	SpanChannelCapacity: 100,
}

var defaultProxyConfig = ProxyConfig{
	MaxIdleConnsPerHost:          100,
	TracingClientCapacity:        1024,
	TracingClientFlushInterval:   500 * time.Millisecond,
	TracingClientMetricsInterval: time.Second,
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
	if c.TracingClientFlushInterval == 0 {
		c.TracingClientFlushInterval = defaultProxyConfig.TracingClientFlushInterval
	}
	if c.TracingClientMetricsInterval == 0 {
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
	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultConfig.ReadBufferSizeBytes
	}

	if c.SpanChannelCapacity == 0 {
		c.SpanChannelCapacity = defaultConfig.SpanChannelCapacity
	}
}
