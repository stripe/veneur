package util

import "flag"

var PrintSecrets = flag.Bool(
	"print-secrets", false, "Disables redacting config secrets")

const Redacted = "REDACTED"

type StringSecret struct {
	Value string
}

func (s StringSecret) String() string {
	if *PrintSecrets {
		return s.Value
	}
	if s.Value == "" {
		return ""
	}
	return Redacted
}

func (s *StringSecret) UnmarshalYAML(unmarshal func(interface{}) error) error {
	return unmarshal(&s.Value)
}
