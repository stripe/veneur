package tls_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util/tls"
	"gopkg.in/yaml.v3"
)

type yamlStruct struct {
	Tls *tls.Tls `yaml:"tls"`
}

func TestGetTlsConfig(t *testing.T) {
	yamlFile := []byte(`---
tls:
  ca_file: "../../testdata/cacert.pem"
  cert_file: "../../testdata/servercert.pem"
  key_file:  "../../testdata/serverkey.pem"
`)
	data := yamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.NoError(t, err)
	tlsConfig, err := data.Tls.GetTlsConfig()
	assert.NoError(t, err)
	assert.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
}

func TestGetTlsConfigMissingField(t *testing.T) {
	yamlFile := []byte(`---
tls:
  cert_file: "../../testdata/servercert.pem"
  key_file:  "../../testdata/serverkey.pem"
`)
	data := yamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Error(t, err)
}

func TestGetTlsConfigEmpty(t *testing.T) {
	yamlFile := []byte(`---
tls:
`)
	data := yamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.NoError(t, err)
	assert.Nil(t, data.Tls)
}

func TestGetTlsConfigUnset(t *testing.T) {
	data := yamlStruct{}
	err := yaml.Unmarshal([]byte{}, &data)
	assert.NoError(t, err)
	assert.Nil(t, data.Tls)
}
