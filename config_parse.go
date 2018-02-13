package veneur

import (
	"io"
	"io/ioutil"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"

	"gopkg.in/yaml.v2"
)

var defaultConfig = Config{
	Aggregates:          []string{"min", "max", "count"},
	FlushMaxPerBody:     25000,
	Interval:            "10s",
	MetricMaxLength:     4096,
	ReadBufferSizeBytes: 1048576 * 2, // 2 MiB
}

// ReadProxyConfig unmarshals the proxy config file and slurps in its data.
func ReadProxyConfig(path string) (c ProxyConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	return readProxyConfig(f)
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

	err = envconfig.Process("veneur", &c)
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
func ReadConfig(path string) (c Config, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()

	c, err = readConfig(f)
	c.applyDefaults()
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

func (c *Config) applyDefaults() {
	if len(c.Aggregates) == 0 {
		c.Aggregates = defaultConfig.Aggregates
	}
	if c.Hostname == "" && !c.OmitEmptyHostname {
		c.Hostname, _ = os.Hostname()
	}
	if c.Interval == "" {
		c.Interval = defaultConfig.Interval
	}
	if c.MetricMaxLength == 0 {
		c.MetricMaxLength = defaultConfig.MetricMaxLength
	}
	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultConfig.ReadBufferSizeBytes
	}
	if c.SsfBufferSize != 0 {
		log.Warn("ssf_buffer_size configuration option has been replaced by datadog_span_buffer_size and will be removed in the next version")
		if c.DatadogSpanBufferSize == 0 {
			c.DatadogSpanBufferSize = c.SsfBufferSize
		}
	}
	if c.FlushMaxPerBody != 0 {
		log.Warn("flush_max_per_body configuration option has been replaced by datadog_flush_max_per_body and will be removed in the next version")
		if c.DatadogFlushMaxPerBody == 0 {
			c.DatadogFlushMaxPerBody = c.FlushMaxPerBody
		}
	}
	if c.TraceLightstepNumClients != 0 {
		log.Warn("trace_lightstep_num_clients configuration option has been replaced by lightstep_num_clients and will be removed in the next version")
		if c.LightstepNumClients == 0 {
			c.LightstepNumClients = c.TraceLightstepNumClients
		}
	}
	if c.TraceLightstepCollectorHost != "" {
		log.Warn("trace_lightstep_collector_host configuration option has been replaced by lightstep_collector_host and will be removed in the next version")
		if c.LightstepCollectorHost == "" {
			c.LightstepCollectorHost = c.TraceLightstepCollectorHost
		}
	}
	if c.TraceLightstepAccessToken != "" {
		log.Warn("trace_lightstep_access_token configuration option has been replaced by lightstep_access_token and will be removed in the next version")
		if c.LightstepAccessToken == "" {
			c.LightstepAccessToken = c.TraceLightstepAccessToken
		}
	}
	if c.TraceLightstepMaximumSpans != 0 {
		log.Warn("trace_lightstep_maximum_spans configuration option has been replaced by lightstep_maximum_spans and will be removed in the next version")
		if c.LightstepMaximumSpans == 0 {
			c.LightstepMaximumSpans = c.TraceLightstepMaximumSpans
		}
	}
	if c.TraceLightstepReconnectPeriod != "" {
		log.Warn("trace_lightstep_reconnect_period configuration option has been replaced by lightstep_reconnect_period and will be removed in the next version")
		if c.LightstepReconnectPeriod == "" {
			c.LightstepReconnectPeriod = c.TraceLightstepReconnectPeriod
		}
	}
}

// ParseInterval handles parsing the flush interval as a time.Duration
func (c Config) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
