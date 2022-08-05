package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
)

func ReadConfig[Config interface{}](
	path string, envBase string,
) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	fileData, err := ioutil.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	config := new(Config)
	err = yaml.UnmarshalStrict(fileData, config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %v", err)
	}

	err = envconfig.Process(envBase, config)
	if err != nil {
		return nil, fmt.Errorf("failed to process environment variables: %v", err)
	}

	return config, nil
}

func HandleConfigJson(config interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		encodedConfig, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.Header().Add("Content-Type", "application/json")
		w.Write(encodedConfig)
	}
}

func HandleConfigYaml(config interface{}) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		encodedConfig, err := yaml.Marshal(config)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(err.Error()))
			return
		}
		w.Header().Add("Content-Type", "application/x-yaml")
		w.Write(encodedConfig)
	}
}
