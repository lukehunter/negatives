package catalog

import (
	"crypto/md5"
	"encoding/hex"
	"io"
)

// State is a file's derived state, computed by readers and never stored
// (docs/design.md, Derived states).
type State string

const (
	// Safe: at least one verified copy on the primary disk and one on the
	// backup disk.
	Safe State = "safe"
	// Pending: in the catalog, but missing a verified copy on primary or
	// backup.
	Pending State = "pending"
	// NotCopied: no document for the hash.
	NotCopied State = "not-copied"
	// Ignored: a zero-byte file, never cataloged.
	Ignored State = "ignored"
)

// Summary is what a state needs from files/{md5}: how many verified copies
// each archive disk holds (primary.copies and backup.copies). archive-hasher
// leaves copies marked missing out of these counts.
type Summary struct {
	PrimaryCopies int
	BackupCopies  int
}

// StateOf returns the state of a file of the given size whose summary
// document is summary, or nil if there is none. Anything short of a
// verified copy on both disks is Pending, never Safe.
func StateOf(size int64, summary *Summary) State {
	switch {
	case size == 0:
		return Ignored
	case summary == nil:
		return NotCopied
	case summary.PrimaryCopies > 0 && summary.BackupCopies > 0:
		return Safe
	}
	return Pending
}

// MD5Hex streams r and returns its MD5 as lowercase hex, the file identity
// used throughout the catalog.
func MD5Hex(r io.Reader) (string, error) {
	h := md5.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
