package config_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/util/config"
)

type Config struct {
	Key1 string
}

func TestReadConfigWithoutTemplate(t *testing.T) {
	file, err := ioutil.TempFile("", "config.yaml")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	file.Write([]byte("key1: value1"))

	parsedConfig, err :=
		config.ReadConfig[Config](file.Name(), nil, "")
	require.NoError(t, err)

	assert.Equal(t, "value1", parsedConfig.Key1)
}

type ConfigParams struct {
	ConfigValue string
}

func TestReadConfigWithTemplate(t *testing.T) {
	file, err := ioutil.TempFile("", "config.yaml")
	require.NoError(t, err)
	defer os.Remove(file.Name())

	file.Write([]byte("key1: {{.ConfigValue}}"))

	parsedConfig, err :=
		config.ReadConfig[Config](file.Name(), ConfigParams{
			ConfigValue: "value1",
		}, "")
	require.NoError(t, err)

	assert.Equal(t, "value1", parsedConfig.Key1)
}
