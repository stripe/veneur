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
	assert.True(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.True(t, config.Match("aaba", []string{}))
	assert.True(t, config.Match("abb", []string{}))
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
	assert.False(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.False(t, config.Match("aaba", []string{}))
	assert.False(t, config.Match("abb", []string{}))
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
	assert.True(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.True(t, config.Match("aaba", []string{}))
	assert.False(t, config.Match("abb", []string{}))
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
	assert.False(t, config.Match("aaa", []string{}))
	assert.True(t, config.Match("aab", []string{}))
	assert.False(t, config.Match("aaba", []string{}))
	assert.True(t, config.Match("abb", []string{}))
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

func TestMatchTagExact(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "exact",
				Value: "aab",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameExactUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "exact",
				Unset: true,
				Value: "aab",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNamePrefix(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "prefix",
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNamePrefixUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "prefix",
				Unset: true,
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "regex",
				Value: "ab+$",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", []string{"aaa"}))
	assert.True(t, config.Match("name", []string{"aab"}))
	assert.False(t, config.Match("name", []string{"aaba"}))
	assert.True(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameRegexUnset(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "regex",
				Unset: true,
				Value: "ab+$",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", []string{"aaa"}))
	assert.False(t, config.Match("name", []string{"aab"}))
	assert.True(t, config.Match("name", []string{"aaba"}))
	assert.False(t, config.Match("name", []string{"abb"}))
}

func TestMatchTagNameInvalidRegex(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "regex",
				Value: "[",
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
				Kind: "invalid",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Error(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
}

func TestMatchTagMultiple(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "prefix",
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.True(t, config.Match("name", []string{"aaab", "baba"}))
	assert.False(t, config.Match("name", []string{"abba", "baba"}))
}

func TestMatchTagUnsetMultiple(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "prefix",
				Unset: true,
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	assert.False(t, config.Match("name", []string{"aaab", "baba"}))
	assert.True(t, config.Match("name", []string{"abba", "baba"}))
}

func TestMultipleTagMatchers(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind: "any",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "exact",
				Value: "ab",
			}, {
				Kind:  "prefix",
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[1]))
	assert.False(t, config.Match("name", []string{"ab", "baab"}))
	assert.False(t, config.Match("name", []string{"aaab", "baba"}))
	assert.True(t, config.Match("name", []string{"ab", "aaab", "baba"}))
}

func TestMultipleMatcherConfigs(t *testing.T) {
	config := veneur.SinkRoutingConfig{
		MatchConfigs: []veneur.MatcherConfig{{
			Name: veneur.NameMatcher{
				Kind:  "exact",
				Value: "aa",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "exact",
				Value: "ab",
			}},
		}, {
			Name: veneur.NameMatcher{
				Kind:  "exact",
				Value: "bb",
			},
			Tags: []veneur.TagMatcher{{
				Kind:  "prefix",
				Value: "aa",
			}},
		}},
	}

	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[0].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[0].Tags[0]))
	require.Nil(t, CreateNameMatcher(t, &config.MatchConfigs[1].Name))
	require.Nil(t, CreateTagMatcher(t, &config.MatchConfigs[1].Tags[0]))
	assert.False(t, config.Match("aa", []string{"aaab", "baba"}))
	assert.True(t, config.Match("bb", []string{"aaab", "baba"}))
	assert.True(t, config.Match("aa", []string{"ab", "baab"}))
	assert.False(t, config.Match("bb", []string{"ab", "baab"}))
}
