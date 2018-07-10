// +build !linux

package hostmetadata

func fillPlatformSpecificOSData(info *OS) error {
	return nil
}

func fillPlatformSpecificCPUData(info *CPU) error {
	return nil
}
