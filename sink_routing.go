package veneur

import (
	"fmt"
	"regexp"
	"strings"
)

type SinkRoutingConfig struct {
	Name         string           `yaml:"name"`
	MatchConfigs []MatcherConfig  `yaml:"match"`
	Sinks        SinkRoutingSinks `yaml:"sinks"`
}

type MatcherConfig struct {
	Name NameMatcher  `yaml:"name"`
	Tags []TagMatcher `yaml:"tags"`
}

type NameMatcher struct {
	match func(string) bool
	regex *regexp.Regexp
	Value string `yaml:"value"`
	Kind string `yaml:"kind"`
}

type TagMatcherConfig struct {
	Kind  string `yaml:"kind"`
	Unset bool   `yaml:"unset"`
	Value string `yaml:"value"`
}

type TagMatcher struct {
	match func(string) bool
	regex *regexp.Regexp
	Unset bool `yaml:"unset"`
	Value string `yaml:"value"`
	Kind string `yaml:"kind"`
}

type SinkRoutingSinks struct {
	Matched    []string `yaml:"matched"`
	NotMatched []string `yaml:"not_matched"`
}

// UnmarshalYAML unmarshals and validates the yaml config for matching the name
// of a metric.
func (matcher *NameMatcher) UnmarshalYAML(
	unmarshal func(interface{}) error,
) error {
	var config map[string]string
	err := unmarshal(&config)
	if err != nil {
		return err
	}

	matcher.Kind = config["kind"]
	switch config["kind"] {
	case "any":
		matcher.match = matcher.matchAny
	case "exact":
		matcher.match = matcher.matchExact
		matcher.Value = config["value"]
	case "prefix":
		matcher.match = matcher.matchPrefix
		matcher.Value = config["value"]
	case "regex":
		matcher.regex, err = regexp.Compile(config["value"])
		matcher.Value = config["value"]
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", config["kind"])
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
	unmarshal func(interface{}) error,
) error {
	var config map[string]interface{}
	err := unmarshal(&config)
	if err != nil {
		return err
	}


	var cfgVal string
	if config["value"] != nil {
		cfgVal = config["value"].(string)
	}

	if config["kind"] != nil {
		matcher.Kind = config["kind"].(string)
	}

	switch matcher.Kind {
	case "exact":
		matcher.match = matcher.matchExact
		matcher.Value = cfgVal
	case "prefix":
		matcher.match = matcher.matchPrefix
		matcher.Value = cfgVal
	case "regex":
		matcher.regex, err = regexp.Compile(cfgVal)
		if err != nil {
			return err
		}
		matcher.match = matcher.matchRegex
	default:
		return fmt.Errorf("unknown matcher kind \"%s\"", matcher.Kind)
	}

	if config["unset"] != nil {
		matcher.Unset = config["unset"].(bool)
	}
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

func (config *SinkRoutingConfig) Match(
	name string, tags []string,
) bool {
configLoop:
	for _, matchConfig := range config.MatchConfigs {
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
