package hasher

import (
	"fmt"
	"testing"
	"time"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
	"github.com/lukehunter/negatives/windows/internal/catalog"
	"pgregory.net/rapid"
)

var (
	t0     = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	shoots = archivesync.Shoots{"2026-10-18 coast": true}
)

func TestNeedsHash(t *testing.T) {
	e := &Entry{Disk: Primary, Path: "a/b.jpg", Size: 10, MtimeNs: 5, MD5: "x", VerifiedAt: t0}
	gone := *e
	gone.MissingSince = &t0
	cases := []struct {
		name string
		prev *Entry
		s    Seen
		want bool
	}{
		{"new file", nil, Seen{Primary, "a/b.jpg", 10, 5}, true},
		{"unchanged", e, Seen{Primary, "a/b.jpg", 10, 5}, false},
		{"new size", e, Seen{Primary, "a/b.jpg", 11, 5}, true},
		{"new time", e, Seen{Primary, "a/b.jpg", 10, 6}, true},
		{"back after missing", &gone, Seen{Primary, "a/b.jpg", 10, 5}, true},
		{"zero bytes", nil, Seen{Primary, "a/empty", 0, 5}, false},
	}
	for _, c := range cases {
		if got := NeedsHash(c.prev, c.s); got != c.want {
			t.Errorf("%s: NeedsHash = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHashed(t *testing.T) {
	orig := "2026-10-18 Coast/Originals/ILCE-7M4-01/DCIM/100MSDCF/DSC00001.ARW"
	der := "2026-10-18 Coast/Derived/Exports/pano.tif"
	legacy := "2019-05-04 Beach/DSC00001.JPG"
	t1 := t0.Add(time.Hour)

	e, ev := Hashed(nil, Seen{Primary, orig, 10, 5}, "aaa", t0, shoots)
	if ev != nil || e.MD5 != "aaa" || !e.VerifiedAt.Equal(t0) {
		t.Fatalf("new file: %+v %+v", e, ev)
	}

	// Same content, touched time: keeps its first-verified time, no event.
	e2, ev := Hashed(&e, Seen{Primary, orig, 10, 6}, "aaa", t1, shoots)
	if ev != nil || !e2.VerifiedAt.Equal(t0) || e2.MtimeNs != 6 {
		t.Fatalf("touched: %+v %+v", e2, ev)
	}

	// New content under Originals or in a legacy folder: a conflict.
	for _, p := range []string{orig, legacy} {
		prev := Entry{Disk: Primary, Path: p, Size: 10, MtimeNs: 5, MD5: "aaa", VerifiedAt: t0}
		e3, ev := Hashed(&prev, Seen{Primary, p, 12, 7}, "bbb", t1, shoots)
		if ev == nil || ev.Kind != Conflict || ev.OldMD5 != "aaa" || ev.NewMD5 != "bbb" {
			t.Fatalf("%s: want a conflict, got %+v", p, ev)
		}
		if e3.MD5 != "bbb" || len(e3.Previous) != 1 || e3.Previous[0] != "aaa" || !e3.VerifiedAt.Equal(t1) {
			t.Fatalf("%s: entry after conflict %+v", p, e3)
		}
	}

	// New content under Derived: a normal replacement.
	prev := Entry{Disk: Primary, Path: der, Size: 10, MtimeNs: 5, MD5: "aaa", VerifiedAt: t0}
	if _, ev := Hashed(&prev, Seen{Primary, der, 12, 7}, "bbb", t1, shoots); ev == nil || ev.Kind != Replaced {
		t.Fatalf("derived: want replaced, got %+v", ev)
	}

	// A missing file back with its content: present again, reported.
	prev.MissingSince = &t0
	e4, ev := Hashed(&prev, Seen{Primary, der, 10, 5}, "aaa", t1, shoots)
	if ev == nil || ev.Kind != Returned || !e4.Present() || !e4.VerifiedAt.Equal(t0) {
		t.Fatalf("returned: %+v %+v", e4, ev)
	}
}

func TestMarkMissingOnlyAfterCompleteWalks(t *testing.T) {
	a := Entry{Disk: Primary, Path: "Shoot A/x.jpg", Size: 1, MD5: "m1", VerifiedAt: t0}
	b := Entry{Disk: Primary, Path: "Shoot B/y.jpg", Size: 1, MD5: "m2", VerifiedAt: t0}
	root := Entry{Disk: Primary, Path: "readme.txt", Size: 1, MD5: "m3", VerifiedAt: t0}
	bk := Entry{Disk: Backup, Path: "Shoot A/x.jpg", Size: 1, MD5: "m1", VerifiedAt: t0}
	state := map[Key]Entry{}
	for _, e := range []Entry{a, b, root, bk} {
		state[KeyOf(e.Disk, e.Path)] = e
	}
	// Nothing seen; only Shoot A and the root files on primary were listed
	// completely. Shoot B failed to list, and the backup walk didn't finish.
	complete := map[Disk]map[string]bool{Primary: {"shoot a": true, "": true}}
	changed, events := MarkMissing(state, map[Key]bool{}, complete, t0)
	if len(changed) != 2 || len(events) != 2 {
		t.Fatalf("changed %v, events %v", changed, events)
	}
	if _, ok := changed[KeyOf(Primary, a.Path)]; !ok {
		t.Error("Shoot A/x.jpg not marked missing")
	}
	if _, ok := changed[KeyOf(Primary, root.Path)]; !ok {
		t.Error("readme.txt not marked missing")
	}
}

func TestSummaries(t *testing.T) {
	gone := t0.Add(time.Hour)
	state := map[Key]Entry{}
	add := func(d Disk, p, md5 string, at time.Time, missing *time.Time) {
		state[KeyOf(d, p)] = Entry{Disk: d, Path: p, Size: 10, MD5: md5, VerifiedAt: at, MissingSince: missing}
	}
	add(Primary, "Shoot A/Originals/x.arw", "m1", t0.Add(time.Minute), nil)
	add(Primary, "Shoot B/x.arw", "m1", t0, nil)
	add(Backup, "Shoot A/Originals/x.arw", "m1", t0.Add(2*time.Minute), &gone)
	add(Primary, "Shoot A/Originals/y.arw", "m2", t0, nil)
	add(Backup, "Shoot A/Originals/y.arw", "m2", t0, nil)
	add(Primary, "top.txt", "m3", t0, nil)

	s := Summaries(state, 2)
	m1 := s["m1"]
	if m1.Primary.Copies != 2 || m1.Backup.Copies != 0 || !m1.Primary.FirstVerifiedAt.Equal(t0) {
		t.Fatalf("m1 %+v %+v %+v", m1, m1.Primary, m1.Backup)
	}
	if !m1.Backup.FirstVerifiedAt.Equal(t0.Add(2 * time.Minute)) {
		t.Error("a missing copy's first-verified time was lost")
	}
	if m1.Shoots != 2 || !m1.Common {
		t.Errorf("m1 shoots %d common %v, want 2 true", m1.Shoots, m1.Common)
	}
	if c := s["m2"].Catalog(); c.PrimaryCopies != 1 || c.BackupCopies != 1 {
		t.Errorf("m2 %+v", c)
	}
	if s["m3"].Shoots != 0 {
		t.Error("a file at the root counted as a shoot")
	}
}

// Invariants over random states: copies count present copies only, and the
// summaries account for every present copy exactly once.
func TestSummaryInvariants(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		state := map[Key]Entry{}
		present := 0
		n := rapid.IntRange(0, 30).Draw(t, "n")
		for i := 0; i < n; i++ {
			d := rapid.SampledFrom([]Disk{Primary, Backup}).Draw(t, "disk")
			p := fmt.Sprintf("%s/f%d.jpg", rapid.SampledFrom([]string{"A", "B", "C"}).Draw(t, "shoot"), rapid.IntRange(0, 5).Draw(t, "file"))
			e := Entry{Disk: d, Path: p, Size: 1, MD5: rapid.SampledFrom([]string{"m1", "m2", "m3"}).Draw(t, "md5"),
				VerifiedAt: t0.Add(time.Duration(rapid.IntRange(0, 100).Draw(t, "at")) * time.Minute)}
			if rapid.Bool().Draw(t, "missing") {
				e.MissingSince = &t0
			}
			k := KeyOf(d, p)
			if old, ok := state[k]; ok && old.Present() {
				present--
			}
			state[k] = e
			if e.Present() {
				present++
			}
		}
		total := 0
		for md5, s := range Summaries(state, 2) {
			for _, ds := range []*DiskSummary{s.Primary, s.Backup} {
				if ds != nil {
					total += ds.Copies
				}
			}
			safe := false
			for _, e := range state {
				if e.MD5 != md5 || !e.Present() {
					continue
				}
				for _, o := range state {
					if o.MD5 == md5 && o.Present() && o.Disk != e.Disk {
						safe = true
					}
				}
			}
			c := s.Catalog()
			if (c.PrimaryCopies > 0 && c.BackupCopies > 0) != safe {
				t.Fatalf("%s: summary %+v disagrees with present copies", md5, c)
			}
		}
		if total != present {
			t.Fatalf("summaries count %d copies, state has %d present", total, present)
		}
	})
}

func TestUpdates(t *testing.T) {
	a := map[string]Summary{"m1": {Size: 1, Primary: &DiskSummary{FirstVerifiedAt: t0, Copies: 1}}}
	same := map[string]Summary{"m1": {Size: 1, Primary: &DiskSummary{FirstVerifiedAt: t0, Copies: 1}}}
	if got := Updates(a, same); len(got) != 0 {
		t.Errorf("unchanged summaries republished: %v", got)
	}
	more := map[string]Summary{"m1": {Size: 1, Primary: &DiskSummary{FirstVerifiedAt: t0, Copies: 2}}, "m2": {Size: 3}}
	if got := Updates(a, more); len(got) != 2 {
		t.Errorf("updates = %v, want m1 and m2", got)
	}
	// m1 gone from the state: written once with no copies, then left alone.
	got := Updates(a, map[string]Summary{})
	if g, ok := got["m1"]; !ok || g.Primary.Copies != 0 || !g.Primary.FirstVerifiedAt.Equal(t0) {
		t.Fatalf("gone content: %+v", got)
	}
	if again := Updates(got, map[string]Summary{}); len(again) != 0 {
		t.Errorf("gone content republished every run: %v", again)
	}
}

// An original edited in place on primary: the old content must stop
// counting a primary copy in the published catalog, or a card holding it
// could be reported safe to format.
func TestEditedOriginalStopsBeingSafe(t *testing.T) {
	p := "2026-10-18 Coast/Originals/ILCE-7M4-01/DCIM/100MSDCF/DSC00001.JPG"
	state := map[Key]Entry{
		KeyOf(Primary, p): {Disk: Primary, Path: p, Size: 10, MtimeNs: 1, MD5: "old", VerifiedAt: t0},
		KeyOf(Backup, p):  {Disk: Backup, Path: p, Size: 10, MtimeNs: 1, MD5: "old", VerifiedAt: t0},
	}
	published := Summaries(state, 10)
	if published["old"].Catalog() != (catalog.Summary{PrimaryCopies: 1, BackupCopies: 1}) {
		t.Fatalf("setup: %+v", published["old"].Catalog())
	}
	prev := state[KeyOf(Primary, p)]
	next, ev := Hashed(&prev, Seen{Primary, p, 12, 2}, "new", t0.Add(time.Hour), shoots)
	if ev == nil || ev.Kind != Conflict {
		t.Fatalf("want a conflict, got %+v", ev)
	}
	state[KeyOf(Primary, p)] = next

	updates := Updates(published, Summaries(state, 10))
	old, ok := updates["old"]
	if !ok {
		t.Fatal("the old content's summary isn't republished, so the catalog still shows its primary copy")
	}
	if c := old.Catalog(); c.PrimaryCopies != 0 || c.BackupCopies != 1 {
		t.Fatalf("old content now %+v, want 0 primary and 1 backup copies", c)
	}
	if catalog.StateOf(10, &catalog.Summary{PrimaryCopies: old.Catalog().PrimaryCopies, BackupCopies: old.Catalog().BackupCopies}) == catalog.Safe {
		t.Fatal("old content still Safe")
	}
	if _, ok := updates["new"]; !ok {
		t.Fatal("the new content's summary isn't published")
	}
	// Publishing again changes nothing.
	for md5, s := range updates {
		published[md5] = s
	}
	if again := Updates(published, Summaries(state, 10)); len(again) != 0 {
		t.Fatalf("second run republishes %v", again)
	}
}

// Across random runs (new content, replacements, edits, missing files),
// with each run's Updates applied to the published catalog, every hash the
// catalog has ever held shows exactly the copies present on disk: never a
// copy that's gone, which could make content look Safe.
func TestPublishedCatalogMatchesDisk(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		state := map[Key]Entry{}
		published := map[string]Summary{}
		now := t0
		runs := rapid.IntRange(1, 8).Draw(t, "runs")
		for r := 0; r < runs; r++ {
			now = now.Add(time.Hour)
			ops := rapid.IntRange(1, 6).Draw(t, "ops")
			for i := 0; i < ops; i++ {
				d := rapid.SampledFrom([]Disk{Primary, Backup}).Draw(t, "disk")
				p := rapid.SampledFrom([]string{
					"2026-10-18 Coast/Originals/a.arw", "2026-10-18 Coast/Derived/b.tif", "2019 Beach/c.jpg",
				}).Draw(t, "path")
				k := KeyOf(d, p)
				if rapid.IntRange(0, 4).Draw(t, "op") == 0 {
					if e, ok := state[k]; ok && e.Present() {
						at := now
						e.MissingSince = &at
						state[k] = e
					}
					continue
				}
				md5 := rapid.SampledFrom([]string{"m1", "m2", "m3"}).Draw(t, "md5")
				var prev *Entry
				if e, ok := state[k]; ok {
					prev = &e
				}
				next, _ := Hashed(prev, Seen{d, p, 10, int64(r)}, md5, now, shoots)
				state[k] = next
			}
			for md5, s := range Updates(published, Summaries(state, 10)) {
				published[md5] = s
			}
		}
		for md5, s := range published {
			want := map[Disk]int{}
			for _, e := range state {
				if e.MD5 == md5 && e.Present() {
					want[e.Disk]++
				}
			}
			if c := s.Catalog(); c.PrimaryCopies != want[Primary] || c.BackupCopies != want[Backup] {
				t.Fatalf("%s published %+v, disk has %v", md5, c, want)
			}
		}
	})
}
