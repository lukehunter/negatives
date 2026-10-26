//go:build windows && robocopy

// Tests against the real robocopy.exe. Run on Windows with:
//
//	go test -tags robocopy -v ./internal/archivesync -run Robocopy
//
// NEGATIVES_BIG_MB sets the size of the interrupted-copy test file
// (default 2048). See docs/tools/archive-sync-manual-tests.md.
package archivesync

import (
	"context"
	"crypto/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// What /XC /XN /XO actually do: a file already on the backup is never
// overwritten, whatever its size or time; new files are copied; files only
// on the backup stay; Derived folders are replaced by pass 2.
func TestRobocopyAdditiveFlags(t *testing.T) {
	d := newDisks(t)
	put(t, d.primary, "Legacy/new.txt", "new", 0)
	put(t, d.primary, "Legacy/same.txt", "same", 0)
	put(t, d.backup, "Legacy/same.txt", "same", 0)
	put(t, d.primary, "Legacy/changed.txt", "primary longer", 0) // same time, other size: "Changed"
	put(t, d.backup, "Legacy/changed.txt", "backup", 0)
	put(t, d.primary, "Legacy/newer.txt", "primary newer", 5) // "Newer"
	put(t, d.backup, "Legacy/newer.txt", "backup older", 0)
	put(t, d.primary, "Legacy/older.txt", "primary older", 0) // "Older"
	put(t, d.backup, "Legacy/older.txt", "backup newer", 5)
	put(t, d.primary, "Legacy/lookalike.txt", "AAAA", 0) // same size and time, other content
	put(t, d.backup, "Legacy/lookalike.txt", "BBBB", 0)
	put(t, d.backup, "Legacy/extra.txt", "only on backup", 0)
	put(t, d.primary, "Legacy/x.MP4.partial", "in progress", 0)
	put(t, d.primary, coast+"/.negatives", "", 0)
	put(t, d.primary, coast+"/Originals/c/a.ARW", "raw", 0)
	put(t, d.primary, coast+"/Derived/c/a.xmp", "rating 5", 5)
	put(t, d.backup, coast+"/Derived/c/a.xmp", "rating 3", 0)
	put(t, d.primary, "root.txt", "root", 0)
	primaryBefore := snapshot(t, d.primary)

	// Report only first: nothing may change.
	backupBefore := snapshot(t, d.backup)
	res, err := Run(context.Background(), d.options(ExecRunner{}, NoFacts{}, true))
	if err != nil || !res.OK() {
		t.Fatalf("report run: %v %s", err, Summary(res))
	}
	sameSnapshot(t, "backup after /L", backupBefore, snapshot(t, d.backup))

	res, err = Run(context.Background(), d.options(ExecRunner{}, NoFacts{}, false))
	if err != nil || !res.OK() {
		t.Fatalf("run: %v %s %v", err, Summary(res), res.Errors)
	}
	for _, j := range res.Jobs {
		t.Logf("pass %d %s: %s", j.Job.Pass, j.Job.Src, j.Outcome)
	}
	sameSnapshot(t, "primary", primaryBefore, snapshot(t, d.primary))
	want := map[string]string{
		"Legacy/new.txt":             "new",
		"Legacy/same.txt":            "same",
		"Legacy/changed.txt":         "backup",
		"Legacy/newer.txt":           "backup older",
		"Legacy/older.txt":           "backup newer",
		"Legacy/lookalike.txt":       "BBBB",
		"Legacy/extra.txt":           "only on backup",
		coast + "/.negatives":        "",
		coast + "/Originals/c/a.ARW": "raw",
		coast + "/Derived/c/a.xmp":   "rating 5",
		"root.txt":                   "root",
	}
	for rel, content := range want {
		if got := read(t, d.backup, rel); got != content {
			t.Errorf("%s on backup is %q, want %q", rel, got, content)
		}
	}
	if exists(d.backup, "Legacy/x.MP4.partial") {
		t.Error(".partial copied")
	}
	if st, err := os.Stat(d.backup); err == nil {
		t.Logf("backup root mode after the root files job: %v", st.Mode())
	}
}

// What an interrupted robocopy leaves behind, and that reconcile recovers
// from it: kept while unverified, quarantined and recopied once verified.
func TestRobocopyInterruptedCopy(t *testing.T) {
	mb := 2048
	if s := os.Getenv("NEGATIVES_BIG_MB"); s != "" {
		mb, _ = strconv.Atoi(s)
	}
	d := newDisks(t)
	clip := coast + "/Originals/a7rv-01/PRIVATE/M4ROOT/CLIP/C0001.MP4"
	put(t, d.primary, coast+"/.negatives", "", 0)
	src := filepath.Join(d.primary, filepath.FromSlash(clip))
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1<<20)
	rand.Read(buf)
	for i := 0; i < mb; i++ {
		buf[0] = byte(i) // not all blocks alike
		if _, err := f.Write(buf); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	mt := baseTime
	os.Chtimes(src, mt, mt)
	pinfo, _ := Stat(src)

	// Run 1, killed as soon as the clip appears on the backup.
	dst := filepath.Join(d.backup, filepath.FromSlash(clip))
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for ctx.Err() == nil {
			if st, err := os.Stat(dst); err == nil && st.Size() > 0 {
				time.Sleep(50 * time.Millisecond)
				cancel()
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	res, err := Run(ctx, d.options(ExecRunner{}, NoFacts{}, false))
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if res.OK() {
		t.Skipf("robocopy finished before it was killed; set NEGATIVES_BIG_MB higher than %d", mb)
	}
	partial, err := Stat(dst)
	if err != nil {
		t.Fatalf("no partial file left: %v (then /XC /XN /XO is no problem at all)", err)
	}
	t.Logf("OBSERVED: primary %d bytes, mtime %s", pinfo.Size, stamp(pinfo.MtimeNs))
	t.Logf("OBSERVED: partial %d bytes, mtime %s", partial.Size, stamp(partial.MtimeNs))
	if partial.Size == pinfo.Size {
		t.Fatalf("robocopy left a partial file with the full size (preallocated). Reconcile's size rule " +
			"can't tell it from a complete copy: the design must change before archive-sync is used. " +
			"Record the mtime above in the manual test notes.")
	}

	// Run 2: no facts, so it's kept.
	res, err = Run(context.Background(), d.options(ExecRunner{}, NoFacts{}, false))
	if err != nil || !res.OK() || res.Counts[HoldUnverified] != 1 || len(res.Moved) != 0 {
		t.Fatalf("run 2: %v %s", err, Summary(res))
	}
	if again, _ := Stat(dst); again != partial {
		t.Fatalf("partial changed by an additive run: %+v -> %+v (robocopy recopied it on its own)", partial, again)
	}

	// Run 3: verified, so quarantined and recopied.
	res, err = Run(context.Background(), d.options(ExecRunner{}, &hasherFacts{primary: d.primary}, false))
	if err != nil || !res.OK() || len(res.Moved) != 1 {
		t.Fatalf("run 3: %v %s %v", err, Summary(res), res.Errors)
	}
	want, _ := md5File(src)
	got, _ := md5File(dst)
	if got != want {
		t.Fatalf("backup clip md5 %s, primary %s", got, want)
	}
	if q, err := Stat(res.Moved[0].To); err != nil || q != partial {
		t.Fatalf("quarantined file %+v, %v; want the partial %+v", q, err, partial)
	}
}
