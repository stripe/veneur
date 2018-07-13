// +build linux

package hostmetadata

import (
	"syscall"
)

// syscallUname maps to the golib system call, but can be modified for testing
var syscallUname = syscall.Uname

func fillPlatformSpecificOSData(info *OS) error {
	info.HostLinuxVersion, _ = GetLinuxVersion()

	uname := &syscall.Utsname{}
	if err := syscallUname(uname); err != nil {
		return err
	}

	info.HostKernelVersion = string(int8ArrayToByteArray(uname.Version[:]))
	return nil
}

func fillPlatformSpecificCPUData(info *CPU) error {
	uname := &syscall.Utsname{}
	if err := syscallUname(uname); err != nil {
		return err
	}

	info.HostMachine = string(int8ArrayToByteArray(uname.Machine[:]))

	// according to the python doc platform.Processor usually returns the same
	// value as platform.Machine
	// https://docs.python.org/3/library/platform.html#platform.processor
	info.HostProcessor = info.HostMachine
	return nil
}
