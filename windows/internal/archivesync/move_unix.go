//go:build !windows

package archivesync

import (
	"fmt"
	"os"
)

// moveNoReplace renames a file, refusing if the destination exists. This
// version is for tests on macOS and Linux, where the check and the rename
// are two steps; the Windows version does both in one call.
func moveNoReplace(from, to string) error {
	if _, err := os.Lstat(to); err == nil {
		return fmt.Errorf("%s already exists", to)
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Rename(from, to)
}
