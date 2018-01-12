package veneur

import (
	"io"
	"io/ioutil"
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
	return c, nil
}

// ParseInterval handles parsing the flush interval as a time.Duration
func (c Config) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
