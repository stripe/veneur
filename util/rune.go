package util

import "unicode/utf8"

type Rune rune

func (r *Rune) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var runeString string
	err := unmarshal(&runeString)
	if err != nil {
		return err
	}

	nativeRune, _ := utf8.DecodeRune([]byte(runeString))
	*r = Rune(nativeRune)
	return nil
}
