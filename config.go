package veneur

import (
	"io/ioutil"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// VenerConfig is a collection of settings that control Veneur.
type VeneurConfig struct {
	APIHostname         string        `yaml:"api_hostname"`
	Debug               bool          `yaml:"debug"`
	Hostname            string        `yaml:"hostname"`
	Interval            time.Duration `yaml:"interval"`
	Key                 string        `yaml:"key"`
	MetricMaxLength     int           `yaml:"metric_max_length"`
	Percentiles         []float64     `yaml:"percentiles"`
	ReadBufferSizeBytes int           `yaml:"read_buffer_size_bytes"`
	SetSize             uint          `yaml:"set_size"`
	SetAccuracy         float64       `yaml:"set_accuracy"`
	UDPAddr             string        `yaml:"udp_address"`
	NumWorkers          int           `yaml:"num_workers"`
	StatsAddr           string        `yaml:"stats_address"`
	Tags                []string      `yaml:"tags"`
}

// Config is the global config that we'll use once it's inited.
var Config *VeneurConfig

// ReadConfig unmarshals the config file and slurps in it's data.
func ReadConfig(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, &Config)
	if err != nil {
		return err
	}

	if Config.Key == "" {
		log.Fatal("A Datadog API key is required in your config file!")
	}

	if Config.Hostname == "" {
		Config.Hostname, _ = os.Hostname()
	}

	if Config.ReadBufferSizeBytes == 0 {
		Config.ReadBufferSizeBytes = 1048576 * 2 // 2 MB
	}

	return nil
}
