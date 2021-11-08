package util_test

import (
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
