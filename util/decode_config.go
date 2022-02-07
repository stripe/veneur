package util

import (
	"fmt"
	"reflect"

	"github.com/kelseyhightower/envconfig"
	"github.com/mitchellh/mapstructure"
)

type stringUnmarshaler interface {
	Decode(value string) error
}

var stringUnmarshalerType = reflect.TypeOf((*stringUnmarshaler)(nil)).Elem()

// DecodeConfig wraps the mapstructure decoder to unpack a map into a struct
// and the envconfig decoder to read environment variables.
//
// This method provides logic to handle decoding into structs that implement the
// stringUnmarshaler interface and is intended to be used by sources and sinks
// while unpacking the configuration specific to that source or sink from within
// the entire config.
func DecodeConfig(name string, input interface{}, output interface{}) error {
	configDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
			textUnmarshalerDecode,
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
	err = envconfig.Process(name, output)
	if err != nil {
		return err
	}
	return nil
}

// A mapstructure decode hook to handle decoding custom fields.
func textUnmarshalerDecode(
	inputType reflect.Type, outputType reflect.Type, data interface{},
) (interface{}, error) {
	if !reflect.PtrTo(outputType).Implements(stringUnmarshalerType) {
		return data, nil
	}
	value, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("invalid type %v", inputType)
	}
	parsedValue, ok := reflect.New(outputType).Interface().(stringUnmarshaler)
	if !ok {
		return nil, fmt.Errorf("invalid output type %v", outputType)
	}
	err := parsedValue.Decode(value)
	if err != nil {
		return nil, err
	}
	return parsedValue, nil
}
