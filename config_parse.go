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

// ReadProxyConfig unmarshals the proxy config file and slurps in its data.
func ReadProxyConfig(path string) (c ProxyConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()

	bts, err := ioutil.ReadAll(f)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(bts, &c)
	if err != nil {
		return
	}

	err = envconfig.Process("veneur", &c)
	if err != nil {
		return c, err
	}

	return c, nil
}

// ReadConfig unmarshals the config file and slurps in it's data.
func ReadConfig(path string) (c Config, err error) {
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	return readConfig(f)
}

func readConfig(r io.Reader) (c Config, err error) {
	// Unfortunately the YAML package does not
	// support reader inputs
	// TODO(aditya) convert this when the
	// upstream PR lands
	bts, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(bts, &c)
	if err != nil {
		return
	}

	err = envconfig.Process("veneur", &c)
	if err != nil {
		return c, err
	}

	if c.Hostname == "" && !c.OmitEmptyHostname {
		c.Hostname, _ = os.Hostname()
	}

	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultBufferSizeBytes
	}

	if c.Key != "" {
		log.Warn("The config key `key` is deprecated and replaced with `datadog_api_key` and will be removed in 2.0!")
		// If they set the DatadogAPIKey, favor it. Otherwise, replace it.
		if c.DatadogAPIKey == "" {
			c.DatadogAPIKey = c.Key
		}
	}

	if c.APIHostname != "" {
		log.Warn("The config key `api_hostname` is deprecated and replaced with `datadog_api_hostname` and will be removed in 2.0!")
		if c.DatadogAPIHostname == "" {
			c.DatadogAPIHostname = c.APIHostname
		}
	}

	if c.TraceAPIAddress != "" {
		log.Warn("The config key `trace_api_address` is deprecated and replaced with `datadog_trace_api_hostname` and will be removed in 2.0!")
		if c.DatadogTraceAPIAddress == "" {
			c.DatadogTraceAPIAddress = c.TraceAPIAddress
		}
	}

	if c.TraceAddress != "" {
		log.Warn("The config key `trace_address` is deprecated and replaced with `ssf_address` and will be removed in 2.0!")
		if c.SsfAddress == "" {
			c.SsfAddress = c.TraceAddress
		}
	}

	var statsdAddrs []string
	if c.UdpAddress != "" {
		if len(c.StatsdListenAddresses) > 0 {
			err = fmt.Errorf("`statsd_listen_addresses` and deprecated parameter `udp_address` are both present")
			return
		}
		log.Warn("The config key `udp_address` is deprecated and replaced with entries in `statsd_listen_addresses` and will be removed in 2.0!")
		statsdAddrs = append(statsdAddrs, (&url.URL{Scheme: "udp", Host: c.UdpAddress}).String())
	}

	if c.TcpAddress != "" {
		if len(c.StatsdListenAddresses) > 0 {
			err = fmt.Errorf("`statsd_listen_addresses` and deprecated parameter `tcp_address` are both present")
			return
		}
		log.Warn("The config key `tcp_address` is deprecated and replaced with entries in `statsd_listen_addresses` and will be removed in 2.0!")
		statsdAddrs = append(statsdAddrs, (&url.URL{Scheme: "tcp", Host: c.TcpAddress}).String())
	}
	c.StatsdListenAddresses = append(c.StatsdListenAddresses, statsdAddrs...)

	var ssfAddrs []string
	if c.SsfAddress != "" {
		if len(c.SsfListenAddresses) > 0 {
			err = fmt.Errorf("`ssf_listen_addresses` and deprecated parameter `ssf_address` are both present")
			return
		}
		log.Warn("The config key `ssf_address` is deprecated and replaced with entries in `ssf_listen_addresses` and will be removed in 2.0!")
		ssfAddrs = append(statsdAddrs, (&url.URL{Scheme: "udp", Host: c.SsfAddress}).String())
	}
	c.SsfListenAddresses = append(c.SsfListenAddresses, ssfAddrs...)

	return c, nil
}

// ParseInterval handles parsing the flush interval as a time.Duration
func (c Config) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
