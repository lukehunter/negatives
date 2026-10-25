// Package config reads the Windows config file, C:\ProgramData\negatives\config.toml.
// See docs/config.md.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultPath is where the Windows tools look for their config file.
const DefaultPath = `C:\ProgramData\negatives\config.toml`

// Windows is the Windows config file.
type Windows struct {
	Disks struct {
		Primary    string `toml:"primary"`
		Backup     string `toml:"backup"`
		Quarantine string `toml:"quarantine"`
	} `toml:"disks"`
	GCP struct {
		Project string `toml:"project"`
		Key     string `toml:"key"`
	} `toml:"gcp"`
	Schedule struct {
		Nightly string `toml:"nightly"`
	} `toml:"schedule"`
	Scrub struct {
		BudgetMinutes int      `toml:"budget_minutes"`
		SmartDisks    []string `toml:"smart_disks"`
	} `toml:"scrub"`
	Logs struct {
		Dir string `toml:"dir"`
	} `toml:"logs"`
}

// Defaults returns the built-in defaults. The disks have none: they must be
// set in the file.
func Defaults() Windows {
	var w Windows
	w.GCP.Project = "negatives-8c3e2"
	w.GCP.Key = `C:\ProgramData\negatives\service-account.json`
	w.Schedule.Nightly = "03:00"
	w.Scrub.BudgetMinutes = 45
	w.Scrub.SmartDisks = []string{"auto"}
	w.Logs.Dir = `C:\ProgramData\negatives\logs`
	return w
}

// Parse reads a config file's text over the defaults. Unknown keys are an
// error, so a misspelled key can't silently fall back to a default.
func Parse(text string) (Windows, error) {
	w := Defaults()
	md, err := toml.Decode(text, &w)
	if err != nil {
		return Windows{}, err
	}
	if und := md.Undecoded(); len(und) > 0 {
		keys := make([]string, len(und))
		for i, k := range und {
			keys[i] = k.String()
		}
		sort.Strings(keys)
		return Windows{}, fmt.Errorf("unknown settings: %s", strings.Join(keys, ", "))
	}
	if w.Disks.Quarantine == "" && w.Disks.Backup != "" {
		w.Disks.Quarantine = filepath.Join(w.Disks.Backup, "_quarantine")
	}
	return w, nil
}

// Load reads and parses the config file at path.
func Load(path string) (Windows, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Windows{}, err
	}
	w, err := Parse(string(b))
	if err != nil {
		return Windows{}, fmt.Errorf("%s: %w", path, err)
	}
	return w, nil
}

// Validate checks the settings that don't depend on the disks.
func (w Windows) Validate() error {
	var errs []error
	if _, err := time.Parse("15:04", w.Schedule.Nightly); err != nil {
		errs = append(errs, fmt.Errorf("schedule.nightly %q is not a time like 03:00", w.Schedule.Nightly))
	}
	if w.Scrub.BudgetMinutes < 1 || w.Scrub.BudgetMinutes > 24*60 {
		errs = append(errs, fmt.Errorf("scrub.budget_minutes %d is out of range (1 to 1440)", w.Scrub.BudgetMinutes))
	}
	if !filepath.IsAbs(w.Logs.Dir) {
		errs = append(errs, fmt.Errorf("logs.dir %q is not an absolute path", w.Logs.Dir))
	}
	return errors.Join(errs...)
}

// Env is how ValidateDisks looks at the machine; tests replace it.
type Env struct {
	Stat func(string) (os.FileInfo, error)
	// VolumeID identifies the volume a path is on.
	VolumeID func(string) (string, error)
}

// OSEnv looks at the real machine.
func OSEnv() Env { return Env{Stat: os.Stat, VolumeID: volumeID} }

// ValidateDisks checks the disk settings archive-sync relies on:
//
//   - primary and backup are set, exist, and are different volumes, with
//     neither inside the other;
//   - quarantine is a folder directly at the backup disk's root (it may not
//     exist yet), on the backup volume;
//   - the logs folder is on neither archive disk, so robocopy's log can
//     never be copied to the backup or written to the primary.
func (w Windows) ValidateDisks(env Env) error {
	p, b, q := w.Disks.Primary, w.Disks.Backup, w.Disks.Quarantine
	if p == "" || b == "" {
		return errors.New("disks.primary and disks.backup must both be set")
	}
	for _, d := range []struct{ key, path string }{{"disks.primary", p}, {"disks.backup", b}, {"disks.quarantine", q}} {
		if !filepath.IsAbs(d.path) {
			return fmt.Errorf("%s %q is not an absolute path", d.key, d.path)
		}
	}
	for _, d := range []struct{ key, path string }{{"disks.primary", p}, {"disks.backup", b}} {
		st, err := env.Stat(d.path)
		if err != nil {
			return fmt.Errorf("%s: %w", d.key, err)
		}
		if !st.IsDir() {
			return fmt.Errorf("%s %q is not a folder", d.key, d.path)
		}
	}
	if within(p, b) || within(b, p) {
		return fmt.Errorf("disks.primary %q and disks.backup %q overlap", p, b)
	}
	pv, err := env.VolumeID(p)
	if err != nil {
		return fmt.Errorf("disks.primary: %w", err)
	}
	bv, err := env.VolumeID(b)
	if err != nil {
		return fmt.Errorf("disks.backup: %w", err)
	}
	if pv == bv {
		return fmt.Errorf("disks.primary %q and disks.backup %q are on the same volume", p, b)
	}
	if !samePath(filepath.Dir(filepath.Clean(q)), filepath.Clean(b)) || samePath(q, b) {
		return fmt.Errorf("disks.quarantine %q must be a folder directly at the backup disk's root, %q", q, b)
	}
	if st, err := env.Stat(q); err == nil {
		if !st.IsDir() {
			return fmt.Errorf("disks.quarantine %q is not a folder", q)
		}
		qv, err := env.VolumeID(q)
		if err != nil {
			return fmt.Errorf("disks.quarantine: %w", err)
		}
		if qv != bv {
			return fmt.Errorf("disks.quarantine %q is not on the backup volume", q)
		}
	}
	if within(p, w.Logs.Dir) || within(b, w.Logs.Dir) {
		return fmt.Errorf("logs.dir %q must not be on an archive disk", w.Logs.Dir)
	}
	return nil
}

func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func samePath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && rel == "."
}
