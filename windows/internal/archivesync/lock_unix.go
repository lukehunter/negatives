//go:build !windows

package archivesync

import (
	"errors"
	"os"
	"syscall"
)

func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func heldElsewhere(err error) bool { return errors.Is(err, syscall.EWOULDBLOCK) }
