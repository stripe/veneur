package pointer

import (
	"fmt"
	"reflect"
	"time"
)

// Duration returns a pointer to a time.Duration
func Duration(d time.Duration) *time.Duration {
	return &d
}

// Int32 returns a pointer to an int32
func Int32(i int32) *int32 {
	return &i
}

// Uint returns a pointer to a uint
func Uint(i uint) *uint {
	return &i
}

// Uint16 returns a pointer to a uint16
func Uint16(i uint16) *uint16 {
	return &i
}

// Uint32 returns a pointer to a uint32
func Uint32(i uint32) *uint32 {
	return &i
}

// String returns a pointer to a string
func String(i string) *string {
	return &i
}

// Int returns a pointer to an int
func Int(i int) *int {
	return &i
}

// Int64 returns a pointer to an int64
func Int64(i int64) *int64 {
	return &i
}

// Bool returns a pointer to a bool
func Bool(b bool) *bool {
	return &b
}

// Float64 returns a pointer to a float64
func Float64(b float64) *float64 {
	return &b
}

func canNil(k reflect.Kind) bool {
	return k == reflect.Chan || k == reflect.Func || k == reflect.Map || k == reflect.Ptr || k == reflect.Interface || k == reflect.Slice
}

// FillDefaultFrom fills default values replacing nil values with the first non nil.  The replacement goes into existing
func FillDefaultFrom(defaultsList ...interface{}) interface{} {
	if len(defaultsList) == 0 {
		return nil
	}

	rootType := reflect.TypeOf(defaultsList[0])
	if rootType.Kind() != reflect.Ptr {
		panic(fmt.Sprintf("FillDefaultsFrom only takes pointer types, not %s", rootType))
	}
	typeToMake := rootType.Elem()

	existing := reflect.New(typeToMake).Interface()
	existingVal := reflect.ValueOf(existing).Elem()
	existingType := reflect.TypeOf(existing)
	for _, defaults := range defaultsList {
		if defaults == nil {
			continue
		}
		defaultType := reflect.TypeOf(defaults)
		if defaultType != existingType {
			panic(fmt.Sprintf("Uncompatible types %s vs %s", existingType, defaultType))
		}
		defaultsVal := reflect.ValueOf(defaults)
		if defaultsVal.IsNil() {
			continue
		}
		defaultsVal = defaultsVal.Elem()
		singleItemCopy(existingVal, defaultsVal)
	}
	return existing
}

func singleItemCopy(existingVal reflect.Value, defaultsVal reflect.Value) {
	for i := 0; i < existingVal.NumField(); i++ {
		if canNil(existingVal.Field(i).Kind()) && existingVal.Field(i).IsNil() {
			defaultValue := defaultsVal.Field(i).Interface()
			if defaultValue != reflect.ValueOf(nil) {
				existingVal.Field(i).Set(defaultsVal.Field(i))
			}
		}
	}
}
