//go:build !windows

package config

import (
	"fmt"
	"os"
	"syscall"
)

// volumeID returns the device number of the file system that holds path.
func volumeID(path string) (string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	sys, ok := st.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("no device number for %s", path)
	}
	return fmt.Sprint(sys.Dev), nil
}
