package veneur

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"

	"gopkg.in/yaml.v2"
)

const defaultBufferSizeBytes = 1048576 * 2 // 2 MB

func unmarshalSemiStrictly(bts []byte, into interface{}) (err error, strictErr error) {
	strictErr = yaml.UnmarshalStrict(bts, into)
	if strictErr != nil {
		err = yaml.Unmarshal(bts, into)
	}
	return
}

// ReadProxyConfig unmarshals the proxy config file and slurps in its data.
func ReadProxyConfig(path string) (c ProxyConfig, err error, warnings []string) {
	f, err := os.Open(path)
	if err != nil {
		return c, err, warnings
	}
	defer f.Close()

	bts, err := ioutil.ReadAll(f)
	if err != nil {
		return
	}
	err, strictErr := unmarshalSemiStrictly(bts, &c)
	if err != nil {
		return c, err, warnings
	}
	if strictErr != nil {
		warnings = append(warnings, strictErr.Error())
	}
	err = envconfig.Process("veneur", &c)
	if err != nil {
		return
	}

	return
}

// ReadConfig unmarshals the config file and slurps in it's data.
func ReadConfig(path string) (c Config, err error, warnings []string) {
	f, err := os.Open(path)
	if err != nil {
		return c, err, []string{}
	}
	defer f.Close()
	return readConfig(f)
}

func readConfig(r io.Reader) (c Config, err error, warnings []string) {
	// Unfortunately the YAML package does not
	// support reader inputs
	// TODO(aditya) convert this when the
	// upstream PR lands
	bts, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	err, strictErr := unmarshalSemiStrictly(bts, &c)
	if err != nil {
		return c, err, warnings
	}
	if strictErr != nil {
		warnings = append(warnings, fmt.Sprintf("Unknown fields encountered in config: %v", err))
	}

	err = envconfig.Process("veneur", &c)
	if err != nil {
		return c, err, warnings
	}

	if c.Hostname == "" && !c.OmitEmptyHostname {
		c.Hostname, _ = os.Hostname()
	}

	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultBufferSizeBytes
	}

	if c.Key != "" {
		warnings = append(warnings, "The config key `key` is deprecated and replaced with `datadog_api_key` and will be removed in 2.0!")
		// If they set the DatadogAPIKey, favor it. Otherwise, replace it.
		if c.DatadogAPIKey == "" {
			c.DatadogAPIKey = c.Key
		}
	}

	if c.APIHostname != "" {
		warnings = append(warnings, "The config key `api_hostname` is deprecated and replaced with `datadog_api_hostname` and will be removed in 2.0!")
		if c.DatadogAPIHostname == "" {
			c.DatadogAPIHostname = c.APIHostname
		}
	}

	if c.TraceAPIAddress != "" {
		warnings = append(warnings, "The config key `trace_api_address` is deprecated and replaced with `datadog_trace_api_hostname` and will be removed in 2.0!")
		if c.DatadogTraceAPIAddress == "" {
			c.DatadogTraceAPIAddress = c.TraceAPIAddress
		}
	}

	if c.TraceAddress != "" {
		warnings = append(warnings, "The config key `trace_address` is deprecated and replaced with `ssf_address` and will be removed in 2.0!")
		if c.SsfAddress == "" {
			c.SsfAddress = c.TraceAddress
		}
	}

	var moreAddrs []string
	if c.UdpAddress != "" {
		if len(c.StatsdListenAddresses) > 0 {
			err = fmt.Errorf("`statsd_listen_addresses` and deprecated parameter `udp_address` are both present")
			return
		}
		warnings = append(warnings, "The config key `udp_address` is deprecated and replaced with entries in `statsd_listen_addresses` and will be removed in 2.0!")
		moreAddrs = append(moreAddrs, (&url.URL{Scheme: "udp", Host: c.UdpAddress}).String())
	}

	if c.TcpAddress != "" {
		if len(c.StatsdListenAddresses) > 0 {
			err = fmt.Errorf("`statsd_listen_addresses` and deprecated parameter `tcp_address` are both present")
			return
		}
		warnings = append(warnings, "The config key `tcp_address` is deprecated and replaced with entries in `statsd_listen_addresses` and will be removed in 2.0!")
		moreAddrs = append(moreAddrs, (&url.URL{Scheme: "tcp", Host: c.TcpAddress}).String())
	}
	c.StatsdListenAddresses = append(c.StatsdListenAddresses, moreAddrs...)

	return c, nil, warnings
}

// ParseInterval handles parsing the flush interval as a time.Duration
func (c Config) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
