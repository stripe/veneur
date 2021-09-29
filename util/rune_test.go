package util_test

import (
	"os"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v3"
)

type runeYamlStruct struct {
	Rune util.Rune `yaml:"rune"`
}

func TestRuneUnmarshalYAMLComma(t *testing.T) {
	yamlFile := []byte(`rune: ","`)
	data := runeYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, ',', rune(data.Rune))
}

func TestRuneUnmarshalYAMLTab(t *testing.T) {
	yamlFile := []byte(`rune: "\t"`)
	data := runeYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, '\t', rune(data.Rune))
}

func TestRuneUnmarshalTextComma(t *testing.T) {
	defer os.Unsetenv("ENVCONFIG_TEST_RUNE")
	os.Setenv("ENVCONFIG_TEST_RUNE", ",")

	data := runeYamlStruct{}
	err := envconfig.Process("ENVCONFIG_TEST", &data)
	assert.Nil(t, err)
	assert.Equal(t, ',', rune(data.Rune))
}

func TestRuneUnmarshalTextTab(t *testing.T) {
	defer os.Unsetenv("ENVCONFIG_TEST_RUNE")
	os.Setenv("ENVCONFIG_TEST_RUNE", "\t")

	data := runeYamlStruct{}
	err := envconfig.Process("ENVCONFIG_TEST", &data)
	assert.Nil(t, err)
	assert.Equal(t, '\t', rune(data.Rune))
}
