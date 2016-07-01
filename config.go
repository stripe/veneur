package veneur

import (
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
	NumWorkers          int           `yaml:"num_workers"`
	NumReaders          int           `yaml:"num_readers"`
	StatsAddr           string        `yaml:"stats_address"`
	Tags                []string      `yaml:"tags"`
	SentryDSN           string        `yaml:"sentry_dsn"`
	HistCounters        bool          `yaml:"publish_histogram_counters"`
	FlushLimit          int           `yaml:"flush_max_per_body"`
}

// ReadConfig unmarshals the config file and slurps in it's data.
func ReadConfig(path string) (ret Config, err error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return
	}

	err = yaml.Unmarshal(data, &ret)
	if err != nil {
		return
	}

	if ret.Hostname == "" {
		ret.Hostname, _ = os.Hostname()
	}

	if ret.ReadBufferSizeBytes == 0 {
		ret.ReadBufferSizeBytes = 1048576 * 2 // 2 MB
	}

	return
}
