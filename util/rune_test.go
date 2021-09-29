package util_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v3"
)

type runeYamlStruct struct {
	Rune util.Rune `yaml:"rune"`
}

func TestRuneUnmarshalComma(t *testing.T) {
	yamlFile := []byte(`rune: ","`)
	data := runeYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, ',', rune(data.Rune))
}

func TestRuneUnmarshalTab(t *testing.T) {
	yamlFile := []byte(`rune: "\t"`)
	data := runeYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, '\t', rune(data.Rune))
}
