package archivesync

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"pgregory.net/rapid"
)

func roots() (primary, backup, quarantine string) {
	if runtime.GOOS == "windows" {
		return `P:\`, `B:\`, `B:\_quarantine`
	}
	return "/p", "/b", "/b/_quarantine"
}

func TestPlanExample(t *testing.T) {
	p, b, q := roots()
	jobs := Plan(PlanInput{
		Primary: p, Backup: b, QuarantineName: filepath.Base(q), Log: "/logs/r.log",
		Entries: []RootEntry{
			{Name: "2026-10-18 Coast", IsDir: true, Marked: true, DerivedName: "Derived"},
			{Name: "2019-05-04 Beach", IsDir: true},
			{Name: "desktop.ini"},
			{Name: "$RECYCLE.BIN", IsDir: true},
		},
	})
	coast := filepath.Join(p, "2026-10-18 Coast")
	want := []Job{
		{Pass: 1, Src: p, Dst: b, Args: []string{p, b, "/XC", "/XN", "/XO", "/DCOPY:T",
			"/R:2", "/W:5", "/XJ", "/NP", "/NDL", "/NJH", "/NJS", "/FP", "/XF", "*.partial", "/UNILOG+:/logs/r.log"}},
		{Pass: 1, Rel: "2019-05-04 Beach", Src: filepath.Join(p, "2019-05-04 Beach"), Dst: filepath.Join(b, "2019-05-04 Beach"),
			Args: []string{filepath.Join(p, "2019-05-04 Beach"), filepath.Join(b, "2019-05-04 Beach"), "/E", "/XC", "/XN", "/XO",
				"/R:2", "/W:5", "/XJ", "/NP", "/NDL", "/NJH", "/NJS", "/FP", "/XF", "*.partial", "/UNILOG+:/logs/r.log"}},
		{Pass: 1, Rel: "2026-10-18 Coast", Src: coast, Dst: filepath.Join(b, "2026-10-18 Coast"),
			Args: []string{coast, filepath.Join(b, "2026-10-18 Coast"), "/E", "/XC", "/XN", "/XO",
				"/R:2", "/W:5", "/XJ", "/NP", "/NDL", "/NJH", "/NJS", "/FP", "/XF", "*.partial", "/UNILOG+:/logs/r.log",
				"/XD", filepath.Join(coast, "Derived")}},
		{Pass: 2, Rel: "2026-10-18 Coast", Src: filepath.Join(coast, "Derived"), Dst: filepath.Join(b, "2026-10-18 Coast", "Derived"),
			Args: []string{filepath.Join(coast, "Derived"), filepath.Join(b, "2026-10-18 Coast", "Derived"), "/E",
				"/R:2", "/W:5", "/XJ", "/NP", "/NDL", "/NJH", "/NJS", "/FP", "/XF", "*.partial", "/UNILOG+:/logs/r.log"}},
	}
	if len(jobs) != len(want) {
		t.Fatalf("got %d jobs, want %d: %+v", len(jobs), len(want), jobs)
	}
	for i := range want {
		if jobs[i].Pass != want[i].Pass || jobs[i].Rel != want[i].Rel || jobs[i].Src != want[i].Src ||
			jobs[i].Dst != want[i].Dst || !slices.Equal(jobs[i].Args, want[i].Args) {
			t.Errorf("job %d:\n got %+v\nwant %+v", i, jobs[i], want[i])
		}
	}
}

func genEntries(t *rapid.T) []RootEntry {
	special := []string{"$RECYCLE.BIN", "System Volume Information", "_quarantine", "x.partial", "Derived", "Originals"}
	n := rapid.IntRange(0, 12).Draw(t, "n")
	seen := map[string]bool{}
	var out []RootEntry
	for i := 0; i < n; i++ {
		var name string
		if rapid.IntRange(0, 4).Draw(t, "special?") == 0 {
			name = rapid.SampledFrom(special).Draw(t, "special")
		} else {
			name = rapid.StringMatching(`[A-Za-z0-9][A-Za-z0-9 _.-]{0,10}[A-Za-z0-9]`).Draw(t, "name")
		}
		if seen[Fold(name)] {
			continue
		}
		seen[Fold(name)] = true
		e := RootEntry{Name: name, IsDir: rapid.Bool().Draw(t, "dir")}
		if e.IsDir {
			e.Marked = rapid.Bool().Draw(t, "marked")
			if e.Marked {
				e.DerivedName = rapid.SampledFrom([]string{"", "Derived", "derived", "DERIVED"}).Draw(t, "derived")
			}
		}
		out = append(out, e)
	}
	return out
}

// Property: for any root listing, the plan copies every top-level entry
// exactly once, only from primary to the same path on backup, additively in
// pass 1, excludes Derived folders from pass 1 by full path, overwrites only
// Derived folders, and never targets the quarantine folder.
func TestPlanInvariants(t *testing.T) {
	p, b, q := roots()
	qname := filepath.Base(q)
	rapid.Check(t, func(t *rapid.T) {
		entries := genEntries(t)
		report := rapid.Bool().Draw(t, "report")
		jobs := Plan(PlanInput{Primary: p, Backup: b, Entries: entries, QuarantineName: qname, Log: "log", Report: report})

		sawPass2 := false
		for _, j := range jobs {
			if err := CheckJob(j, p, b, q); err != nil {
				t.Fatalf("CheckJob: %v", err)
			}
			rel, _ := filepath.Rel(p, j.Src)
			if j.Dst != filepath.Join(b, rel) {
				t.Fatalf("job %s -> %s doesn't keep the path", j.Src, j.Dst)
			}
			if slices.Contains(j.Args, "/L") != report {
				t.Fatalf("report=%v but args %v", report, j.Args)
			}
			additive := slices.Contains(j.Args, "/XC") && slices.Contains(j.Args, "/XN") && slices.Contains(j.Args, "/XO")
			switch j.Pass {
			case 1:
				if sawPass2 {
					t.Fatal("pass 1 job after a pass 2 job")
				}
				if !additive {
					t.Fatalf("pass 1 job without /XC /XN /XO: %v", j.Args)
				}
			case 2:
				sawPass2 = true
				if filepath.Dir(j.Src) != filepath.Join(p, j.Rel) || Fold(filepath.Base(j.Src)) != Fold(DerivedDir) {
					t.Fatalf("pass 2 job outside a Derived folder: %s", j.Src)
				}
			default:
				t.Fatalf("pass %d", j.Pass)
			}
		}

		files := false
		for _, e := range entries {
			skipped := SkipTop(e.Name, qname)
			if !e.IsDir {
				files = files || (!skipped && !IsPartial(e.Name))
				continue
			}
			src := filepath.Join(p, e.Name)
			var p1, p2 []Job
			for _, j := range jobs {
				if j.Pass == 1 && j.Src == src {
					p1 = append(p1, j)
				}
				if j.Pass == 2 && filepath.Dir(j.Src) == src {
					p2 = append(p2, j)
				}
			}
			if skipped {
				if len(p1)+len(p2) > 0 {
					t.Fatalf("skipped %q has jobs", e.Name)
				}
				continue
			}
			if len(p1) != 1 || !slices.Contains(p1[0].Args, "/E") {
				t.Fatalf("%q: want one recursive pass 1 job, got %+v", e.Name, p1)
			}
			var xd []string
			for i, a := range p1[0].Args {
				if a == "/XD" {
					xd = append(xd, p1[0].Args[i+1])
				}
			}
			switch {
			case e.Marked:
				derived := e.DerivedName
				if derived == "" {
					derived = DerivedDir
				}
				if !slices.Equal(xd, []string{filepath.Join(src, derived)}) {
					t.Fatalf("%q: /XD %v", e.Name, xd)
				}
			case len(xd) > 0:
				t.Fatalf("legacy %q excludes %v", e.Name, xd)
			}
			if wantP2 := e.Marked && e.DerivedName != ""; (len(p2) == 1) != wantP2 || len(p2) > 1 {
				t.Fatalf("%q: pass 2 jobs %+v", e.Name, p2)
			}
		}
		rootJobs := 0
		for _, j := range jobs {
			if j.Src == p {
				rootJobs++
				if slices.Contains(j.Args, "/E") {
					t.Fatal("root files job recurses")
				}
			}
		}
		if (rootJobs == 1) != files || rootJobs > 1 {
			t.Fatalf("root files %v, root jobs %d", files, rootJobs)
		}
	})
}

func TestCheckJobRefuses(t *testing.T) {
	p, b, q := roots()
	ok := Job{Pass: 1, Src: filepath.Join(p, "x"), Dst: filepath.Join(b, "x")}
	ok.Args = []string{ok.Src, ok.Dst, "/E"}
	if err := CheckJob(ok, p, b, q); err != nil {
		t.Fatalf("good job refused: %v", err)
	}
	job := func(src, dst string, extra ...string) Job {
		return Job{Pass: 1, Src: src, Dst: dst, Args: append([]string{src, dst, "/E"}, extra...)}
	}
	noArgs := job(filepath.Join(p, "x"), filepath.Join(b, "x"))
	noArgs.Args = nil
	bad := map[string]Job{
		"reversed, would write to primary": job(filepath.Join(b, "x"), filepath.Join(p, "x")),
		"writes to primary":                job(filepath.Join(p, "x"), filepath.Join(p, "y")),
		"into quarantine":                  job(filepath.Join(p, "_quarantine"), q),
		"inside quarantine":                job(filepath.Join(p, "x"), filepath.Join(q, "x")),
		"args don't match":                 noArgs,
		"forbidden flag":                   job(filepath.Join(p, "x"), filepath.Join(b, "x"), "/MIR"),
	}
	for name, j := range bad {
		if err := CheckJob(j, p, b, q); err == nil {
			t.Errorf("%s: accepted %+v", name, j)
		}
	}
}
