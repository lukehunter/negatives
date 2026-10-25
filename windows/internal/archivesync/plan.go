package archivesync

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// RootEntry is one top-level entry on the primary disk.
type RootEntry struct {
	Name  string
	IsDir bool
	// Marked: the folder holds a .negatives marker, so it's a negatives
	// shoot with Originals and Derived folders.
	Marked bool
	// DerivedName is the name of the shoot's Derived folder as the disk
	// reports it, or "" if it has none.
	DerivedName string
}

// PlanInput is everything Plan needs.
type PlanInput struct {
	Primary, Backup string // disk roots, such as P:\ and B:\
	Entries         []RootEntry
	QuarantineName  string // the quarantine folder's name at the backup root
	Log             string // robocopy's log file (/UNILOG+)
	Report          bool   // list only (/L), copy nothing
}

// Job is one robocopy run.
type Job struct {
	Pass     int    // 1: additive only; 2: a Derived folder, overwrites allowed
	Rel      string // the top-level entry it covers; "" for files at the root
	Src, Dst string
	Args     []string // the full robocopy argument list, Src and Dst first
}

// Plan returns the robocopy jobs for one run, in order.
//
// Pass 1 copies everything additively: /XC /XN /XO mean a file already on
// the backup disk is never overwritten, whatever its size or time. Each
// top-level folder gets its own job, so a negatives shoot's Derived folder
// can be excluded by its full path without one command line holding every
// shoot. Files at the disk root get one job of their own.
//
// Pass 2 copies each negatives shoot's Derived folder with overwrites
// allowed, so replaced renders and edits reach the backup.
//
// No job ever has /MIR or /PURGE, so nothing on the backup disk is deleted,
// and every job's destination is the same path on the backup disk.
func Plan(in PlanInput) []Job {
	entries := append([]RootEntry(nil), in.Entries...)
	sort.Slice(entries, func(i, j int) bool { return Fold(entries[i].Name) < Fold(entries[j].Name) })

	var jobs []Job
	rootFiles := false
	for _, e := range entries {
		if !e.IsDir && !SkipTop(e.Name, in.QuarantineName) && !IsPartial(e.Name) {
			rootFiles = true
		}
	}
	if rootFiles {
		// No /E: only the files directly at the root. /DCOPY:T copies the
		// root's timestamps but not its hidden and system attributes.
		args := []string{in.Primary, in.Backup, "/XC", "/XN", "/XO", "/DCOPY:T"}
		jobs = append(jobs, Job{Pass: 1, Src: in.Primary, Dst: in.Backup,
			Args: append(args, commonFlags(in)...)})
	}
	for _, e := range entries {
		if !e.IsDir || SkipTop(e.Name, in.QuarantineName) {
			continue
		}
		src, dst := filepath.Join(in.Primary, e.Name), filepath.Join(in.Backup, e.Name)
		args := []string{src, dst, "/E", "/XC", "/XN", "/XO"}
		args = append(args, commonFlags(in)...)
		if e.Marked {
			derived := e.DerivedName
			if derived == "" {
				// Excluded even before it exists, in case it appears
				// mid-run.
				derived = DerivedDir
			}
			args = append(args, "/XD", filepath.Join(src, derived))
		}
		jobs = append(jobs, Job{Pass: 1, Rel: e.Name, Src: src, Dst: dst, Args: args})
	}
	for _, e := range entries {
		if !e.IsDir || !e.Marked || e.DerivedName == "" || SkipTop(e.Name, in.QuarantineName) {
			continue
		}
		src := filepath.Join(in.Primary, e.Name, e.DerivedName)
		dst := filepath.Join(in.Backup, e.Name, e.DerivedName)
		args := append([]string{src, dst, "/E"}, commonFlags(in)...)
		jobs = append(jobs, Job{Pass: 2, Rel: e.Name, Src: src, Dst: dst, Args: args})
	}
	return jobs
}

// commonFlags are on every job:
//
//	/R:2 /W:5   retry a failed file twice, five seconds apart
//	/XJ         never follow junctions, which could lead off the disk
//	/NP /NDL /NJH /NJS /FP   log only files copied and errors, with full paths
//	/XF *.partial            skip ssd-push's in-progress temporary files
//	/L          report mode: list what would be copied, copy nothing
//	/UNILOG+:   append to the log as Unicode, so any file name logs correctly
func commonFlags(in PlanInput) []string {
	f := []string{"/R:2", "/W:5", "/XJ", "/NP", "/NDL", "/NJH", "/NJS", "/FP", "/XF", "*" + PartialSuffix}
	if in.Report {
		f = append(f, "/L")
	}
	return append(f, "/UNILOG+:"+in.Log)
}

// CheckJob is the last check before a job runs: robocopy must read only
// from the primary disk, write only to the backup disk outside the
// quarantine folder, and never get a flag that deletes or moves.
func CheckJob(j Job, primary, backup, quarantine string) error {
	if f, bad := Forbidden(j.Args); bad {
		return fmt.Errorf("job for %q has forbidden flag %s", j.Rel, f)
	}
	if len(j.Args) < 2 || j.Args[0] != j.Src || j.Args[1] != j.Dst {
		return fmt.Errorf("job for %q: arguments don't start with its source and destination", j.Rel)
	}
	if !Within(primary, j.Src) || Within(backup, j.Src) {
		return fmt.Errorf("job source %s is not on the primary disk", j.Src)
	}
	if !Within(backup, j.Dst) || Within(primary, j.Dst) {
		return fmt.Errorf("job destination %s is not on the backup disk", j.Dst)
	}
	if Within(quarantine, j.Dst) || (Within(j.Dst, quarantine) && j.Dst != backup) {
		return fmt.Errorf("job destination %s overlaps the quarantine folder", j.Dst)
	}
	return nil
}

// Within reports whether p is root or inside it. On Windows the comparison
// ignores case, as filepath.Rel does there.
func Within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
