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
	Match func(string) bool
	Regex *regexp.Regexp
	Value string
}

type TagMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Unset bool   `yaml:"unset"`
	Value string `yaml:"value"`
}

type TagMatcher struct {
	Match func(string) bool
	Regex *regexp.Regexp
	Unset bool
	Value string
}

func (matcherConfigs MatcherConfigs) Match(
	name string, tags []string,
) bool {
configLoop:
	for _, matcherConfig := range matcherConfigs {
		if !matcherConfig.Name.Match(name) {
			continue
		}
	tagLoop:
		for _, tagMatchConfig := range matcherConfig.Tags {
			for _, tag := range tags {
				if tagMatchConfig.Match(tag) {
					if tagMatchConfig.Unset {
						continue configLoop
					} else {
						continue tagLoop
					}
				}
			}
			if !tagMatchConfig.Unset {
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
		matcher.Match = matcher.matchAny
	case "exact":
		matcher.Match = matcher.matchExact
		matcher.Value = config.Value
	case "prefix":
		matcher.Match = matcher.matchPrefix
		matcher.Value = config.Value
	case "regex":
		matcher.Regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.Match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	return nil
}

func (matcher *NameMatcher) matchAny(value string) bool {
	return true
}

func (matcher *NameMatcher) matchExact(value string) bool {
	return value == matcher.Value
}

func (matcher *NameMatcher) matchPrefix(value string) bool {
	return strings.HasPrefix(value, matcher.Value)
}

func (matcher *NameMatcher) matchRegex(value string) bool {
	return matcher.Regex.MatchString(value)
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
		matcher.Match = matcher.matchExact
		matcher.Value = config.Value
	case "prefix":
		matcher.Match = matcher.matchPrefix
		matcher.Value = config.Value
	case "regex":
		matcher.Regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.Match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	matcher.Unset = config.Unset
	return nil
}

func (matcher *TagMatcher) matchExact(value string) bool {
	return value == matcher.Value
}

func (matcher *TagMatcher) matchPrefix(value string) bool {
	return strings.HasPrefix(value, matcher.Value)
}

func (matcher *TagMatcher) matchRegex(value string) bool {
	return matcher.Regex.MatchString(value)
}
