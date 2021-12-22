package util

import (
	"fmt"
	"regexp"
)

type Regexp struct {
	Value *regexp.Regexp
}

func (r *Regexp) Decode(value string) error {
	var err error
	r.Value, err = regexp.Compile(value)
	return err
}

func (r Regexp) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%s\"", r.Value.String())), nil
}

func (r Regexp) MarshalYAML() (interface{}, error) {
	return r.Value.String(), nil
}

func (r *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var value string
	err := unmarshal(&value)
	if err != nil {
		return err
	}
	r.Value, err = regexp.Compile(value)
	if err != nil {
		return err
	}
	return nil
}
