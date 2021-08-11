package util

import (
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"
)

func DecodeConfig(input interface{}, output interface{}) error {
	configDecoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			StringSecretDecode,
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

func StringSecretDecode(
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
