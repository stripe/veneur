package util_test

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"github.com/kelseyhightower/envconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stripe/veneur/v14/util"
	"gopkg.in/yaml.v3"
)

type regexpYamlStruct struct {
	Regexp util.Regexp `yaml:"regexp"`
}

func TestRegexpMarshalJSON(t *testing.T) {
	marshaledRegexp, err := json.Marshal(util.Regexp{
		Value: regexp.MustCompile("^foo$"),
	})
	assert.Nil(t, err)
	assert.Equal(t, "\"^foo$\"", string(marshaledRegexp))
}

func TestRegexpMarshalJSONWithSpecialCharacters(t *testing.T) {
	marshaledRegexp, err := json.Marshal(util.Regexp{
		Value: regexp.MustCompile("^foo\"bar$"),
	})
	assert.Nil(t, err)
	assert.Equal(t, "\"^foo\\\"bar$\"", string(marshaledRegexp))
}

func TestRegexpMarshalYAML(t *testing.T) {
	marshaledRegexp, err := yaml.Marshal(util.Regexp{
		Value: regexp.MustCompile("^foo$"),
	})
	assert.Nil(t, err)
	assert.Equal(t, "^foo$\n", string(marshaledRegexp))
}

func TestRegexpUnmarshalYAML(t *testing.T) {
	yamlFile := []byte(`regexp: ^foo$`)
	data := regexpYamlStruct{}
	err := yaml.Unmarshal(yamlFile, &data)
	assert.Nil(t, err)
	assert.True(t, data.Regexp.Value.MatchString("foo"))
	assert.False(t, data.Regexp.Value.MatchString("bar"))
}

func TestRegexpDecode(t *testing.T) {
	defer os.Unsetenv("ENVCONFIG_TEST_REGEXP")
	os.Setenv("ENVCONFIG_TEST_REGEXP", "^foo$")

	data := regexpYamlStruct{}
	err := envconfig.Process("ENVCONFIG_TEST", &data)
	assert.Nil(t, err)
	assert.True(t, data.Regexp.Value.MatchString("foo"))
	assert.False(t, data.Regexp.Value.MatchString("bar"))
}
