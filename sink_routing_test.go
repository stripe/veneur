package veneur_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/veneur/v14"
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
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	assert.True(t, config.Match("aaa", map[string]string{}))
	assert.True(t, config.Match("aab", map[string]string{}))
	assert.True(t, config.Match("aaba", map[string]string{}))
	assert.True(t, config.Match("abb", map[string]string{}))
}

func TestMatchNameExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "exact",
				Value: "aab",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	assert.False(t, config.Match("aaa", map[string]string{}))
	assert.True(t, config.Match("aab", map[string]string{}))
	assert.False(t, config.Match("aaba", map[string]string{}))
	assert.False(t, config.Match("abb", map[string]string{}))
}

func TestMatchNamePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "prefix",
				Value: "aa",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	assert.True(t, config.Match("aaa", map[string]string{}))
	assert.True(t, config.Match("aab", map[string]string{}))
	assert.True(t, config.Match("aaba", map[string]string{}))
	assert.False(t, config.Match("abb", map[string]string{}))
}

func TestMatchNameRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "regex",
				Value: "ab+$",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	assert.False(t, config.Match("aaa", map[string]string{}))
	assert.True(t, config.Match("aab", map[string]string{}))
	assert.False(t, config.Match("aaba", map[string]string{}))
	assert.True(t, config.Match("abb", map[string]string{}))
}

func TestMatchNameInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "regex",
				Value: "[",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	assert.Error(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
}

func TestMatchNameInvalidKind(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "invalid",
			},
			Tags: []veneur.TagMatcher{},
		}},
	}

	assert.Error(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
}

func TestMatchTagNameAny(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameAnyUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind: "unset",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "exact",
					Value: "aab",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameExactUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "exact",
					Value: "aab",
				},
				Value: veneur.TagValueMatcher{
					Kind: "unset",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNamePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNamePrefixUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
				Value: veneur.TagValueMatcher{
					Kind: "unset",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "regex",
					Value: "ab+$",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameRegexUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "regex",
					Value: "ab+$",
				},
				Value: veneur.TagValueMatcher{
					Kind: "unset",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", map[string]string{
		"aaa": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"aab": "value",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aaba": "value",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"abb": "value",
	}))
}

func TestMatchTagNameInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "regex",
					Value: "[",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Error(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
}

func TestMatchTagNameInvalidKind(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "invalid",
				},
				Value: veneur.TagValueMatcher{
					Kind: "any",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Error(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
}

func TestMatchTagValueExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "exact",
					Value: "aab",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"key": "aaa",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"key": "aab",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"key": "aaba",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"key": "abb",
	}))
}

func TestMatchTagValuePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", map[string]string{
		"key": "aaa",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"key": "aab",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"key": "aaba",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"key": "abb",
	}))
}

func TestMatchTagValueRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "regex",
					Value: "ab+$",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"key": "aaa",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"key": "aab",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"key": "aaba",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"key": "abb",
	}))
}

func TestMatchTagValueInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "regex",
					Value: "[",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Error(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
}

func TestMatchTagValueInvalidKind(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind: "any",
				},
				Value: veneur.TagValueMatcher{
					Kind: "invalid",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Error(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
}

func TestMatchMultipleTags(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "bb",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", map[string]string{
		"aaab": "abab",
		"baba": "bbab",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"aaba": "bbab",
		"baba": "bbab",
	}))
}

func TestMultipleTagMatchers(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "exact",
					Value: "ab",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "ab",
				},
			}, {
				Key: veneur.TagKeyMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "bb",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[1]))
	assert.False(t, config.Match("name", map[string]string{
		"aaab": "bbab",
		"baba": "abab",
	}))
	assert.False(t, config.Match("name", map[string]string{
		"ab":   "abab",
		"baab": "bbab",
	}))
	assert.True(t, config.Match("name", map[string]string{
		"ab":   "abab",
		"aaab": "bbab",
		"baba": "bbab",
	}))
}

func TestMultipleMatcherConfigs(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "exact",
				Value: "aa",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "exact",
					Value: "ab",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "ab",
				},
			}},
		}, {
			Name: veneur.NameMatcher{
				Kind:  "exact",
				Value: "bb",
			},
			Tags: []veneur.TagMatcher{{
				Key: veneur.TagKeyMatcher{
					Kind:  "prefix",
					Value: "aa",
				},
				Value: veneur.TagValueMatcher{
					Kind:  "prefix",
					Value: "bb",
				},
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[1].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[1].Tags[0]))
	assert.False(t, config.Match("aa", map[string]string{
		"aaab": "bbab",
		"baba": "abab",
	}))
	assert.True(t, config.Match("bb", map[string]string{
		"aaab": "bbab",
		"baba": "abab",
	}))
	assert.True(t, config.Match("aa", map[string]string{
		"ab":   "abab",
		"baab": "bbab",
	}))
	assert.False(t, config.Match("bb", map[string]string{
		"ab":   "abab",
		"baab": "bbab",
	}))
}
