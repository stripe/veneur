package util

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

// DecodeConfig wraps the mapstructure decoder to unpack a map into a struct.
// This method provides logic to handle decoding into StringSecret and
// time.Duration fields, and is intended to be used by sinks while unpacking the
// configuration specific to that sink from within the entire config.
func DecodeConfig(input interface{}, output interface{}) error {
	configDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringSecretDecode,
			mapstructure.StringToTimeDurationHookFunc(),
		),
		Result:  &output,
		TagName: "yaml",
	})
	if err != nil {
		return err
	}
	err = configDecoder.Decode(input)
	if err != nil {
		return err
	}
	return nil
}

// A mapstructure decode hook to handle decoding StringSecret fields.
func stringSecretDecode(
	inputType reflect.Type, outputType reflect.Type, data interface{},
) (interface{}, error) {
	if outputType != reflect.TypeOf(StringSecret{}) {
		return data, nil
	}
	value, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type %v", inputType)
	}
	return StringSecret{
		Value: value,
	}, nil
}
