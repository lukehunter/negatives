// Package archivesync copies the primary archive disk to the backup disk,
// additively, with robocopy. See docs/tools/archive-sync.md.
//
// The decisions (which robocopy jobs to run, what a difference between the
// disks means, what may be quarantined) are pure functions in layout.go,
// plan.go, robocopy.go and reconcile.go. The rest reads the disks and carries
// those decisions out.
package archivesync

import (
	"path"
	"strings"
)

// Names fixed by the folder layout in docs/design.md.
const (
	MarkerName    = ".negatives"
	OriginalsDir  = "Originals"
	DerivedDir    = "Derived"
	PartialSuffix = ".partial"
)

// systemDirs are top-level folders Windows keeps on every NTFS volume. They
// are never copied and never compared.
var systemDirs = []string{"$RECYCLE.BIN", "System Volume Information"}

// Kind says how a file on the primary disk is protected.
type Kind int

const (
	// KindLegacy: in a shoot folder without a .negatives marker, or a file
	// at the disk root. Copied additively; treated like an original.
	KindLegacy Kind = iota
	// KindOriginal: under a negatives shoot's Originals folder. Never
	// overwritten on either disk.
	KindOriginal
	// KindDerived: under a negatives shoot's Derived folder. Replaceable;
	// pass 2 copies it with overwrites allowed.
	KindDerived
	// KindShootOther: anywhere else in a negatives shoot, such as the
	// marker itself. Copied additively.
	KindShootOther
)

func (k Kind) String() string {
	switch k {
	case KindLegacy:
		return "legacy"
	case KindOriginal:
		return "original"
	case KindDerived:
		return "derived"
	case KindShootOther:
		return "shoot"
	}
	return "unknown"
}

// Fold is the case folding used to compare names. NTFS, APFS and exFAT are
// case-insensitive by default, so PRIVATE and private are the same folder.
func Fold(name string) string { return strings.ToLower(name) }

// Shoots is the set of top-level folders on the primary disk that hold a
// .negatives marker, keyed by folded name.
type Shoots map[string]bool

// Marked reports whether the top-level folder name is a negatives shoot.
func (s Shoots) Marked(name string) bool { return s[Fold(name)] }

// Classify returns the Kind of the file at rel, a "/"-separated path
// relative to the disk root.
func Classify(rel string, shoots Shoots) Kind {
	seg := strings.Split(rel, "/")
	if len(seg) < 2 || !shoots.Marked(seg[0]) {
		return KindLegacy
	}
	if len(seg) >= 3 {
		switch Fold(seg[1]) {
		case Fold(OriginalsDir):
			return KindOriginal
		case Fold(DerivedDir):
			return KindDerived
		}
	}
	return KindShootOther
}

// IsDerivedDir reports whether rel is a negatives shoot's Derived folder
// itself (not something inside it).
func IsDerivedDir(rel string, shoots Shoots) bool {
	seg := strings.Split(rel, "/")
	return len(seg) == 2 && shoots.Marked(seg[0]) && Fold(seg[1]) == Fold(DerivedDir)
}

// IsPartial reports whether name is an in-progress temporary file written
// by ssd-push. These are never copied to the backup disk.
func IsPartial(name string) bool {
	return strings.HasSuffix(Fold(name), PartialSuffix)
}

// SkipTop reports whether a top-level entry is never copied or compared:
// Windows system folders, and the quarantine folder's name (which lives at
// the backup root and must never be a copy destination).
func SkipTop(name, quarantineName string) bool {
	f := Fold(name)
	for _, s := range systemDirs {
		if f == Fold(s) {
			return true
		}
	}
	return quarantineName != "" && f == Fold(quarantineName)
}

// ValidRel reports whether rel is a clean relative path: "/"-separated, no
// empty, "." or ".." segments, not absolute, no drive letter or backslash.
func ValidRel(rel string) bool {
	if rel == "" || strings.ContainsAny(rel, `\:`) || path.IsAbs(rel) {
		return false
	}
	for _, s := range strings.Split(rel, "/") {
		if s == "" || s == "." || s == ".." {
			return false
		}
	}
	return true
}
