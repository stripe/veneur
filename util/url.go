package util

import (
	"encoding/json"
	"net/url"
)

type Url struct {
	Value *url.URL
}

func (u Url) MarshalJSON() ([]byte, error) {
	if u.Value == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(u.Value.String())
}

func (u Url) MarshalYAML() (interface{}, error) {
	if u.Value == nil {
		return "", nil
	}
	return u.Value.String(), nil
}

func (u *Url) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	err := unmarshal(&s)
	if err != nil {
		return err
	}
	u.Value, err = url.Parse(s)
	return err
}

func (u *Url) Decode(s string) error {
	var err error
	u.Value, err = url.Parse(s)
	return err
}
