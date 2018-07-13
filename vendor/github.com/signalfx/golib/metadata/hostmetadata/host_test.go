package hostmetadata

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
)

func TestGetCPU(t *testing.T) {
	type testfixture struct {
		cpuInfo   func() ([]cpu.InfoStat, error)
		cpuCounts func(bool) (int, error)
	}
	tests := []struct {
		name     string
		fixtures testfixture
		wantInfo map[string]string
		wantErr  bool
	}{
		{
			name: "successful host cpu info",
			fixtures: testfixture{
				cpuInfo: func() ([]cpu.InfoStat, error) {
					return []cpu.InfoStat{
						{
							ModelName: "testmodelname",
							Cores:     4,
						},
						{
							ModelName: "testmodelname2",
							Cores:     4,
						},
					}, nil
				},
				cpuCounts: func(bool) (int, error) {
					return 2, nil
				},
			},
			wantInfo: map[string]string{
				"host_physical_cpus": "2",
				"host_cpu_cores":     "8",
				"host_cpu_model":     "testmodelname2",
				"host_logical_cpus":  "2",
			},
		},
		{
			name: "unsuccessful host cpu info (missing cpu info)",
			fixtures: testfixture{
				cpuInfo: func() ([]cpu.InfoStat, error) {
					return nil, errors.New("bad cpu info")
				},
			},
			wantInfo: map[string]string{
				"host_physical_cpus": "0",
				"host_cpu_cores":     "0",
				"host_cpu_model":     "",
				"host_logical_cpus":  "0",
			},
			wantErr: true,
		},
		{
			name: "unsuccessful host cpu info (missing cpu counts)",
			fixtures: testfixture{
				cpuInfo: func() ([]cpu.InfoStat, error) {
					return []cpu.InfoStat{
						{
							ModelName: "testmodelname",
							Cores:     4,
						},
						{
							ModelName: "testmodelname2",
							Cores:     4,
						},
					}, nil
				},
				cpuCounts: func(bool) (int, error) {
					return 0, errors.New("bad cpu counts")
				},
			},
			wantInfo: map[string]string{
				"host_physical_cpus": "2",
				"host_cpu_cores":     "0",
				"host_cpu_model":     "",
				"host_logical_cpus":  "0",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cpuInfo = tt.fixtures.cpuInfo
			cpuCounts = tt.fixtures.cpuCounts
			gotInfo, err := GetCPU()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCPU() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			gotMap := gotInfo.ToStringMap()
			for k, v := range tt.wantInfo {
				if gv, ok := gotMap[k]; !ok || v != gv {
					t.Errorf("GetCPU() expected key '%s' was found %v.  Expected value '%s' and got '%s'.", k, ok, v, gv)
				}
			}
		})
	}
}

func TestGetOS(t *testing.T) {
	type testfixture struct {
		hostInfo func() (*host.InfoStat, error)
		hostEtc  string
	}
	tests := []struct {
		name         string
		testfixtures testfixture
		wantInfo     map[string]string
		wantErr      bool
	}{
		{
			name: "get kernel info",
			testfixtures: testfixture{
				hostInfo: func() (*host.InfoStat, error) {
					return &host.InfoStat{
						OS:              "linux",
						KernelVersion:   "4.4.0-112-generic",
						Platform:        "ubuntu",
						PlatformVersion: "16.04",
					}, nil
				},
				hostEtc: "./testdata/lsb-release",
			},
			wantInfo: map[string]string{
				"host_kernel_name":    "linux",
				"host_kernel_release": "4.4.0-112-generic",
				"host_os_name":        "ubuntu",
			},
		},
		{
			name: "get kernel info error",
			testfixtures: testfixture{
				hostInfo: func() (*host.InfoStat, error) {
					return nil, errors.New("no host info")
				},
				hostEtc: "./testdata/lsb-release",
			},
			wantInfo: map[string]string{
				"host_kernel_name":    "",
				"host_kernel_version": "",
				"host_os_name":        "",
				"host_linux_version":  "",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hostInfo = tt.testfixtures.hostInfo
			if err := os.Setenv("HOST_ETC", tt.testfixtures.hostEtc); err != nil {
				t.Errorf("GetOS() error = %v failed to set HOST_ETC env var", err)
				return
			}
			gotInfo, err := GetOS()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetOS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			gotMap := gotInfo.ToStringMap()
			for k, v := range tt.wantInfo {
				if gv, ok := gotMap[k]; !ok || v != gv {
					t.Errorf("GetOS() expected key '%s' was found %v.  Expected value '%s' and got '%s'.", k, ok, v, gv)
				}
			}
		})
		os.Unsetenv("HOST_ETC")
	}
}

func Test_GetLinuxVersion(t *testing.T) {
	tests := []struct {
		name    string
		etc     string
		want    string
		wantErr bool
	}{
		{
			name: "lsb-release",
			etc:  "./testdata/lsb-release",
			want: "Ubuntu 18.04 LTS",
		},
		{
			name: "os-release",
			etc:  "./testdata/os-release",
			want: "Debian GNU/Linux 9 (stretch)",
		},
		{
			name: "centos-release",
			etc:  "./testdata/centos-release",
			want: "CentOS Linux release 7.5.1804 (Core)",
		},
		{
			name: "redhat-release",
			etc:  "./testdata/redhat-release",
			want: "Red Hat Enterprise Linux Server release 7.5 (Maipo)",
		},
		{
			name: "system-release",
			etc:  "./testdata/system-release",
			want: "CentOS Linux release 7.5.1804 (Core)",
		},
		{
			name:    "no release returns error",
			etc:     "./testdata",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Setenv("HOST_ETC", tt.etc); err != nil {
				t.Errorf("GetLinuxVersion() error = %v failed to set HOST_ETC env var", err)
				return
			}
			got, err := GetLinuxVersion()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLinuxVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetLinuxVersion() = %v, want %v", got, tt.want)
			}
		})
		os.Unsetenv("HOST_ETC")
	}
}

func TestGetMemory(t *testing.T) {
	tests := []struct {
		name             string
		memVirtualMemory func() (*mem.VirtualMemoryStat, error)
		want             map[string]string
		wantErr          bool
	}{
		{
			name: "memory utilization",
			memVirtualMemory: func() (*mem.VirtualMemoryStat, error) {
				return &mem.VirtualMemoryStat{
					Total: 1024,
				}, nil
			},
			want: map[string]string{"host_mem_total": strconv.FormatFloat(1, 'f', 6, 64)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memVirtualMemory = tt.memVirtualMemory
			mem, err := GetMemory()
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMemory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			got := mem.ToStringMap()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetMemory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHostEtc(t *testing.T) {
	tests := []struct {
		name string
		etc  string
		want string
	}{
		{
			name: "test default host etc",
			etc:  "",
			want: "/etc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.etc != "" {
				if err := os.Setenv("HOST_ETC", tt.etc); err != nil {
					t.Errorf("HostEtc error = %v failed to set HOST_ETC env var", err)
					return
				}
			}
			if got := HostEtc(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("HostEtc = %v, want %v", got, tt.want)
			}
		})
		os.Unsetenv("HOST_ETC")
	}

}

func TestInt8ArrayToByteArray(t *testing.T) {
	type args struct {
		in []int8
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "convert int8 array to byte array",
			args: args{
				in: []int8{72, 69, 76, 76, 79, 32, 87, 79, 82, 76, 68},
			},
			want: []byte{72, 69, 76, 76, 79, 32, 87, 79, 82, 76, 68},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := int8ArrayToByteArray(tt.args.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("int8ArrayToByteArray() = %v, want %v", got, tt.want)
			}
		})
	}
}
