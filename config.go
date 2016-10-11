package veneur

import (
	"io"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// Config is a collection of settings that control Veneur.
type Config struct {
	APIHostname         string        `yaml:"api_hostname"`
	Debug               bool          `yaml:"debug"`
	Hostname            string        `yaml:"hostname"`
	Interval            time.Duration `yaml:"interval"`
	Key                 string        `yaml:"key"`
	MetricMaxLength     int           `yaml:"metric_max_length"`
	Percentiles         []float64     `yaml:"percentiles"`
	ReadBufferSizeBytes int           `yaml:"read_buffer_size_bytes"`
	UDPAddr             string        `yaml:"udp_address"`
	HTTPAddr            string        `yaml:"http_address"`
	ForwardAddr         string        `yaml:"forward_address"`
	NumWorkers          int           `yaml:"num_workers"`
	NumReaders          int           `yaml:"num_readers"`
	StatsAddr           string        `yaml:"stats_address"`
	Tags                []string      `yaml:"tags"`
	SentryDSN           string        `yaml:"sentry_dsn"`
	FlushLimit          int           `yaml:"flush_max_per_body"`
	AWSAccessKeyId      string        `yaml:"aws_access_key_id"`
	AWSSecretAccessKey  string        `yaml:"aws_secret_access_key"`
	AWSRegion           string        `yaml:"aws_region"`
	AWSBucket           string        `yaml:"aws_bucket"`
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

	if c.Hostname == "" {
		c.Hostname, _ = os.Hostname()
	}

	if c.ReadBufferSizeBytes == 0 {
		c.ReadBufferSizeBytes = 1048576 * 2 // 2 MB
	}

	return c, nil
}
