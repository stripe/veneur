package util

import "unicode/utf8"

type Rune rune

func (r *Rune) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var runeString string
	err := unmarshal(&runeString)
	if err != nil {
		return err
	}

	return r.UnmarshalText([]byte(runeString))
}

func (r *Rune) UnmarshalText(text []byte) error {
	nativeRune, _ := utf8.DecodeRune(text)
	*r = Rune(nativeRune)
	return nil
}
