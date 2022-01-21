package util_test

import (
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v3"
)

type urlYamlStruct struct {
	Url util.Url `yaml:"url"`
}

func TestUrlUnmarshalYAML(t *testing.T) {
	yamlFile := []byte(`url: https://example.com/path`)
	data := urlYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, url.URL{
		Scheme: "https",
		Host:   "example.com",
		Path:   "/path",
	}, *data.Url.Value)
}

func TestUrlUnmarshalYAMLUnset(t *testing.T) {
	yamlFile := []byte("")
	data := urlYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Nil(t, data.Url.Value)
}

func TestUrlDecode(t *testing.T) {
	defer os.Unsetenv("ENVCONFIG_TEST_URL")
	os.Setenv("ENVCONFIG_TEST_URL", "https://example.com/path")

	data := urlYamlStruct{}
	err := envconfig.Process("ENVCONFIG_TEST", &data)
	assert.Nil(t, err)
	assert.Equal(t, url.URL{
		Scheme: "https",
		Host:   "example.com",
		Path:   "/path",
	}, *data.Url.Value)
}

func TestUrlMarshalJSON(t *testing.T) {
	url := util.Url{
		Value: &url.URL{
			Scheme: "https",
			Host:   "example.com",
		},
	}
	value, err := json.Marshal(url)
	assert.Nil(t, err)
	assert.Equal(t, `"https://example.com"`, string(value))
}

func TestUrlMarshalJSONUnset(t *testing.T) {
	url := util.Url{}
	value, err := json.Marshal(url)
	assert.Nil(t, err)
	assert.Equal(t, "null", string(value))
}

func TestUrlMarshalYAML(t *testing.T) {
	url := util.Url{
		Value: &url.URL{
			Scheme: "https",
			Host:   "example.com",
		},
	}
	value, err := yaml.Marshal(url)
	assert.Nil(t, err)
	assert.Equal(t, "https://example.com\n", string(value))
}

func TestUrlMarshalYAMLUnset(t *testing.T) {
	url := util.Url{}
	value, err := yaml.Marshal(url)
	assert.Nil(t, err)
	assert.Equal(t, "\"\"\n", string(value))
}
