package main

import (
	"io/ioutil"
	"log"
	"time"

	"gopkg.in/yaml.v2"
)

type VeneurConfig struct {
	APIURL      string        `yaml:"api_url"`
	BufferSize  int           `yaml:"buffer_size"`
	Debug       bool          `yaml:"debug"`
	Expiry      time.Duration `yaml:"expiry"`
	Interval    time.Duration `yaml:"interval"`
	Key         string        `yaml:"key"`
	Percentiles []float64     `yaml:"percentiles"`
	UDPAddr     string        `yaml:"udp_address"`
	NumWorkers  int           `yaml:"num_workers"`
	SampleRate  float64       `yaml:"sample_rate"`
	StatsAddr   string        `yaml:"stats_address"`
	Tags        []string      `yaml:"tags"`
}

func ReadConfig(path string) (*VeneurConfig, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var config VeneurConfig

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	if config.Key == "" {
		log.Fatal("A Datadog API key is required in your config file!")
	}

	return &config, nil
}
