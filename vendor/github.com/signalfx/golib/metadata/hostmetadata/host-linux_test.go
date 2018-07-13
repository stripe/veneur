// +build linux

package hostmetadata

import (
	"errors"
	"os"
	"reflect"
	"syscall"
	"testing"
)

func TestFillOSSpecificData(t *testing.T) {
	type args struct {
		syscallUname func(*syscall.Utsname) error
		etc          string
	}
	tests := []struct {
		name    string
		args    args
		want    *OS
		wantErr bool
	}{
		{
			name: "get uname os information",
			args: args{
				etc: "./testdata/lsb-release",
				syscallUname: func(in *syscall.Utsname) error {
					in.Version = [65]int8{35, 57, 45, 85, 98, 117, 110, 116,
						117, 32, 83, 77, 80, 32, 87, 101, 100,
						32, 77, 97, 121, 32, 49, 54, 32, 49,
						53, 58, 50, 50, 58, 53, 52, 32, 85,
						84, 67, 32, 50, 48, 49, 56}
					return nil
				},
			},
			want: &OS{
				HostKernelVersion: "#9-Ubuntu SMP Wed May 16 15:22:54 UTC 2018",
				HostLinuxVersion:  "Ubuntu 18.04 LTS",
			},
		},
		{
			name: "get uname os information uname call fails",
			args: args{
				etc: "./testdata/lsb-release",
				syscallUname: func(in *syscall.Utsname) error {
					in.Version = [65]int8{}
					return errors.New("shouldn't work")
				},
			},
			want:    &OS{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syscallUname = tt.args.syscallUname
			if err := os.Setenv("HOST_ETC", tt.args.etc); err != nil {
				t.Errorf("GetOS() error = %v failed to set HOST_ETC env var", err)
				return
			}
			in := &OS{}
			if err := fillPlatformSpecificOSData(in); err != nil {
				if !tt.wantErr {
					t.Errorf("fillPlatformSpecificOSData returned an error %v", err)
				}
				return
			}
			if !reflect.DeepEqual(in, tt.want) {
				t.Errorf("fillPlatformSpecificOSData() = %v, want %v", in, tt.want)
			}
		})
		os.Unsetenv("HOST_ETC")
		syscallUname = syscall.Uname
	}
}

func TestFillPlatformSpecificCPUData(t *testing.T) {
	type args struct {
		syscallUname func(*syscall.Utsname) error
	}
	tests := []struct {
		name    string
		args    args
		want    *CPU
		wantErr bool
	}{
		{
			name: "get uname cpu information",
			args: args{
				syscallUname: func(in *syscall.Utsname) error {
					in.Machine = [65]int8{120, 56, 54, 95, 54, 52}
					return nil
				},
			},
			want: &CPU{
				HostMachine:   "x86_64",
				HostProcessor: "x86_64",
			},
		},
		{
			name: "get uname cpu information and the call to uname fails",
			args: args{
				syscallUname: func(in *syscall.Utsname) error {
					in.Machine = [65]int8{}
					return errors.New("shouldn't work")
				},
			},
			want:    &CPU{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syscallUname = tt.args.syscallUname
			in := &CPU{}
			if err := fillPlatformSpecificCPUData(in); err != nil {
				if !tt.wantErr {
					t.Errorf("fillPlatformSpecificCPUData returned an error %v", err)
				}
				return
			}
			if !reflect.DeepEqual(in, tt.want) {
				t.Errorf("fillPlatformSpecificCPUData() = %v, want %v", in, tt.want)
			}
		})
		os.Unsetenv("HOST_ETC")
		syscallUname = syscall.Uname
	}
}
