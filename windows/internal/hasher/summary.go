package hasher

import (
	"strings"
	"time"

	"github.com/lukehunter/negatives/windows/internal/catalog"
)

// DiskSummary is one disk's part of files/{md5}: primary or backup.
type DiskSummary struct {
	// FirstVerifiedAt is the earliest VerifiedAt of any copy of this
	// content on the disk, missing or not, so history survives a lost copy.
	FirstVerifiedAt time.Time
	// Copies counts copies present on the disk. Missing copies don't count,
	// so a lost copy stops the content being Safe.
	Copies int
}

// Summary is the archive-hasher half of files/{md5}.
type Summary struct {
	Size    int64
	Primary *DiskSummary // nil when the disk has never held this content
	Backup  *DiskSummary
	// Shoots counts distinct top-level folders holding a present copy, on
	// either disk (the same names on both). Files at a disk's root count
	// toward no shoot.
	Shoots int
	Common bool
}

// Catalog returns the counts card-check and ssd-evict decide from.
func (s Summary) Catalog() catalog.Summary {
	var c catalog.Summary
	if s.Primary != nil {
		c.PrimaryCopies = s.Primary.Copies
	}
	if s.Backup != nil {
		c.BackupCopies = s.Backup.Copies
	}
	return c
}

// Summaries computes every hash's summary from the whole local state, so
// nothing needs a read-modify-write and no transaction is involved.
// commonThreshold is config/settings commonShootThreshold.
func Summaries(state map[Key]Entry, commonThreshold int) map[string]Summary {
	out := map[string]Summary{}
	shoots := map[string]map[string]bool{}
	for _, e := range state {
		if e.Size == 0 || e.MD5 == "" {
			continue
		}
		s := out[e.MD5]
		s.Size = e.Size
		ds := &s.Primary
		if e.Disk == Backup {
			ds = &s.Backup
		}
		if *ds == nil {
			*ds = &DiskSummary{FirstVerifiedAt: e.VerifiedAt}
		}
		d := **ds
		if e.VerifiedAt.Before(d.FirstVerifiedAt) {
			d.FirstVerifiedAt = e.VerifiedAt
		}
		if e.Present() {
			d.Copies++
			if top, _, ok := strings.Cut(catalog.FoldPath(e.Path), "/"); ok {
				if shoots[e.MD5] == nil {
					shoots[e.MD5] = map[string]bool{}
				}
				shoots[e.MD5][top] = true
			}
		}
		*ds = &d
		out[e.MD5] = s
	}
	for md5, s := range out {
		s.Shoots = len(shoots[md5])
		s.Common = s.Shoots >= commonThreshold
		out[md5] = s
	}
	return out
}

// Changed returns the hashes whose summary differs from what was last
// published, so a run writes only those documents. Publishing every
// summary, or refreshing every path fact's time, on every run would cost
// millions of writes a night for a large archive.
func Changed(published, current map[string]Summary) []string {
	var out []string
	for md5, s := range current {
		if p, ok := published[md5]; !ok || !sameSummary(p, s) {
			out = append(out, md5)
		}
	}
	return out
}

func sameSummary(a, b Summary) bool {
	return a.Size == b.Size && a.Shoots == b.Shoots && a.Common == b.Common &&
		sameDisk(a.Primary, b.Primary) && sameDisk(a.Backup, b.Backup)
}

func sameDisk(a, b *DiskSummary) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Copies == b.Copies && a.FirstVerifiedAt.Equal(b.FirstVerifiedAt)
}
