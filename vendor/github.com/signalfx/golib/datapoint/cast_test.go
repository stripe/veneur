package datapoint

import (
	"reflect"
	"testing"
)

func TestCastMetricValue(t *testing.T) {
	type args struct {
		value interface{}
	}
	tests := []struct {
		name            string
		args            args
		wantMetricValue Value
		wantErr         bool
	}{
		{
			name: "cast float32 to int value",
			args: args{
				value: float32(1),
			},
			wantMetricValue: NewFloatValue(float64(1)),
		},
		{
			name: "cast float64 to int value",
			args: args{
				value: float64(2),
			},
			wantMetricValue: NewFloatValue(float64(2)),
		},
		{
			name: "cast uint to int value",
			args: args{
				value: uint(4),
			},
			wantMetricValue: NewIntValue(int64(4)),
		},
		{
			name: "cast uint8 to int value",
			args: args{
				value: uint8(5),
			},
			wantMetricValue: NewIntValue(int64(5)),
		},
		{
			name: "cast uint16 to int value",
			args: args{
				value: uint16(6),
			},
			wantMetricValue: NewIntValue(int64(6)),
		},
		{
			name: "cast uint32 to int value",
			args: args{
				value: uint32(7),
			},
			wantMetricValue: NewIntValue(int64(7)),
		},
		{
			name: "cast uint64 to int value",
			args: args{
				value: uint64(8),
			},
			wantMetricValue: NewIntValue(int64(8)),
		},
		{
			name: "cast int to int value",
			args: args{
				value: int(10),
			},
			wantMetricValue: NewIntValue(int64(10)),
		},
		{
			name: "cast int8 to int value",
			args: args{
				value: int8(11),
			},
			wantMetricValue: NewIntValue(int64(11)),
		},
		{
			name: "cast int16 to int value",
			args: args{
				value: int16(12),
			},
			wantMetricValue: NewIntValue(int64(12)),
		},
		{
			name: "cast int32 to int value",
			args: args{
				value: int32(13),
			},
			wantMetricValue: NewIntValue(int64(13)),
		},
		{
			name: "cast int64 to int value",
			args: args{
				value: int64(14),
			},
			wantMetricValue: NewIntValue(int64(14)),
		},
		{
			name: "cast metric value error",
			args: args{
				value: "hello world",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMetricValue, err := CastMetricValue(tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CastMetricValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMetricValue, tt.wantMetricValue) {
				t.Errorf("CastMetricValue() = %v, want %v", gotMetricValue, tt.wantMetricValue)
			}
		})
	}
}

func TestCastFloatValue(t *testing.T) {
	type args struct {
		value interface{}
	}
	tests := []struct {
		name            string
		args            args
		wantMetricValue Value
		wantErr         bool
	}{
		{
			name: "cast float err",
			args: args{
				value: int64(3),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMetricValue, err := CastFloatValue(tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CastFloatValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMetricValue, tt.wantMetricValue) {
				t.Errorf("CastFloatValue() = %v, want %v", gotMetricValue, tt.wantMetricValue)
			}
		})
	}
}

func TestCastUIntegerValue(t *testing.T) {
	type args struct {
		value interface{}
	}
	tests := []struct {
		name            string
		args            args
		wantMetricValue Value
		wantErr         bool
	}{
		{
			name: "cast uint error",
			args: args{
				value: int64(9),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMetricValue, err := CastUIntegerValue(tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CastUnsignedIntegerValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMetricValue, tt.wantMetricValue) {
				t.Errorf("CastUnsignedIntegerValue() = %v, want %v", gotMetricValue, tt.wantMetricValue)
			}
		})
	}
}

func TestCastIntegerValue(t *testing.T) {
	type args struct {
		value interface{}
	}
	tests := []struct {
		name            string
		args            args
		wantMetricValue Value
		wantErr         bool
	}{
		{
			name: "cast int error",
			args: args{
				value: uint64(15),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMetricValue, err := CastIntegerValue(tt.args.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CastIntegerValue() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotMetricValue, tt.wantMetricValue) {
				t.Errorf("CastIntegerValue() = %v, want %v", gotMetricValue, tt.wantMetricValue)
			}
		})
	}
}
