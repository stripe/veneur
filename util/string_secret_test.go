package util_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v3"
)

type yamlStruct struct {
	StringSecret util.StringSecret `yaml:"stringSecret"`
}

func TestUnmarshal(t *testing.T) {
	yamlFile := []byte("stringSecret: secret-value")
	data := yamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.Equal(t, "secret-value", data.StringSecret.Value)
}

func TestPrint(t *testing.T) {
	stringSecret := util.StringSecret{Value: "secret-value"}
	printedStringSecret := fmt.Sprintf("%v", stringSecret)
	assert.Equal(t, util.Redacted, printedStringSecret)
}

func TestPrintEmptyValue(t *testing.T) {
	stringSecret := util.StringSecret{Value: ""}
	printedStringSecret := fmt.Sprintf("%v", stringSecret)
	assert.Equal(t, "", printedStringSecret)
}

func TestPrintRedactingDisabled(t *testing.T) {
	*util.PrintSecrets = true
	stringSecret := util.StringSecret{Value: "secret-value"}
	printedStringSecret := fmt.Sprintf("%v", stringSecret)
	assert.Equal(t, "secret-value", printedStringSecret)
}

func TestGetValue(t *testing.T) {
	stringSecret := util.StringSecret{Value: "secret-value"}
	assert.Equal(t, "secret-value", stringSecret.Value)
}
