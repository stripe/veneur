package hostmetadata

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"github.com/signalfx/golib/dataunit"
)

// HostEtc is the path to host etc and can be set using the env var "HOST_ETC"
// this is to maintain consistency with gopsutil
var HostEtc = func() string {
	if etcPath := os.Getenv("HOST_ETC"); etcPath != "" {
		return etcPath
	}
	return "/etc"
}

// Map library functions to unexported package variables for testing purposes.
// It would be great if we could patch this somehow
var cpuInfo = cpu.Info
var cpuCounts = cpu.Counts
var memVirtualMemory = mem.VirtualMemory
var hostInfo = host.Info

// CPU information about the host
type CPU struct {
	HostPhysicalCPUs int
	HostLogicalCPUs  int
	HostCPUCores     int64
	HostCPUModel     string
	HostMachine      string
	HostProcessor    string
}

// ToStringMap returns the CPU as a string map
func (c *CPU) ToStringMap() map[string]string {
	return map[string]string{
		"host_physical_cpus": strconv.Itoa(c.HostPhysicalCPUs),
		"host_cpu_cores":     strconv.FormatInt(c.HostCPUCores, 10),
		"host_cpu_model":     c.HostCPUModel,
		"host_logical_cpus":  strconv.Itoa(c.HostLogicalCPUs),
		"host_processor":     c.HostProcessor,
		"host_machine":       c.HostMachine,
	}
}

// GetCPU - adds information about the host cpu to the supplied map
func GetCPU() (info *CPU, err error) {
	info = &CPU{}

	// get physical cpu stats
	var cpus []cpu.InfoStat
	if cpus, err = cpuInfo(); err != nil {
		return info, err
	}

	info.HostPhysicalCPUs = len(cpus)

	// get logical cpu stats
	if info.HostLogicalCPUs, err = cpuCounts(true); err != nil {
		return info, err
	}

	// total number of cpu cores
	for _, cpu := range cpus {
		info.HostCPUCores += int64(cpu.Cores)
		// TODO: This is not ideal... if there are different processors
		// we will only report one of the models... This is unlikely to happen,
		// but it could
		info.HostCPUModel = cpu.ModelName
	}

	err = fillPlatformSpecificCPUData(info)
	return info, err
}

// OS is a struct containing information about the host os
type OS struct {
	HostOSName        string
	HostKernelName    string
	HostKernelRelease string
	HostKernelVersion string
	HostLinuxVersion  string
}

// ToStringMap returns a map of key/value metadata about the host os
func (o *OS) ToStringMap() map[string]string {
	return map[string]string{
		"host_kernel_name":    o.HostKernelName,
		"host_kernel_release": o.HostKernelRelease,
		"host_kernel_version": o.HostKernelVersion,
		"host_os_name":        o.HostOSName,
		"host_linux_version":  o.HostLinuxVersion,
	}
}

// int8ArrayToByteArray converts an []int8 to []byte
func int8ArrayToByteArray(in []int8) []byte {
	bts := make([]byte, len(in))
	for i, c := range in {
		bts[i] = byte(c)
	}
	return bytes.Trim(bts, "\x00")
}

// GetOS returns a struct with information about the host os
func GetOS() (info *OS, err error) {
	info = &OS{}
	hInfo, err := hostInfo()

	if err != nil {
		return info, err
	}

	info.HostOSName = hInfo.Platform
	info.HostKernelName = hInfo.OS
	// in gopsutil KernelVersion returns what we would expect for Kernel Release
	info.HostKernelRelease = hInfo.KernelVersion
	err = fillPlatformSpecificOSData(info)
	return info, err
}

// GetLinuxVersion - adds information about the host linux version to the supplied map
func GetLinuxVersion() (string, error) {
	etc := HostEtc()
	if value, err := getStringFromFile(`DISTRIB_DESCRIPTION="(.*)"`, filepath.Join(etc, "lsb-release")); err == nil {
		return value, nil
	}
	if value, err := getStringFromFile(`PRETTY_NAME="(.*)"`, filepath.Join(etc, "os-release")); err == nil {
		return value, nil
	}
	if value, err := ioutil.ReadFile(filepath.Join(etc, "centos-release")); err == nil {
		return string(value), nil
	}
	if value, err := ioutil.ReadFile(filepath.Join(etc, "redhat-release")); err == nil {
		return string(value), nil
	}
	if value, err := ioutil.ReadFile(filepath.Join(etc, "system-release")); err == nil {
		return string(value), nil
	}
	return "", errors.New("unable to find linux version")
}

// Memory stores memory collected from the host
type Memory struct {
	Total dataunit.Size
}

// ToStringMap returns a map of key/value metadata about the host memory
// where memory sizes are reported in Kb
func (m *Memory) ToStringMap() map[string]string {
	return map[string]string{
		"host_mem_total": strconv.FormatFloat(m.Total.Kilobytes(), 'f', 6, 64),
	}
}

// GetMemory returns the amount of memory on the host as datatype.USize
func GetMemory() (*Memory, error) {
	m := &Memory{}
	memoryStat, err := memVirtualMemory()
	if err == nil {
		m.Total = dataunit.Size(int64(memoryStat.Total))
	}
	return m, err
}

func getStringFromFile(pattern string, path string) (string, error) {
	var err error
	var file []byte
	var reg = regexp.MustCompile(pattern)
	if file, err = ioutil.ReadFile(path); err == nil {
		if match := reg.FindSubmatch(file); len(match) > 1 {
			return string(match[1]), nil
		}
	}
	return "", err
}
