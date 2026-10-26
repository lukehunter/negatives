package archivesync

import (
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func verified(p FileInfo) Facts {
	return Facts{Known: true, Verified: true, Size: p.Size, MtimeNs: p.MtimeNs, MD5: "d41d8cd98f00b204e9800998ecf8427e", CardMatch: true}
}

func TestDecide(t *testing.T) {
	prim := FileInfo{Size: 1000, MtimeNs: 5e18}
	small := &FileInfo{Size: 400, MtimeNs: 6e18}
	pair := func(k Kind, b *FileInfo) Pair { return Pair{Rel: "s/x", Kind: k, Primary: prim, Backup: b} }
	with := func(mod func(*Facts)) Facts { f := verified(prim); mod(&f); return f }
	cases := []struct {
		name  string
		pair  Pair
		facts Facts
		want  Verdict
	}{
		{"same", pair(KindLegacy, &FileInfo{1000, 5e18}), Facts{}, Same},
		{"missing", pair(KindOriginal, nil), Facts{}, Missing},
		{"derived differs", pair(KindDerived, small), verified(prim), Replaceable},
		{"folder on backup", Pair{Rel: "s/x", Primary: prim, BackupIsDir: true}, verified(prim), TypeMismatch},
		{"same size, other time", pair(KindOriginal, &FileInfo{1000, 7e18}), verified(prim), TimeDiffers},
		{"backup larger", pair(KindLegacy, &FileInfo{2000, 5e18}), verified(prim), BackupLarger},
		{"smaller, no facts", pair(KindOriginal, small), Facts{}, HoldUnverified},
		{"smaller, known but unverified", pair(KindOriginal, small), with(func(f *Facts) { f.Verified = false }), HoldUnverified},
		{"smaller, facts for an older size", pair(KindLegacy, small), with(func(f *Facts) { f.Size = 999 }), HoldUnverified},
		{"smaller, facts for an older time", pair(KindLegacy, small), with(func(f *Facts) { f.MtimeNs++ }), HoldUnverified},
		{"smaller, primary edited in place", pair(KindLegacy, small), with(func(f *Facts) { f.PathChanged = true }), HoldPathChanged},
		{"smaller, backup is known content", pair(KindLegacy, small), with(func(f *Facts) { f.BackupContentKnown = true }), HoldBackupKnown},
		{"original not matching a card", pair(KindOriginal, small), with(func(f *Facts) { f.CardMatch = false }), HoldNoCardMatch},
		{"legacy needs no card match", pair(KindLegacy, small), with(func(f *Facts) { f.CardMatch = false }), Quarantine},
		{"shoot file needs no card match", pair(KindShootOther, small), with(func(f *Facts) { f.CardMatch = false }), Quarantine},
		{"original partial", pair(KindOriginal, small), verified(prim), Quarantine},
		{"empty partial", pair(KindOriginal, &FileInfo{0, 0}), verified(prim), Quarantine},
	}
	for _, c := range cases {
		if got := Decide(c.pair, c.facts); got != c.want {
			t.Errorf("%s: Decide = %v, want %v", c.name, got, c.want)
		}
	}
}

func genPair(t *rapid.T) (Pair, Facts) {
	kind := rapid.SampledFrom([]Kind{KindLegacy, KindOriginal, KindDerived, KindShootOther}).Draw(t, "kind")
	p := FileInfo{Size: rapid.Int64Range(0, 50).Draw(t, "psize"), MtimeNs: rapid.Int64Range(0, 3).Draw(t, "pmtime")}
	pair := Pair{Rel: "s/f", Kind: kind, Primary: p}
	switch rapid.IntRange(0, 2).Draw(t, "backup") {
	case 1:
		b := FileInfo{Size: rapid.Int64Range(0, 50).Draw(t, "bsize"), MtimeNs: rapid.Int64Range(0, 3).Draw(t, "bmtime")}
		pair.Backup = &b
	case 2:
		pair.BackupIsDir = true
	}
	f := Facts{
		Known:              rapid.Bool().Draw(t, "known"),
		Verified:           rapid.Bool().Draw(t, "verified"),
		Size:               rapid.SampledFrom([]int64{p.Size, p.Size + 1}).Draw(t, "fsize"),
		MtimeNs:            rapid.SampledFrom([]int64{p.MtimeNs, p.MtimeNs - 1}).Draw(t, "fmtime"),
		CardMatch:          rapid.Bool().Draw(t, "card"),
		PathChanged:        rapid.Bool().Draw(t, "changed"),
		BackupContentKnown: rapid.Bool().Draw(t, "backupKnown"),
	}
	return pair, f
}

// Property: Decide quarantines exactly when every condition in its doc
// comment holds, and facts never matter unless NeedsFacts says so.
func TestDecideQuarantineExactlyWhenSafe(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pair, f := genPair(t)
		v := Decide(pair, f)
		safe := pair.Kind != KindDerived && !pair.BackupIsDir && pair.Backup != nil &&
			pair.Backup.Size < pair.Primary.Size &&
			f.Known && f.Verified && f.Size == pair.Primary.Size && f.MtimeNs == pair.Primary.MtimeNs &&
			!f.PathChanged && !f.BackupContentKnown &&
			(pair.Kind != KindOriginal || f.CardMatch)
		if (v == Quarantine) != safe {
			t.Fatalf("Decide(%+v, %+v) = %v, conditions hold: %v", pair, f, v, safe)
		}
		if !NeedsFacts(pair) && Decide(pair, Facts{}) != v {
			t.Fatalf("facts changed the verdict although NeedsFacts is false")
		}
		if v == Quarantine && !NeedsFacts(pair) {
			t.Fatal("quarantine without facts being needed")
		}
	})
}

// Property: with no facts (build step 2, before archive-hasher), nothing is
// ever quarantined.
func TestNoFactsNeverQuarantines(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		pair, _ := genPair(t)
		if v := Decide(pair, Facts{}); v == Quarantine {
			t.Fatalf("quarantined %+v with no facts", pair)
		}
	})
}

func TestQuarantinePath(t *testing.T) {
	_, _, q := roots()
	got, err := QuarantinePath(q, "20261025T163000Z", "2026-10-18 Coast/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4")
	want := filepath.Join(q, "20261025T163000Z", "2026-10-18 Coast", "Originals", "a7rv-01", "PRIVATE", "M4ROOT", "CLIP", "C0001.MP4")
	if err != nil || got != want {
		t.Fatalf("QuarantinePath = %q, %v; want %q", got, err, want)
	}
	for _, c := range []struct{ run, rel string }{
		{"r", "../x"}, {"r", "a/../../x"}, {"r", "/abs"}, {"r", ""}, {"..", "x"}, {"a/b", "x"}, {"", "x"},
	} {
		if p, err := QuarantinePath(q, c.run, c.rel); err == nil {
			t.Errorf("QuarantinePath(%q, %q) = %q, want an error", c.run, c.rel, p)
		}
	}
}

// Property: any accepted quarantine path is inside the run's quarantine
// folder.
func TestQuarantinePathStaysInside(t *testing.T) {
	_, _, q := roots()
	rapid.Check(t, func(t *rapid.T) {
		segs := rapid.SliceOfN(rapid.SampledFrom([]string{"a", "..", ".", "", "B c", "x.ARW"}), 1, 5).Draw(t, "segs")
		rel := strings.Join(segs, "/")
		p, err := QuarantinePath(q, "run", rel)
		if err != nil {
			return
		}
		inside := filepath.Join(q, "run")
		if !Within(inside, p) || p == inside {
			t.Fatalf("QuarantinePath(%q) = %q, outside %q", rel, p, inside)
		}
	})
}
