package matcher

import (
	"fmt"
	"regexp"
	"strings"
)

type Matcher struct {
	Name NameMatcher  `yaml:"name"`
	Tags []TagMatcher `yaml:"tags"`
}

type NameMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Value string `yaml:"value"`
}

type NameMatcher struct {
	Kind  string `yaml:"kind"`
	match func(string) bool
	regex *regexp.Regexp
	Value string `yaml:"value"`
}

type TagMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Unset bool   `yaml:"unset"`
	Value string `yaml:"value"`
}

type TagMatcher struct {
	Kind  string `yaml:"kind"`
	match func(string) bool
	regex *regexp.Regexp
	Unset bool   `yaml:"unset"`
	Value string `yaml:"value"`
}

func CreateNameMatcher(config *NameMatcherConfig) NameMatcher {
	nameMatcher := NameMatcher{}
	nameMatcher.UnmarshalYAML(func(c interface{}) error {
		nameMatcherConfig := c.(*NameMatcherConfig)
		*nameMatcherConfig = *config
		return nil
	})
	return nameMatcher
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

	matcher.Kind = config.Kind
	switch config.Kind {
	case "any":
		matcher.match = matcher.matchAny
	case "exact":
		matcher.match = matcher.matchExact
	case "prefix":
		matcher.match = matcher.matchPrefix
	case "regex":
		matcher.regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	matcher.Value = config.Value

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
	return matcher.regex.MatchString(value)
}

func CreateTagMatcher(config *TagMatcherConfig) TagMatcher {
	tagMatcher := TagMatcher{}
	tagMatcher.UnmarshalYAML(func(c interface{}) error {
		tagMatcherConfig := c.(*TagMatcherConfig)
		*tagMatcherConfig = *config
		return nil
	})
	return tagMatcher
}

func (matcher *TagMatcher) Match(tag string) bool {
	return matcher.match(tag)
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

	matcher.Kind = config.Kind
	switch config.Kind {
	case "exact":
		matcher.match = matcher.matchExact
	case "prefix":
		matcher.match = matcher.matchPrefix
	case "regex":
		matcher.regex, err = regexp.Compile(config.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config.Kind)
	}
	matcher.Unset = config.Unset
	matcher.Value = config.Value

	return nil
}

func (matcher *TagMatcher) matchExact(value string) bool {
	return value == matcher.Value
}

func (matcher *TagMatcher) matchPrefix(value string) bool {
	return strings.HasPrefix(value, matcher.Value)
}

func (matcher *TagMatcher) matchRegex(value string) bool {
	return matcher.regex.MatchString(value)
}

func Match(
	matchConfigs []Matcher, name string, tags []string,
) bool {
configLoop:
	for _, matchConfig := range matchConfigs {
		if !matchConfig.Name.match(name) {
			continue
		}
	tagLoop:
		for _, tagMatchConfig := range matchConfig.Tags {
			for _, tag := range tags {
				if tagMatchConfig.match(tag) {
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
