package veneur_test

import (
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
	"gopkg.in/yaml.v3"
)

func CreateNameMatcher(
	t *testing.T, matcher *veneur.NameMatcher,
) error {
	return matcher.UnmarshalYAML(func(matcher interface{}) error {
		assert.IsType(t, &veneur.NameMatcher{}, matcher)
		return nil
	})
}

func CreateTagMatcher(
	t *testing.T, matcher *veneur.TagMatcher,
) error {
	return matcher.UnmarshalYAML(func(matcher interface{}) error {
		assert.IsType(t, &veneur.TagMatcher{}, matcher)
		return nil
	})
}

func TestMatchNameAny(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.True(t, config.Match("aaba", []string{}))
	assert.True(t, config.Match("abb", []string{}))
}

func TestMatchNameExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: exact
      value: aab
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.False(t, config.Match("aaba", []string{}))
	assert.False(t, config.Match("abb", []string{}))
}

func TestMatchNamePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: prefix
      value: aa
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.True(t, config.Match("aaba", []string{}))
	assert.False(t, config.Match("abb", []string{}))
}

func TestMatchNameRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: regex
      value: ab+$
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.False(t, config.Match("aaba", []string{}))
	assert.True(t, config.Match("abb", []string{}))
}

func TestMatchNameInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: regex
      value: "["
`), &config)

	require.Error(t, err)
	syntaxError, ok := err.(*syntax.Error)
	require.True(t, ok)
	assert.Equal(t, syntaxError.Code, syntax.ErrMissingBracket)
}

func TestMatchNameInvalidKind(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: invalid
`), &config)

	require.Error(t, err)
	assert.Equal(t, err.Error(), "unknown matcher kind \"invalid\"")
}

func TestMatchTagExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: exact
        value: aab
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameExactUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: exact
        unset: true
        value: aab
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNamePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: prefix
        value: aa
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNamePrefixUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: prefix
        unset: true
        value: aa
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: regex
        value: ab+$
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameRegexUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: regex
        unset: true
        value: ab+$
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: regex
        value: "["
`), &config)

	require.Error(t, err)
	syntaxError, ok := err.(*syntax.Error)
	require.True(t, ok)
	assert.Equal(t, syntaxError.Code, syntax.ErrMissingBracket)
}

func TestMatchTagNameInvalidKind(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: invalid
`), &config)

	require.Error(t, err)
	assert.Equal(t, err.Error(), "unknown matcher kind \"invalid\"")
}

func TestMatchTagMultiple(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: prefix
        value: aa
`), &config)

	require.Nil(t, err)
	assert.True(t, config.Match("name", []string{"aaab", "baba"}))
	assert.True(t, config.Match("name", []string{"baba", "aaab"}))
	assert.False(t, config.Match("name", []string{"abba", "baba"}))
}

func TestMatchTagUnsetMultiple(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: prefix
        unset: true
        value: aa
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("name", []string{"aaab", "baba"}))
	assert.False(t, config.Match("name", []string{"baba", "aaab"}))
	assert.True(t, config.Match("name", []string{"abba", "baba"}))
}

func TestMultipleTagMatchers(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: any
    tags:
      - kind: exact
        value: ab
      - kind: prefix
        value: aa
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("name", []string{"ab", "baab"}))
	assert.False(t, config.Match("name", []string{"aaab", "baba"}))
	assert.True(t, config.Match("name", []string{"ab", "aaab", "baba"}))
}

func TestMultipleMatcherConfigs(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	err := yaml.Unmarshal([]byte(`---
match:
  - name:
      kind: exact
      value: aa
    tags:
      - kind: exact
        value: ab
  - name:
      kind: exact
      value: bb
    tags:
      - kind: prefix
        value: aa
`), &config)

	require.Nil(t, err)
	assert.False(t, config.Match("aa", []string{"aaab", "baba"}))
	assert.True(t, config.Match("bb", []string{"aaab", "baba"}))
	assert.True(t, config.Match("aa", []string{"ab", "baab"}))
	assert.False(t, config.Match("bb", []string{"ab", "baab"}))
}

func TestUnmarshallingAndMarshalling(t *testing.T) {
	config := veneur.SinkRoutingConfig{}
	yamlString := []byte(`name: test
match:
  - name:
      kind: exact
      value: aa
    tags:
      - kind: exact
        value: ab
  - name:
      kind: exact
      value: bb
    tags:
      - kind: prefix
        value: aa
        unset: true 
`)

	require.Nil(t, yaml.Unmarshal(yamlString, &config))

	backToYaml, err := yaml.Marshal(config)
	require.Nil(t, err)

	nConfig := veneur.SinkRoutingConfig{}
	require.Nil(t, yaml.Unmarshal(backToYaml, &nConfig))

	assert.Equal(t, config.MatchConfigs[0].Name.Kind, nConfig.MatchConfigs[0].Name.Kind)
	assert.Equal(t, config.MatchConfigs[0].Name.Value, nConfig.MatchConfigs[0].Name.Value)
	assert.Equal(t, config.MatchConfigs[0].Tags[0].Kind, nConfig.MatchConfigs[0].Tags[0].Kind)
	assert.Equal(t, config.MatchConfigs[0].Tags[0].Value, nConfig.MatchConfigs[0].Tags[0].Value)

	assert.Equal(t, config.MatchConfigs[1].Name.Kind, nConfig.MatchConfigs[1].Name.Kind)
	assert.Equal(t, config.MatchConfigs[1].Name.Value, nConfig.MatchConfigs[1].Name.Value)
	assert.Equal(t, config.MatchConfigs[1].Tags[0].Kind, nConfig.MatchConfigs[1].Tags[0].Kind)
	assert.Equal(t, config.MatchConfigs[1].Tags[0].Value, nConfig.MatchConfigs[1].Tags[0].Value)
	assert.Equal(t, config.MatchConfigs[1].Tags[0].Unset, nConfig.MatchConfigs[1].Tags[0].Unset)
}
