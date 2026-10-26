package hasher

import (
	"sort"
	"strings"
	"time"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
)

// Hashed applies a fresh hash of a file the walk found to what was known
// at its path, and returns the new entry and any event. md5 is the hash of
// the file exactly as s describes it.
//
//   - Nothing known: a new entry, verified now.
//   - Same content (a touched time, or a missing file come back): the entry
//     keeps its first-verified time and is present again.
//   - New content at a protected path: a Conflict. The new content gets
//     the entry, the old hash goes into Previous, and the catalog marks the
//     old fact missing; nothing is ever fixed automatically.
//   - New content at a replaceable path: Replaced, the normal case for
//     Derived/ files.
func Hashed(prev *Entry, s Seen, md5 string, now time.Time, shoots archivesync.Shoots) (Entry, *Event) {
	next := Entry{Disk: s.Disk, Path: s.Path, Size: s.Size, MtimeNs: s.MtimeNs, MD5: md5, VerifiedAt: now}
	if prev == nil {
		return next, nil
	}
	next.Previous = append([]string(nil), prev.Previous...)
	if prev.MD5 == md5 {
		next.VerifiedAt = prev.VerifiedAt
		if !prev.Present() {
			return next, &Event{Kind: Returned, Disk: s.Disk, Path: s.Path, OldMD5: md5, NewMD5: md5, At: now}
		}
		return next, nil
	}
	next.Previous = append(next.Previous, prev.MD5)
	kind := Replaced
	if Protected(s.Path, shoots) {
		kind = Conflict
	}
	return next, &Event{Kind: kind, Disk: s.Disk, Path: s.Path, OldMD5: prev.MD5, NewMD5: md5, At: now}
}

// MarkMissing marks entries the walk didn't find, but only under roots the
// walk completed: complete holds disk roots' top-level names (folded) whose
// whole tree was listed without error, plus "" for files at a disk's root.
// A folder that couldn't be read, or a walk that stopped early, never makes
// a file missing: any doubt keeps the fact as it was. It returns the
// changed entries and a Missing event for each.
func MarkMissing(state map[Key]Entry, seen map[Key]bool, complete map[Disk]map[string]bool, now time.Time) (map[Key]Entry, []Event) {
	changed := map[Key]Entry{}
	var events []Event
	keys := make([]Key, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Disk != keys[j].Disk {
			return keys[i].Disk < keys[j].Disk
		}
		return keys[i].Fold < keys[j].Fold
	})
	for _, k := range keys {
		e := state[k]
		if !e.Present() || seen[k] || !complete[k.Disk][topOf(k.Fold)] {
			continue
		}
		t := now
		e.MissingSince = &t
		changed[k] = e
		events = append(events, Event{Kind: Missing, Disk: e.Disk, Path: e.Path, OldMD5: e.MD5, At: now})
	}
	return changed, events
}

// topOf is a folded path's top-level folder, or "" for a file at the root.
func topOf(fold string) string {
	top, _, found := strings.Cut(fold, "/")
	if !found {
		return ""
	}
	return top
}
