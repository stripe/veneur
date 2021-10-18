package util_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14/util"
)

func TestDecodeConfig(t *testing.T) {
	type configStruct struct {
		ConfigItem string `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "config-value",
	}
	result := configStruct{}
	err := util.DecodeConfig(config, &result)

	require.Nil(t, err)
	assert.Equal(t, "config-value", result.ConfigItem)
}

func TestDecodeConfigWithStringSecret(t *testing.T) {
	type configStruct struct {
		ConfigItem util.StringSecret `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "config-value",
	}
	result := configStruct{}
	err := util.DecodeConfig(config, &result)

	require.Nil(t, err)
	assert.Equal(t, "config-value", result.ConfigItem.Value)
}

func TestDecodeConfigWithDuration(t *testing.T) {
	type configStruct struct {
		ConfigItem time.Duration `yaml:"config_item"`
	}

	config := map[string]interface{}{
		"config_item": "10m",
	}
	result := configStruct{}
	err := util.DecodeConfig(config, &result)

	require.Nil(t, err)
	assert.Equal(t, int64(600000), result.ConfigItem.Milliseconds())
}

func TestDecodeConfigWithDurationUnset(t *testing.T) {
	type configStruct struct {
		ConfigItem time.Duration `yaml:"config_item"`
	}

	config := map[string]interface{}{}
	result := configStruct{}
	err := util.DecodeConfig(config, &result)

	require.Nil(t, err)
	assert.Equal(t, int64(0), result.ConfigItem.Milliseconds())
}
