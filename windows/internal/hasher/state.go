// Package hasher is archive-hasher's decision core: which files need
// hashing, what a new hash at a known path means, when a file counts as
// missing, and each hash's summary document. Everything here is a pure
// function of the local state and what a walk saw, so it's property-tested
// without disks or Firestore. See docs/tools/archive-hasher.md.
package hasher

import (
	"time"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
	"github.com/lukehunter/negatives/windows/internal/catalog"
)

// Disk is an archive disk.
type Disk string

const (
	Primary Disk = "primary"
	Backup  Disk = "backup"
)

// Loc is the catalog location for the disk.
func (d Disk) Loc() catalog.Loc { return catalog.Loc(d) }

// Key identifies one file in the local state: its disk and folded path.
type Key struct {
	Disk Disk
	Fold string // catalog.FoldPath of the normalized path
}

// Entry is what the local state knows about one file on one disk.
type Entry struct {
	Disk Disk
	Path string // normalized, case as the disk reports it
	// Size and MtimeNs are the file as it was hashed.
	Size    int64
	MtimeNs int64
	MD5     string
	// VerifiedAt is when this content was first hashed at this path.
	VerifiedAt time.Time
	// MissingSince is set when a complete walk no longer found the file.
	// A missing copy doesn't count toward its hash's copies.
	MissingSince *time.Time
	// Previous lists earlier contents seen at this path, oldest first, so
	// archive-sync's reconcile can tell the primary was edited in place.
	Previous []string
}

// KeyOf returns the state key for a normalized path on a disk.
func KeyOf(d Disk, path string) Key { return Key{Disk: d, Fold: catalog.FoldPath(path)} }

// Present reports whether the copy counts: not marked missing.
func (e Entry) Present() bool { return e.MissingSince == nil }

// Seen is one regular file a walk found.
type Seen struct {
	Disk    Disk
	Path    string // normalized
	Size    int64
	MtimeNs int64
}

// NeedsHash reports whether a file the walk found must be (re)hashed:
// nothing is known at its path, its size or time changed, or it was marked
// missing and has come back. Zero-byte files are never hashed.
func NeedsHash(prev *Entry, s Seen) bool {
	if s.Size == 0 {
		return false
	}
	return prev == nil || !prev.Present() || prev.Size != s.Size || prev.MtimeNs != s.MtimeNs
}

// EventKind is something a person or the catalog must hear about.
type EventKind string

const (
	// Conflict: new content at a path whose old content must never change
	// (Originals/ or a legacy folder). Reported to conflicts as
	// content-changed. The old content is no longer on this disk at this
	// path, so its fact is marked missing rather than replaced silently.
	Conflict EventKind = "content-changed"
	// Replaced: new content at a replaceable path (Derived/ and other
	// files in a negatives shoot). The fact moves to the new hash.
	Replaced EventKind = "replaced"
	// Missing: a complete walk didn't find a known file. Alert naming it.
	Missing EventKind = "missing"
	// Returned: a file marked missing was found again with its content.
	Returned EventKind = "returned"
)

// Event is one thing a run reports.
type Event struct {
	Kind   EventKind
	Disk   Disk
	Path   string
	OldMD5 string
	NewMD5 string
	At     time.Time
}

// Protected reports whether content at path must never change in place:
// under a negatives shoot's Originals/, or anywhere outside a negatives
// shoot (legacy folders and files at the root).
func Protected(path string, shoots archivesync.Shoots) bool {
	switch archivesync.Classify(path, shoots) {
	case archivesync.KindOriginal, archivesync.KindLegacy:
		return true
	}
	return false
}
