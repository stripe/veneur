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
	return readConfig(f)
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

	if c.Hostname == "" && !c.OmitEmptyHostname {
		c.Hostname, _ = os.Hostname()
	}

	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = defaultBufferSizeBytes
	}

	// pass back an error about any unknown fields:
	return c, unmarshalErr
}

// ParseInterval handles parsing the flush interval as a time.Duration
func (c Config) ParseInterval() (time.Duration, error) {
	return time.ParseDuration(c.Interval)
}
