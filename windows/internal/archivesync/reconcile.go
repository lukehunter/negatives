package archivesync

import (
	"context"
	"fmt"
	"path/filepath"
)

// FileInfo is what reconcile compares: size and modification time.
type FileInfo struct {
	Size    int64
	MtimeNs int64
}

// Facts is what archive-hasher has established about a primary copy and
// its backup counterpart. Only a FactSource fills it; the zero value means
// "nothing known", which never allows a quarantine.
type Facts struct {
	// Known: a fact source answered for this path at all.
	Known bool
	// Verified: archive-hasher has hashed the primary copy at this path,
	// and Size and MtimeNs are the size and time it hashed.
	Verified bool
	Size     int64
	MtimeNs  int64
	MD5      string
	// CardMatch: MD5 is the hash read from a card (a card or SSD originals
	// copy of the same content is in the catalog).
	CardMatch bool
	// PathChanged: archive-hasher has seen different content at this
	// primary path before, so the primary may have been edited in place
	// and the backup may hold the original.
	PathChanged bool
	// BackupContentKnown: archive-hasher has hashed the backup file as it
	// is now, and that content is real data known elsewhere (seen on a
	// card, or verified at another path), so it isn't a partial copy.
	BackupContentKnown bool
}

// FactSource answers for a primary copy. archive-hasher's state and the
// catalog provide one from build step 3; until then NoFacts is used.
type FactSource interface {
	Lookup(ctx context.Context, rel string, primary FileInfo) (Facts, error)
}

// NoFacts knows nothing, so reconcile reports partial copies but never
// quarantines them.
type NoFacts struct{}

func (NoFacts) Lookup(context.Context, string, FileInfo) (Facts, error) { return Facts{}, nil }

// Pair is a file on the primary disk and whatever is at the same path on
// the backup disk.
type Pair struct {
	Rel     string // "/"-separated, relative to the disk root
	Kind    Kind
	Primary FileInfo
	// Backup is nil when nothing is at that path on the backup disk.
	Backup *FileInfo
	// BackupIsDir: the backup has a folder where the primary has a file.
	BackupIsDir bool
}

// Verdict is what reconcile concluded about one path.
type Verdict int

const (
	// Same: same size and time on both disks.
	Same Verdict = iota
	// Missing: not on the backup yet; pass 1 copies it.
	Missing
	// MissingDir: a whole folder not on the backup yet; pass 1 copies it.
	MissingDir
	// Extra: only on the backup disk. Kept, never deleted.
	Extra
	// Replaceable: in a Derived folder or a .partial file; pass 2 or a
	// later push takes care of it.
	Replaceable
	// NotRegular: a link, junction or device; never followed or copied.
	NotRegular
	// TypeMismatch: a file on one disk, a folder on the other.
	TypeMismatch
	// TimeDiffers: same size, different time. Could be a different file
	// of the same size: archive-hasher compares the hashes and reports a
	// conflict if they differ. Never moved.
	TimeDiffers
	// BackupLarger: the backup copy is larger than the primary, so it
	// can't be a partial copy of it. Never moved; resolve by hand.
	BackupLarger
	// HoldUnverified: the backup copy is smaller, but archive-hasher
	// hasn't verified the primary copy as it is now. Kept until it has.
	HoldUnverified
	// HoldPathChanged: the primary path has held other content before, so
	// the backup may be the original. Never moved; resolve by hand.
	HoldPathChanged
	// HoldNoCardMatch: an original whose primary hash doesn't match any
	// card hash. Never moved; resolve by hand.
	HoldNoCardMatch
	// HoldBackupKnown: the backup content is real data known elsewhere,
	// not a partial copy. Never moved; resolve by hand.
	HoldBackupKnown
	// Quarantine: a partial copy on the backup disk. It's moved to the
	// quarantine folder and pass 1 recopies it.
	Quarantine
)

var verdictNames = map[Verdict]string{
	Same: "same", Missing: "missing-on-backup", MissingDir: "folder-missing-on-backup",
	Extra: "only-on-backup", Replaceable: "replaceable", NotRegular: "not-a-regular-file",
	TypeMismatch: "file-vs-folder", TimeDiffers: "same-size-time-differs",
	BackupLarger: "backup-larger", HoldUnverified: "partial-awaiting-verification",
	HoldPathChanged: "primary-content-changed", HoldNoCardMatch: "original-not-card-hash",
	HoldBackupKnown: "backup-content-known", Quarantine: "quarantine",
}

func (v Verdict) String() string {
	if s, ok := verdictNames[v]; ok {
		return s
	}
	return fmt.Sprintf("verdict(%d)", int(v))
}

// NeedsAttention reports whether the verdict is something a person should
// look at, rather than routine work the next pass does.
func (v Verdict) NeedsAttention() bool {
	switch v {
	case TypeMismatch, TimeDiffers, BackupLarger, HoldPathChanged, HoldNoCardMatch, HoldBackupKnown:
		return true
	}
	return false
}

// NeedsFacts reports whether Decide's answer for p depends on facts: only
// when the backup copy is a smaller regular file. Facts are looked up only
// then, which is rare.
func NeedsFacts(p Pair) bool {
	return p.Kind != KindDerived && !p.BackupIsDir && p.Backup != nil && p.Backup.Size < p.Primary.Size
}

// Decide is the reconcile rule. A backup copy is quarantined only when all
// of these hold:
//
//   - it's not in a Derived folder (pass 2 overwrites those anyway);
//   - it's smaller than the primary copy (a partial copy is never larger;
//     same size is for archive-hasher to judge by hash);
//   - archive-hasher has verified the primary copy exactly as it is now
//     (same size and time as it hashed);
//   - the primary path has never held other content, so the backup can't
//     be an original kept from before an in-place edit;
//   - the backup content isn't real data known elsewhere;
//   - for originals, the primary hash matches a card hash.
//
// Everything else is kept where it is.
func Decide(p Pair, f Facts) Verdict {
	switch {
	case p.Kind == KindDerived:
		return Replaceable
	case p.BackupIsDir:
		return TypeMismatch
	case p.Backup == nil:
		return Missing
	case p.Backup.Size == p.Primary.Size:
		if p.Backup.MtimeNs == p.Primary.MtimeNs {
			return Same
		}
		return TimeDiffers
	case p.Backup.Size > p.Primary.Size:
		return BackupLarger
	}
	// The backup copy is smaller: possibly a partial copy.
	switch {
	case !f.Known || !f.Verified || f.Size != p.Primary.Size || f.MtimeNs != p.Primary.MtimeNs:
		return HoldUnverified
	case f.PathChanged:
		return HoldPathChanged
	case f.BackupContentKnown:
		return HoldBackupKnown
	case p.Kind == KindOriginal && !f.CardMatch:
		return HoldNoCardMatch
	}
	return Quarantine
}

// QuarantinePath is where the backup file at rel goes: the same relative
// path inside a folder for this run, so nothing from two runs collides.
func QuarantinePath(quarantine, runID, rel string) (string, error) {
	if !ValidRel(rel) || !ValidRel(runID) || filepath.Base(runID) != runID {
		return "", fmt.Errorf("unsafe quarantine path: run %q, file %q", runID, rel)
	}
	return filepath.Join(quarantine, runID, filepath.FromSlash(rel)), nil
}
