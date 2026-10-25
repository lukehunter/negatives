package archivesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options configures one run.
type Options struct {
	Primary, Backup string // disk roots
	Quarantine      string // folder directly at the backup root
	LogDir          string
	// Report: compare and list only. Nothing on either disk is written,
	// moved or copied.
	Report bool
	Facts  FactSource
	Runner Runner
	Now    func() time.Time
	// Out gets the same lines as the log file, for a person watching.
	Out io.Writer
}

// Moved is one backup file set aside in quarantine.
type Moved struct {
	Rel     string
	To      string
	Backup  FileInfo
	Primary FileInfo
	MD5     string
}

// JobResult is one robocopy run.
type JobResult struct {
	Job     Job
	Outcome Outcome
	Err     error
}

// Result is what a run did.
type Result struct {
	RunID      string
	Same       int
	Found      []Found
	Counts     map[Verdict]int
	Moved      []Moved
	Jobs       []JobResult
	Errors     []string
	ReportPath string
}

// OK reports whether the run succeeded: every robocopy job exited 0 to 7,
// and nothing failed to read or move. Partial copies held for verification
// and differences listed for a person don't fail a run.
func (r *Result) OK() bool {
	if len(r.Errors) > 0 {
		return false
	}
	for _, j := range r.Jobs {
		if j.Err != nil || !j.Outcome.OK {
			return false
		}
	}
	return true
}

// Run does one archive-sync run:
//
//  1. reconcile: compare the disks, and move partial copies on the backup
//     disk to quarantine when Decide allows it;
//  2. pass 1: copy everything new, additive only;
//  3. pass 2: copy negatives shoots' Derived folders, overwrites allowed.
//
// Reconcile goes first, so a partial copy left by an interrupted earlier
// run is recopied by this run's pass 1.
//
// Nothing here writes to the primary disk: it's only listed, and robocopy
// only reads from it. In report mode nothing writes to the backup disk
// either.
func Run(ctx context.Context, o Options) (*Result, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Facts == nil {
		o.Facts = NoFacts{}
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	now := o.Now()
	res := &Result{RunID: now.UTC().Format("20060102T150405Z"), Counts: map[Verdict]int{}}
	day := now.Format("2006-01-02")

	if err := os.MkdirAll(o.LogDir, 0o755); err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(filepath.Join(o.LogDir, "archive-sync-"+day+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	defer lf.Close()
	out := io.MultiWriter(lf, o.Out)
	logf := func(format string, a ...any) {
		fmt.Fprintf(out, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), fmt.Sprintf(format, a...))
	}
	fail := func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		res.Errors = append(res.Errors, msg)
		logf("ERROR %s", msg)
	}
	mode := "sync"
	if o.Report {
		mode = "report only (nothing is copied or moved)"
	}
	logf("archive-sync %s: %s -> %s, %s", res.RunID, o.Primary, o.Backup, mode)

	qname := filepath.Base(o.Quarantine)
	if _, err := os.Lstat(filepath.Join(o.Primary, qname)); err == nil {
		return nil, fmt.Errorf("the primary disk has %s, the quarantine folder's name: rename one of them so it can't be copied into quarantine",
			filepath.Join(o.Primary, qname))
	}
	entries, err := ListRoot(o.Primary)
	if err != nil {
		return nil, fmt.Errorf("listing the primary disk: %w", err)
	}
	shoots := ShootsOf(entries)
	logf("primary: %d top-level entries, %d negatives shoots", len(entries), len(shoots))

	// 1. Reconcile.
	lookup := func(rel string, p FileInfo) (Facts, error) { return o.Facts.Lookup(ctx, rel, p) }
	same, err := Walk(WalkInput{Primary: o.Primary, Backup: o.Backup, Shoots: shoots, QuarantineName: qname, Lookup: lookup},
		func(f Found) {
			res.Found = append(res.Found, f)
			res.Counts[f.Verdict]++
			if f.Err != nil {
				fail("%s: %v", f.Rel, f.Err)
			}
		})
	res.Same = same
	if err != nil {
		fail("comparing the disks: %v", err)
	}
	for _, f := range res.Found {
		if f.Verdict == Quarantine || f.Verdict.NeedsAttention() || f.Verdict == HoldUnverified {
			logf("%s %s: primary %s, backup %s", f.Verdict, f.Rel, describe(&f.Primary), describe(f.Backup))
		}
		if f.Verdict != Quarantine || o.Report {
			continue
		}
		m, err := quarantine(o, res.RunID, f, lookup)
		if err != nil {
			fail("%s: left in place: %v", f.Rel, err)
			continue
		}
		res.Moved = append(res.Moved, m)
		logf("quarantined %s to %s", f.Rel, m.To)
	}
	if n := res.Counts[HoldUnverified]; n > 0 {
		logf("%d smaller backup copies kept in place until archive-hasher verifies their primary copies", n)
	}

	// 2 and 3. robocopy.
	jobs := Plan(PlanInput{
		Primary: o.Primary, Backup: o.Backup, Entries: entries, QuarantineName: qname,
		Log: filepath.Join(o.LogDir, "robocopy-"+day+".log"), Report: o.Report,
	})
	for _, j := range jobs {
		if ctx.Err() != nil {
			fail("stopped before pass %d %q: %v", j.Pass, j.Rel, ctx.Err())
			break
		}
		if err := CheckJob(j, o.Primary, o.Backup, o.Quarantine); err != nil {
			fail("refused: %v", err)
			continue
		}
		code, err := o.Runner.Robocopy(ctx, j.Args)
		r := JobResult{Job: j, Outcome: Interpret(code), Err: err}
		res.Jobs = append(res.Jobs, r)
		switch {
		case err != nil:
			logf("ERROR pass %d %s: %v", j.Pass, j.Src, err)
		case !r.Outcome.OK:
			logf("ERROR pass %d %s: %s", j.Pass, j.Src, r.Outcome)
		case r.Outcome.Copied || r.Outcome.Mismatch:
			logf("pass %d %s: %s", j.Pass, j.Src, r.Outcome)
		}
	}

	if o.Report {
		res.ReportPath = filepath.Join(o.LogDir, "archive-sync-report-"+res.RunID+".tsv")
		if err := writeReport(res.ReportPath, res.Found); err != nil {
			fail("writing the report: %v", err)
		}
	}
	logf("%s", Summary(res))
	return res, nil
}

// quarantine moves one backup file into the quarantine folder, after
// checking again that nothing changed since the walk and that Decide still
// says so.
func quarantine(o Options, runID string, f Found, lookup func(string, FileInfo) (Facts, error)) (Moved, error) {
	src := filepath.Join(o.Backup, filepath.FromSlash(f.Rel))
	pi, err := Stat(filepath.Join(o.Primary, filepath.FromSlash(f.Rel)))
	if err != nil {
		return Moved{}, err
	}
	bi, err := Stat(src)
	if err != nil {
		return Moved{}, err
	}
	if pi != f.Primary || f.Backup == nil || bi != *f.Backup {
		return Moved{}, errors.New("changed during the run")
	}
	facts, err := lookup(f.Rel, pi)
	if err != nil {
		return Moved{}, err
	}
	if v := Decide(Pair{Rel: f.Rel, Kind: f.Kind, Primary: pi, Backup: &bi}, facts); v != Quarantine {
		return Moved{}, fmt.Errorf("now %s", v)
	}
	dst, err := QuarantinePath(o.Quarantine, runID, f.Rel)
	if err != nil {
		return Moved{}, err
	}
	if !Within(o.Quarantine, dst) || !Within(o.Backup, src) {
		return Moved{}, errors.New("move would leave the backup disk")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return Moved{}, err
	}
	if err := moveNoReplace(src, dst); err != nil {
		return Moved{}, err
	}
	m := Moved{Rel: f.Rel, To: dst, Backup: bi, Primary: pi, MD5: facts.MD5}
	// A record next to the moved files, so quarantine can be reviewed
	// without the logs. The file is already safe in quarantine if this
	// fails.
	if err := appendManifest(filepath.Join(o.Quarantine, runID, "MANIFEST.tsv"), m); err != nil {
		return m, fmt.Errorf("moved to %s, but writing the manifest failed: %w", dst, err)
	}
	return m, nil
}

func appendManifest(path string, m Moved) error {
	_, statErr := os.Stat(path)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if os.IsNotExist(statErr) {
		fmt.Fprintln(f, "path\tbackup_size\tbackup_mtime\tprimary_size\tprimary_mtime\tprimary_md5\tmoved_at")
	}
	fmt.Fprintf(f, "%s\t%d\t%s\t%d\t%s\t%s\t%s\n", m.Rel, m.Backup.Size, stamp(m.Backup.MtimeNs),
		m.Primary.Size, stamp(m.Primary.MtimeNs), m.MD5, time.Now().UTC().Format(time.RFC3339))
	return f.Close()
}

func writeReport(path string, found []Found) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	fmt.Fprintln(f, "verdict\tkind\tpath\tprimary_size\tprimary_mtime\tbackup_size\tbackup_mtime\terror")
	for _, x := range found {
		ps, pm, bs, bm := "", "", "", ""
		if x.Verdict != Extra && x.Verdict != MissingDir && x.Verdict != NotRegular && x.Primary != (FileInfo{}) {
			ps, pm = fmt.Sprint(x.Primary.Size), stamp(x.Primary.MtimeNs)
		}
		if x.Backup != nil {
			bs, bm = fmt.Sprint(x.Backup.Size), stamp(x.Backup.MtimeNs)
		}
		e := ""
		if x.Err != nil {
			e = x.Err.Error()
		}
		fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", x.Verdict, x.Kind, x.Rel, ps, pm, bs, bm, e)
	}
	return f.Close()
}

// Summary is the run's closing log line.
func Summary(r *Result) string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%d files same on both disks", r.Same))
	vs := make([]Verdict, 0, len(r.Counts))
	for v := range r.Counts {
		vs = append(vs, v)
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i] < vs[j] })
	for _, v := range vs {
		parts = append(parts, fmt.Sprintf("%d %s", r.Counts[v], v))
	}
	parts = append(parts, fmt.Sprintf("%d quarantined", len(r.Moved)))
	failed := 0
	for _, j := range r.Jobs {
		if j.Err != nil || !j.Outcome.OK {
			failed++
		}
	}
	parts = append(parts, fmt.Sprintf("%d robocopy jobs, %d failed", len(r.Jobs), failed))
	if r.ReportPath != "" {
		parts = append(parts, "report: "+r.ReportPath)
	}
	status := "done"
	if !r.OK() {
		status = fmt.Sprintf("FAILED (%d errors)", len(r.Errors)+failed)
	}
	return status + ": " + strings.Join(parts, ", ")
}

func describe(fi *FileInfo) string {
	if fi == nil {
		return "none"
	}
	return fmt.Sprintf("%d bytes, %s", fi.Size, stamp(fi.MtimeNs))
}

func stamp(ns int64) string { return time.Unix(0, ns).UTC().Format(time.RFC3339Nano) }
