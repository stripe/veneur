package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/template"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v2"
)

func ReadConfig[Config interface{}](
	path string, templateData interface{}, envBase string,
) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	configTemplate, err := template.New("config").Parse(string(fileData))
	if err != nil {
		return nil, err
	}

	configReader, configWriter := io.Pipe()
	templateErr := make(chan error)
	go func() {
		err := configTemplate.Execute(configWriter, templateData)
		configWriter.Close()
		templateErr <- err
	}()

	decoder := yaml.NewDecoder(configReader)
	decoder.SetStrict(true)

	config := new(Config)
	err = decoder.Decode(config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config file: %v", err)
	}

	err = <-templateErr
	if err != nil {
		return nil, err
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
