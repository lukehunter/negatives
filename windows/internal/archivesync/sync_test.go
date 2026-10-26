package archivesync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lukehunter/negatives/windows/internal/diskid"
)

type disks struct {
	primary, backup, quarantine, logs string
	primaryID, backupID               string
}

func newDisks(t *testing.T) disks {
	dir := t.TempDir()
	d := disks{
		primary: filepath.Join(dir, "P"), backup: filepath.Join(dir, "B"), logs: filepath.Join(dir, "logs"),
	}
	d.quarantine = filepath.Join(d.backup, "_quarantine")
	for _, p := range []string{d.primary, d.backup} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pid, err := diskid.Create(d.primary, diskid.Primary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bid, err := diskid.Create(d.backup, diskid.Backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	d.primaryID, d.backupID = pid.ID, bid.ID
	return d
}

func (d disks) options(r Runner, f FactSource, report bool) Options {
	clock := time.Date(2026, 10, 25, 3, 0, 0, 0, time.UTC)
	return Options{
		Primary: d.primary, Backup: d.backup, Quarantine: d.quarantine, LogDir: d.logs,
		PrimaryID: d.primaryID, BackupID: d.backupID,
		Report: report, Facts: f, Runner: r,
		Now: func() time.Time { clock = clock.Add(time.Second); return clock },
	}
}

const coast = "2026-10-18 Coast"

// A primary disk with one negatives shoot, one legacy shoot, a file at the
// root, an in-progress push and the recycle bin.
func fillPrimary(t *testing.T, d disks) {
	put(t, d.primary, coast+"/.negatives", "", 0)
	put(t, d.primary, coast+"/Originals/a7rv-01/DCIM/100MSDCF/DSC09304.ARW", "raw 9304", 0)
	put(t, d.primary, coast+"/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4", strings.Repeat("v", 4096), 0)
	put(t, d.primary, coast+"/Derived/a7rv-01/DCIM/100MSDCF/DSC09304.xmp", "rating 5", 3)
	put(t, d.primary, coast+"/Derived/Exports/pano.tif.partial", "half a pano", 3)
	put(t, d.primary, "2019-05-04 Beach/IMG_0001.JPG", "beach 1", -5000)
	put(t, d.primary, "2019-05-04 Beach/IMG_0002.JPG", "beach 2", -5000)
	put(t, d.primary, "notes.txt", "root file", -10)
	put(t, d.primary, "$RECYCLE.BIN/S-1-5/$R123.JPG", "deleted", 0)
}

func TestRunCopiesAdditively(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	// Already on the backup: an older version of a legacy file with the
	// same size (never overwritten), a file only on the backup (kept), and
	// an older Derived sidecar (replaced by pass 2).
	put(t, d.backup, "2019-05-04 Beach/IMG_0002.JPG", "BEACH 2", -6000)
	put(t, d.backup, "2019-05-04 Beach/only-on-backup.txt", "extra", 0)
	put(t, d.backup, coast+"/Derived/a7rv-01/DCIM/100MSDCF/DSC09304.xmp", "rating 3", 1)
	primaryBefore := snapshot(t, d.primary)

	r := &fakeRobocopy{}
	res, err := Run(context.Background(), d.options(r, NoFacts{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() {
		t.Fatalf("run failed: %v %s", res.Errors, Summary(res))
	}
	sameSnapshot(t, "primary", primaryBefore, snapshot(t, d.primary))

	for _, rel := range []string{
		coast + "/.negatives",
		coast + "/Originals/a7rv-01/DCIM/100MSDCF/DSC09304.ARW",
		coast + "/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4",
		"2019-05-04 Beach/IMG_0001.JPG",
		"notes.txt",
	} {
		if read(t, d.backup, rel) != read(t, d.primary, rel) {
			t.Errorf("%s not copied", rel)
		}
	}
	if got := read(t, d.backup, "2019-05-04 Beach/IMG_0002.JPG"); got != "BEACH 2" {
		t.Errorf("existing backup file overwritten: %q", got)
	}
	if !exists(d.backup, "2019-05-04 Beach/only-on-backup.txt") {
		t.Error("backup-only file removed")
	}
	if got := read(t, d.backup, coast+"/Derived/a7rv-01/DCIM/100MSDCF/DSC09304.xmp"); got != "rating 5" {
		t.Errorf("Derived file not replaced: %q", got)
	}
	if exists(d.backup, coast+"/Derived/Exports/pano.tif.partial") {
		t.Error(".partial copied")
	}
	if exists(d.backup, "$RECYCLE.BIN") {
		t.Error("recycle bin copied")
	}
	if exists(d.quarantine, "") {
		t.Error("quarantine created with nothing to quarantine")
	}
	if res.Counts[TimeDiffers] != 1 || res.Counts[Extra] != 1 {
		t.Errorf("counts %v", res.Counts)
	}
	// Pass 1 never got to overwrite: every pass 1 call was additive.
	for _, c := range r.calls {
		isPass2 := strings.HasSuffix(c[0], "Derived")
		if !isPass2 && !(slices.Contains(c, "/XC") && slices.Contains(c, "/XN") && slices.Contains(c, "/XO")) {
			t.Errorf("non-additive call %v", c)
		}
	}
}

func TestRunReportOnlyChangesNothing(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	put(t, d.backup, "2019-05-04 Beach/IMG_0002.JPG", "BEACH 2", -6000)
	put(t, d.backup, "2019-05-04 Beach/only-on-backup.txt", "extra", 0)
	put(t, d.backup, coast+"/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4", "vvv", 0) // partial
	primaryBefore, backupBefore := snapshot(t, d.primary), snapshot(t, d.backup)

	r := &fakeRobocopy{}
	facts := &hasherFacts{primary: d.primary}
	res, err := Run(context.Background(), d.options(r, facts, true))
	if err != nil {
		t.Fatal(err)
	}
	sameSnapshot(t, "primary", primaryBefore, snapshot(t, d.primary))
	sameSnapshot(t, "backup", backupBefore, snapshot(t, d.backup))
	if len(res.Moved) != 0 {
		t.Errorf("report mode moved %v", res.Moved)
	}
	if res.Counts[Quarantine] != 1 {
		t.Errorf("want the partial reported as quarantine-eligible, counts %v", res.Counts)
	}
	for _, c := range r.calls {
		if !slices.Contains(c, "/L") {
			t.Errorf("report mode ran robocopy without /L: %v", c)
		}
	}
	report, err := os.ReadFile(res.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"same-size-time-differs\tlegacy\t2019-05-04 Beach/IMG_0002.JPG",
		"only-on-backup\tlegacy\t2019-05-04 Beach/only-on-backup.txt",
		"quarantine\toriginal\t" + coast + "/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4\t4096",
		"\nmissing-on-backup\tlegacy\tnotes.txt",
	} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
}

// The design's failure case: robocopy killed mid-file leaves a partial copy
// that /XC /XN /XO would skip forever. It's kept until the primary copy is
// verified, then quarantined (not deleted) and recopied.
func TestRunInterruptedCopy(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	clip := coast + "/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4"
	primaryBefore := snapshot(t, d.primary)

	// Run 1: killed one byte into the clip.
	r := &fakeRobocopy{kill: "C0001.MP4", killAfter: 1}
	res, err := Run(context.Background(), d.options(r, NoFacts{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Fatal("a killed robocopy must fail the run")
	}
	if read(t, d.backup, clip) != "v" {
		t.Fatal("no partial clip on the backup")
	}

	// Run 2: no archive-hasher yet. The partial is reported and kept.
	res, err = Run(context.Background(), d.options(&fakeRobocopy{}, NoFacts{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || res.Counts[HoldUnverified] != 1 || len(res.Moved) != 0 {
		t.Fatalf("run 2: %s, errors %v, moved %v", Summary(res), res.Errors, res.Moved)
	}
	if read(t, d.backup, clip) != "v" {
		t.Fatal("partial changed without verification")
	}

	// Run 3: archive-hasher has verified the primary clip.
	facts := &hasherFacts{primary: d.primary}
	res, err = Run(context.Background(), d.options(&fakeRobocopy{}, facts, false))
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.Moved) != 1 {
		t.Fatalf("run 3: ok %v, errors %v, moved %v", res.OK(), res.Errors, res.Moved)
	}
	m := res.Moved[0]
	if m.Rel != clip || !Within(d.quarantine, m.To) {
		t.Fatalf("moved %+v", m)
	}
	if b, _ := os.ReadFile(m.To); string(b) != "v" {
		t.Errorf("quarantined copy is %q, want the partial", b)
	}
	if read(t, d.backup, clip) != read(t, d.primary, clip) {
		t.Error("clip not recopied after quarantine")
	}
	manifest, err := os.ReadFile(filepath.Join(d.quarantine, res.RunID, "MANIFEST.tsv"))
	if err != nil || !strings.Contains(string(manifest), clip+"\t1\t") || !strings.Contains(string(manifest), m.MD5) {
		t.Errorf("manifest %q, %v", manifest, err)
	}
	if !slices.Equal(facts.lookups, []string{clip, clip}) {
		t.Errorf("facts looked up for %v; want only the partial, once in the walk and once before moving", facts.lookups)
	}
	sameSnapshot(t, "primary", primaryBefore, snapshot(t, d.primary))

	// Run 4: nothing left to do.
	res, err = Run(context.Background(), d.options(&fakeRobocopy{}, facts, false))
	if err != nil || !res.OK() || len(res.Moved) != 0 || res.Counts[Quarantine] != 0 {
		t.Fatalf("run 4: %v %s", err, Summary(res))
	}
}

func TestRunKeepsBackupWhenFactsSayNo(t *testing.T) {
	clip := coast + "/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4"
	for name, c := range map[string]struct {
		modify func(string, *Facts)
		want   Verdict
	}{
		"primary edited in place": {func(_ string, f *Facts) { f.PathChanged = true }, HoldPathChanged},
		"backup content known":    {func(_ string, f *Facts) { f.BackupContentKnown = true }, HoldBackupKnown},
		"no card match":           {func(_ string, f *Facts) { f.CardMatch = false }, HoldNoCardMatch},
		"not verified":            {func(_ string, f *Facts) { f.Verified = false }, HoldUnverified},
	} {
		t.Run(name, func(t *testing.T) {
			d := newDisks(t)
			fillPrimary(t, d)
			put(t, d.backup, clip, "v", 0)
			res, err := Run(context.Background(), d.options(&fakeRobocopy{}, &hasherFacts{primary: d.primary, modify: c.modify}, false))
			if err != nil {
				t.Fatal(err)
			}
			if res.Counts[c.want] != 1 || len(res.Moved) != 0 || read(t, d.backup, clip) != "v" {
				t.Fatalf("counts %v, moved %v", res.Counts, res.Moved)
			}
		})
	}
}

func TestRunRefusesQuarantineNameOnPrimary(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	put(t, d.primary, "_quarantine/x", "x", 0)
	r := &fakeRobocopy{}
	if _, err := Run(context.Background(), d.options(r, NoFacts{}, false)); err == nil {
		t.Fatal("ran with a quarantine-named folder on primary")
	}
	if len(r.calls) != 0 {
		t.Fatal("robocopy ran")
	}
}

func TestQuarantineRechecksBeforeMoving(t *testing.T) {
	d := newDisks(t)
	rel := "Legacy/a.MP4"
	put(t, d.primary, rel, "full content", 0)
	put(t, d.backup, rel, "full", 0)
	pi, _ := Stat(filepath.Join(d.primary, rel))
	bi, _ := Stat(filepath.Join(d.backup, rel))
	facts := &hasherFacts{primary: d.primary}
	lookup := func(rel string, p FileInfo) (Facts, error) { return facts.Lookup(context.Background(), rel, p) }
	o := d.options(nil, facts, false)

	// The backup file grew since the walk (say robocopy is still writing).
	stale := Found{Pair: Pair{Rel: rel, Kind: KindLegacy, Primary: pi, Backup: &FileInfo{Size: 2, MtimeNs: bi.MtimeNs}}}
	if _, err := quarantine(o, "run1", stale, lookup); err == nil {
		t.Fatal("moved a file that changed since the walk")
	}
	// The primary was edited since the walk.
	put(t, d.primary, rel, "full content, edited", 1)
	f := Found{Pair: Pair{Rel: rel, Kind: KindLegacy, Primary: pi, Backup: &bi}}
	if _, err := quarantine(o, "run1", f, lookup); err == nil {
		t.Fatal("moved although the primary changed")
	}
	if read(t, d.backup, rel) != "full" {
		t.Fatal("backup file moved")
	}
}

func TestMoveNoReplace(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	put(t, dir, "a", "A", 0)
	put(t, dir, "b", "B", 0)
	if err := moveNoReplace(a, b); err == nil {
		t.Fatal("overwrote an existing file")
	}
	if read(t, dir, "a") != "A" || read(t, dir, "b") != "B" {
		t.Fatal("files changed")
	}
	c := filepath.Join(dir, "c")
	if err := moveNoReplace(a, c); err != nil || read(t, dir, "c") != "A" || exists(dir, "a") {
		t.Fatalf("move failed: %v", err)
	}
}

func TestLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "archive-sync.lock")
	l, err := AcquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AcquireLock(p); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatal(err)
	}
	l2, err := AcquireLock(p)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	l2.Release()
}

// A run stopped part way (Ctrl-C, or Task Scheduler ending it) fails and
// runs no further jobs.
func TestRunStopped(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &fakeRobocopy{}
	res, err := Run(ctx, d.options(r, NoFacts{}, false))
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() || len(r.calls) != 0 {
		t.Fatalf("ok %v, calls %d", res.OK(), len(r.calls))
	}
}

// Drive letters swapped, a disk replaced, or an identity file missing: the
// run stops before reading or writing anything, in both modes.
func TestRunRefusesWrongDisks(t *testing.T) {
	for name, mod := range map[string]func(*Options, disks){
		"swapped": func(o *Options, d disks) {
			o.Primary, o.Backup, o.Quarantine = d.backup, d.primary, filepath.Join(d.primary, "_quarantine")
		},
		"swapped IDs too": func(o *Options, d disks) {
			o.Primary, o.Backup, o.Quarantine = d.backup, d.primary, filepath.Join(d.primary, "_quarantine")
			o.PrimaryID, o.BackupID = d.backupID, d.primaryID
		},
		"replaced backup": func(o *Options, _ disks) { o.BackupID = "0f0e0d0c-0b0a-4908-8706-050403020100" },
		"no primary ID":   func(o *Options, _ disks) { o.PrimaryID = "" },
		"no identity file": func(_ *Options, d disks) {
			os.Remove(filepath.Join(d.primary, diskid.FileName))
		},
	} {
		for _, report := range []bool{false, true} {
			d := newDisks(t)
			fillPrimary(t, d)
			put(t, d.backup, coast+"/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4", "v", 0)
			o := d.options(&fakeRobocopy{}, &hasherFacts{primary: d.primary}, report)
			mod(&o, d)
			pBefore, bBefore := snapshot(t, d.primary), snapshot(t, d.backup)
			if _, err := Run(context.Background(), o); err == nil {
				t.Fatalf("%s, report %v: ran", name, report)
			}
			if r := o.Runner.(*fakeRobocopy); len(r.calls) != 0 {
				t.Fatalf("%s: robocopy ran", name)
			}
			sameSnapshot(t, name+": primary", pBefore, snapshot(t, d.primary))
			sameSnapshot(t, name+": backup", bBefore, snapshot(t, d.backup))
			log, _ := os.ReadFile(filepath.Join(d.logs, "archive-sync-2026-10-25.log"))
			if !strings.Contains(string(log), "ERROR") {
				t.Errorf("%s: refusal not logged: %q", name, log)
			}
		}
	}
}

// Each disk keeps its own identity file: it's never copied or compared.
func TestRunLeavesIdentityFilesAlone(t *testing.T) {
	d := newDisks(t)
	fillPrimary(t, d)
	res, err := Run(context.Background(), d.options(&fakeRobocopy{}, NoFacts{}, false))
	if err != nil || !res.OK() {
		t.Fatalf("%v %s", err, Summary(res))
	}
	if id, err := diskid.Read(d.backup); err != nil || id.Role != diskid.Backup || id.ID != d.backupID {
		t.Fatalf("backup identity now %+v, %v", id, err)
	}
	for _, f := range res.Found {
		if f.Rel == diskid.FileName {
			t.Fatalf("identity file compared: %+v", f)
		}
	}
}
