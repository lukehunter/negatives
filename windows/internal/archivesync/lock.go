package archivesync

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrLocked means another archive-sync run is in progress.
var ErrLocked = errors.New("another archive-sync run is in progress")

// Lock keeps a manual run from overlapping a scheduled one. It's an
// operating-system lock on a file, so it's released when the process exits
// for any reason, and a crash never leaves a stale lock behind.
type Lock struct{ f *os.File }

// AcquireLock takes the lock at path, or returns ErrLocked at once if
// another process holds it. Any other error (the lock file can't be opened
// or locked for another reason) is returned as it is.
func AcquireLock(path string) (*Lock, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if heldElsewhere(err) {
			return nil, fmt.Errorf("%w (lock %s)", ErrLocked, path)
		}
		return nil, fmt.Errorf("locking %s: %w", path, err)
	}
	// For a person looking at the file; the lock itself is what counts.
	_ = f.Truncate(0)
	_, _ = fmt.Fprintf(f, "pid %d since %s\n", os.Getpid(), time.Now().Format(time.RFC3339))
	return &Lock{f: f}, nil
}

// Release gives the lock up.
func (l *Lock) Release() error { return l.f.Close() }
