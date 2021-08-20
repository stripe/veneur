package veneur

import (
	"fmt"
	"regexp"
	"strings"
)

type SinkRoutingConfig struct {
	Name         string          `yaml:"name"`
	MatchConfigs []MatcherConfig `yaml:"match"`
	Sinks        []string        `yaml:"sinks"`
}

type MatcherConfig struct {
	Name NameMatcher  `yaml:"name"`
	Tags []TagMatcher `yaml:"tags"`
}

type NameMatcher struct {
	Kind  string `yaml:"kind"`
	match func(string) bool
	regex *regexp.Regexp
	Value string `yaml:"value"`
}

type TagMatcher struct {
	Key   TagKeyMatcher   `yaml:"key"`
	Value TagValueMatcher `yaml:"value"`
	match func(map[string]string) bool
}

type TagKeyMatcher struct {
	Kind  string `yaml:"kind"`
	regex *regexp.Regexp
	Value string `yaml:"value"`
}

type TagValueMatcher struct {
	Kind  string `yaml:"kind"`
	match func(string) bool
	regex *regexp.Regexp
	Value string `yaml:"value"`
}

// UnmarshalYAML unmarshals and validates the yaml config for matching the name
// of a metric.
func (matcher *NameMatcher) UnmarshalYAML(
	unmarshal func(interface{}) error) error {
	err := unmarshal(matcher)
	if err != nil {
		return err
	}
	switch matcher.Kind {
	case "any":
		matcher.match = matcher.matchAny
	case "exact":
		matcher.match = matcher.matchExact
	case "prefix":
		matcher.match = matcher.matchPrefix
	case "regex":
		matcher.regex, err = regexp.Compile(matcher.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", matcher.Kind)
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
	return matcher.regex.MatchString(value)
}

// UnmarshalYAML unmarshals and validates the yaml config for matching tags
// within a metric.
func (matcher *TagMatcher) UnmarshalYAML(
	unmarshal func(interface{}) error) error {
	err := unmarshal(matcher)
	if err != nil {
		return err
	}
	switch matcher.Value.Kind {
	case "any":
		matcher.Value.match = matcher.matchValueAny
	case "exact":
		matcher.Value.match = matcher.matchValueExact
	case "prefix":
		matcher.Value.match = matcher.matchValuePrefix
	case "regex":
		matcher.Value.regex, err = regexp.Compile(matcher.Value.Value)
		if err != nil {
			return err
		}
		matcher.Value.match = matcher.matchValueRegex
	case "unset":
		switch matcher.Key.Kind {
		case "any":
			matcher.match = matcher.matchUnsetKeyAny
		case "exact":
			matcher.match = matcher.matchUnsetKeyExact
		case "prefix":
			matcher.match = matcher.matchUnsetKeyPrefix
		case "regex":
			matcher.Key.regex, err = regexp.Compile(matcher.Key.Value)
			if err != nil {
				return err
			}
			matcher.match = matcher.matchUnsetKeyRegex
		default:
			return fmt.Errorf("unknown key matcher kind \"%s\"", matcher.Key.Kind)
		}
		return nil
	default:
		return fmt.Errorf("unknown value matcher kind \"%s\"", matcher.Value.Kind)
	}
	switch matcher.Key.Kind {
	case "any":
		matcher.match = matcher.matchKeyAny
	case "exact":
		matcher.match = matcher.matchKeyExact
	case "prefix":
		matcher.match = matcher.matchKeyPrefix
	case "regex":
		matcher.Key.regex, err = regexp.Compile(matcher.Key.Value)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchKeyRegex
	default:
		return fmt.Errorf("unknown key matcher kind \"%s\"", matcher.Key.Kind)
	}
	return nil
}

func (matcher *TagMatcher) matchKeyAny(tags map[string]string) bool {
	for _, value := range tags {
		if matcher.Value.match(value) {
			return true
		}
	}
	return false
}

func (matcher *TagMatcher) matchKeyExact(tags map[string]string) bool {
	value, ok := tags[matcher.Key.Value]
	if !ok {
		return false
	}
	return matcher.Value.match(value)
}

func (matcher *TagMatcher) matchKeyPrefix(tags map[string]string) bool {
	for key, value := range tags {
		if !strings.HasPrefix(key, matcher.Key.Value) {
			continue
		}
		if matcher.Value.match(value) {
			return true
		}
	}
	return false
}

func (matcher *TagMatcher) matchKeyRegex(tags map[string]string) bool {
	for key, value := range tags {
		if !matcher.Key.regex.MatchString(key) {
			continue
		}
		if matcher.Value.match(value) {
			return true
		}
	}
	return false
}

func (matcher *TagMatcher) matchUnsetKeyAny(tags map[string]string) bool {
	return len(tags) == 0
}

func (matcher *TagMatcher) matchUnsetKeyExact(tags map[string]string) bool {
	_, ok := tags[matcher.Key.Value]
	return !ok
}

func (matcher *TagMatcher) matchUnsetKeyPrefix(tags map[string]string) bool {
	for key := range tags {
		if strings.HasPrefix(key, matcher.Key.Value) {
			return false
		}
	}
	return true
}

func (matcher *TagMatcher) matchUnsetKeyRegex(tags map[string]string) bool {
	for key := range tags {
		if matcher.Key.regex.MatchString(key) {
			return false
		}
	}
	return true
}

func (matcher *TagMatcher) matchValueAny(value string) bool {
	return true
}

func (matcher *TagMatcher) matchValueExact(value string) bool {
	return value == matcher.Value.Value
}

func (matcher *TagMatcher) matchValuePrefix(value string) bool {
	return strings.HasPrefix(value, matcher.Value.Value)
}

func (matcher *TagMatcher) matchValueRegex(value string) bool {
	return matcher.Value.regex.MatchString(value)
}

func (config *SinkRoutingConfig) Match(
	name string, tags map[string]string,
) bool {
outer:
	for _, matchConfig := range config.MatchConfigs {
		if !matchConfig.Name.match(name) {
			continue
		}
		for _, tagMatchConfig := range matchConfig.Tags {
			if !tagMatchConfig.match(tags) {
				continue outer
			}
		}
		return true
	}
	return false
}
