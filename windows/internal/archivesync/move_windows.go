//go:build windows

package archivesync

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// moveNoReplace renames a file within one volume. MoveFileEx with no flags
// fails if the destination exists (no MOVEFILE_REPLACE_EXISTING) and fails
// across volumes rather than copying and deleting (no
// MOVEFILE_COPY_ALLOWED), so it can neither overwrite anything nor leave
// the backup disk.
func moveNoReplace(from, to string) error {
	f, err := windows.UTF16PtrFromString(longPath(from))
	if err != nil {
		return err
	}
	t, err := windows.UTF16PtrFromString(longPath(to))
	if err != nil {
		return err
	}
	return windows.MoveFileEx(f, t, 0)
}

// longPath adds the \\?\ prefix so paths over 260 characters work, as the
// os package does for its own calls.
func longPath(p string) string {
	if strings.HasPrefix(p, `\\`) || !filepath.IsAbs(p) {
		return p
	}
	return `\\?\` + filepath.Clean(p)
}
