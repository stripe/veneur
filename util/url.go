package util

import (
	"net/url"
)

type Url struct {
	Value *url.URL
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
