package tls_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util/tls"
	"gopkg.in/yaml.v2"
)

type yamlStruct struct {
	Tls tls.Tls `yaml:"tls"`
}

const yamlFile = `---
tls: 
  ca_file: "test/cert.crt"
  cert_file: "test/cert.crt"
  key_file:  "test/key.key"
`

func TestGetTlsConfig(t *testing.T) {
	data := yamlStruct{}
	err := yaml.Unmarshal([]byte(yamlFile), &data)
	assert.NoError(t, err)
	tlsConfig := data.Tls.GetTlsConfig()
	assert.NotNil(t, tlsConfig)
	assert.Len(t, tlsConfig.Certificates, 1)
}

func TestGetTlsConfigUnset(t *testing.T) {
	data := yamlStruct{}
	err := yaml.Unmarshal([]byte(""), &data)
	assert.NoError(t, err)
	tlsConfig := data.Tls.GetTlsConfig()
	assert.Nil(t, tlsConfig)
}
