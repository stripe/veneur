package routing

import (
	"fmt"
	"regexp"
	"strings"
)

type MatcherConfig struct {
	Name NameMatcher  `yaml:"name"`
	Tags []TagMatcher `yaml:"tags"`
}

type MatcherConfigs []MatcherConfig

type NameMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Value string `yaml:"value"`
}

type NameMatcher struct {
	match func(string) bool
	regex *regexp.Regexp
	value string
}

type TagMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Unset bool   `yaml:"unset"`
	Value string `yaml:"value"`
}

type TagMatcher struct {
	match func(string) bool
	regex *regexp.Regexp
	unset bool
	value string
}

func (matcherConfigs MatcherConfigs) Match(
	name string, tags []string,
) bool {
configLoop:
	for _, matcherConfig := range matcherConfigs {
		if !matcherConfig.Name.match(name) {
			continue
		}
	tagLoop:
		for _, tagMatchConfig := range matcherConfig.Tags {
			for _, tag := range tags {
				if tagMatchConfig.match(tag) {
					if tagMatchConfig.unset {
						continue configLoop
					} else {
						continue tagLoop
					}
				}
			}
			if !tagMatchConfig.unset {
				continue configLoop
			}
		}
		return true
	}
	return false
}

// UnmarshalYAML unmarshals and validates the yaml config for matching the name
// of a metric.
func (matcher *NameMatcher) UnmarshalYAML(
	unmarshal func(interface{}) error,
) error {
	config := NameMatcherConfig{}
	err := unmarshal(&config)
	if err != nil {
		return err
	}
	switch config.Kind {
	case "any":
		matcher.match = matcher.matchAny
	case "exact":
		matcher.match = matcher.matchExact
		matcher.value = config.Value
	case "prefix":
		matcher.match = matcher.matchPrefix
		matcher.value = config.Value
	case "regex":
		matcher.regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	return nil
}

func (matcher *NameMatcher) matchAny(value string) bool {
	return true
}

func (matcher *NameMatcher) matchExact(value string) bool {
	return value == matcher.value
}

func (matcher *NameMatcher) matchPrefix(value string) bool {
	return strings.HasPrefix(value, matcher.value)
}

func (matcher *NameMatcher) matchRegex(value string) bool {
	return matcher.regex.MatchString(value)
}

// UnmarshalYAML unmarshals and validates the yaml config for matching tags
// within a metric.
func (matcher *TagMatcher) UnmarshalYAML(
	unmarshal func(interface{}) error,
) error {
	config := TagMatcherConfig{}
	err := unmarshal(&config)
	if err != nil {
		return err
	}
	switch config.Kind {
	case "exact":
		matcher.match = matcher.matchExact
		matcher.value = config.Value
	case "prefix":
		matcher.match = matcher.matchPrefix
		matcher.value = config.Value
	case "regex":
		matcher.regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	matcher.unset = config.Unset
	return nil
}

func (matcher *TagMatcher) matchExact(value string) bool {
	return value == matcher.value
}

func (matcher *TagMatcher) matchPrefix(value string) bool {
	return strings.HasPrefix(value, matcher.value)
}

func (matcher *TagMatcher) matchRegex(value string) bool {
	return matcher.regex.MatchString(value)
}
