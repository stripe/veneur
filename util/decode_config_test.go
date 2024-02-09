package util_test

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/util"
)

func TestDecodeConfigYaml(t *testing.T) {
	type configStruct struct {
		ConfigItem string `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "config-value",
	}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, "config-value", result.ConfigItem)
}

func TestDecodeConfigEnvironment(t *testing.T) {
	type configStruct struct {
		ConfigItem string `yaml:"config_item"`
	}

	os.Setenv("NAME_CONFIGITEM", "config-value")
	defer os.Unsetenv("NAME_CONFIGITEM")
	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, "config-value", result.ConfigItem)
}

func TestDecodeConfigWithStringSecretYaml(t *testing.T) {
	type configStruct struct {
		ConfigItem util.StringSecret `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "config-value",
	}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, "config-value", result.ConfigItem.Value)
}

func TestDecodeConfigWithStringSecretEnvironment(t *testing.T) {
	type configStruct struct {
		ConfigItem util.StringSecret `yaml:"config_item"`
	}

	os.Setenv("NAME_CONFIGITEM", "config-value")
	defer os.Unsetenv("NAME_CONFIGITEM")
	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, "config-value", result.ConfigItem.Value)
}

func TestDecodeConfigWithDurationYaml(t *testing.T) {
	type configStruct struct {
		ConfigItem time.Duration `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "10m",
	}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, int64(600000), result.ConfigItem.Milliseconds())
}

func TestDecodeConfigWithDurationEnvironment(t *testing.T) {
	type configStruct struct {
		ConfigItem time.Duration `yaml:"config_item"`
	}

	os.Setenv("NAME_CONFIGITEM", "10m")
	defer os.Unsetenv("NAME_CONFIGITEM")
	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, int64(600000), result.ConfigItem.Milliseconds())
}

func TestDecodeConfigWithDurationUnset(t *testing.T) {
	type configStruct struct {
		ConfigItem time.Duration `yaml:"config_item"`
	}

	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.ConfigItem.Milliseconds())
}

func TestDecodeConfigWithRegexpYaml(t *testing.T) {
	type configStruct struct {
		ConfigItem util.Regexp `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "^foo$",
	}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	if assert.NotNil(t, result.ConfigItem.Value) {
		assert.True(t, result.ConfigItem.Value.Match([]byte("foo")))
		assert.False(t, result.ConfigItem.Value.Match([]byte("bar")))
	}
}

func TestDecodeConfigWithRegexpEnvironment(t *testing.T) {
	type configStruct struct {
		ConfigItem util.Regexp `yaml:"config_item"`
	}

	os.Setenv("NAME_CONFIGITEM", "^foo$")
	defer os.Unsetenv("NAME_CONFIGITEM")
	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	if assert.NotNil(t, result.ConfigItem.Value) {
		assert.True(t, result.ConfigItem.Value.Match([]byte("foo")))
		assert.False(t, result.ConfigItem.Value.Match([]byte("bar")))
	}
}

func TestDecodeConfigWithUrlYaml(t *testing.T) {
	type configStruct struct {
		ConfigItem util.Url `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "http://example.com/path",
	}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	if assert.NotNil(t, result.ConfigItem.Value) {
		assert.Equal(t, "http", result.ConfigItem.Value.Scheme)
		assert.Equal(t, "example.com", result.ConfigItem.Value.Host)
		assert.Equal(t, "/path", result.ConfigItem.Value.Path)
	}
}

func TestDecodeConfigWithUrlEnvironment(t *testing.T) {
	type configStruct struct {
		ConfigItem util.Url `yaml:"config_item"`
	}

	os.Setenv("NAME_CONFIGITEM", "http://example.com/path")
	defer os.Unsetenv("NAME_CONFIGITEM")
	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig("name", config, &result)

	require.NoError(t, err)
	if assert.NotNil(t, result.ConfigItem.Value) {
		assert.Equal(t, "http", result.ConfigItem.Value.Scheme)
		assert.Equal(t, "example.com", result.ConfigItem.Value.Host)
		assert.Equal(t, "/path", result.ConfigItem.Value.Path)
	}
}
