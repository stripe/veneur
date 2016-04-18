package veneur

import (
	"io/ioutil"
	"log"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

type VeneurConfig struct {
	APIHostname          string        `yaml:"api_hostname"`
	Debug                bool          `yaml:"debug"`
	MetricExpiryDuration time.Duration `yaml:"metric_expiry_duration"`
	Hostname             string        `yaml:"hostname"`
	Interval             time.Duration `yaml:"interval"`
	Key                  string        `yaml:"key"`
	MetricMaxLength      int           `yaml:"metric_max_length"`
	Percentiles          []float64     `yaml:"percentiles"`
	SetSize              uint          `yaml:"set_size"`
	SetAccuracy          float64       `yaml:"set_accuracy"`
	UDPAddr              string        `yaml:"udp_address"`
	NumWorkers           int           `yaml:"num_workers"`
	SampleRate           float64       `yaml:"sample_rate"`
	StatsAddr            string        `yaml:"stats_address"`
	Tags                 []string      `yaml:"tags"`
}

var Config *VeneurConfig

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

	return nil
}
