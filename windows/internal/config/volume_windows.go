//go:build windows

package config

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// volumeID returns the serial number of the volume that holds path, so two
// drive letters or mounted folders on one disk are recognized as the same.
func volumeID(path string) (string, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(p, &buf[0], uint32(len(buf))); err != nil {
		return "", fmt.Errorf("finding the volume of %s: %w", path, err)
	}
	var serial uint32
	if err := windows.GetVolumeInformation(&buf[0], nil, 0, &serial, nil, nil, nil, 0); err != nil {
		return "", fmt.Errorf("reading the volume of %s: %w", path, err)
	}
	return fmt.Sprintf("%08x", serial), nil
}
