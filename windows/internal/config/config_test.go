package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The example from docs/config.md.
const example = `
[disks]
primary = 'P:\'
backup = 'B:\'
quarantine = 'B:\_quarantine'           # must be on the backup disk
primary_id = "0f0e0d0c-0b0a-4908-8706-050403020100"
backup_id = "1f1e1d1c-1b1a-4918-9716-151413121110"

[gcp]
project = "negatives-8c3e2"
key = 'C:\ProgramData\negatives\service-account.json'

[schedule]
nightly = "03:00"                       # local time of the nightly safety-net run

[scrub]
budget_minutes = 45                     # time archive-scrub may spend each night
smart_disks = ["auto"]                  # or explicit disk numbers

[logs]
dir = 'C:\ProgramData\negatives\logs'
`

const (
	pid = "0f0e0d0c-0b0a-4908-8706-050403020100"
	bid = "1f1e1d1c-1b1a-4918-9716-151413121110"
)

func TestParseExample(t *testing.T) {
	w, err := Parse(example)
	if err != nil {
		t.Fatal(err)
	}
	if w.Disks.Primary != `P:\` || w.Disks.Backup != `B:\` || w.Disks.Quarantine != `B:\_quarantine` ||
		w.Disks.PrimaryID != pid || w.Disks.BackupID != bid {
		t.Errorf("disks %+v", w.Disks)
	}
	if w.Logs.Dir != `C:\ProgramData\negatives\logs` || w.Scrub.BudgetMinutes != 45 || w.Schedule.Nightly != "03:00" {
		t.Errorf("parsed %+v", w)
	}
}

func TestParseDefaults(t *testing.T) {
	w, err := Parse("[disks]\nprimary = '/p'\nbackup = '/b'\n")
	if err != nil {
		t.Fatal(err)
	}
	d := Defaults()
	if w.Logs.Dir != d.Logs.Dir || w.Schedule.Nightly != d.Schedule.Nightly || w.Scrub.BudgetMinutes != d.Scrub.BudgetMinutes {
		t.Errorf("defaults not applied: %+v", w)
	}
	if w.Disks.Quarantine != filepath.Join("/b", "_quarantine") {
		t.Errorf("default quarantine %q", w.Disks.Quarantine)
	}
}

func TestParseRejectsUnknownKeys(t *testing.T) {
	_, err := Parse("[disks]\nprimary = 'P:\\'\nbakup = 'B:\\'\n")
	if err == nil || !strings.Contains(err.Error(), "disks.bakup") {
		t.Fatalf("misspelled key: %v", err)
	}
}

func TestValidate(t *testing.T) {
	w := Defaults()
	if runtime.GOOS != "windows" {
		w.Logs.Dir = "/var/log/negatives"
	}
	if err := w.Validate(); err != nil {
		t.Fatalf("defaults invalid: %v", err)
	}
	w.Schedule.Nightly = "3am"
	w.Scrub.BudgetMinutes = 0
	err := w.Validate()
	if err == nil || !strings.Contains(err.Error(), "nightly") || !strings.Contains(err.Error(), "budget") {
		t.Fatalf("Validate = %v", err)
	}
}

type fakeDir struct{ fs.FileInfo }

func (fakeDir) IsDir() bool { return true }

type fakeFile struct{ fs.FileInfo }

func (fakeFile) IsDir() bool { return false }

func TestValidateDisks(t *testing.T) {
	// Paths on this OS: /p, /b and so on. Volumes are the first segment,
	// except /b2, which is on the same volume as /p.
	abs := func(p string) string {
		if runtime.GOOS == "windows" {
			return `C:` + filepath.FromSlash(p)
		}
		return p
	}
	files := map[string]fs.FileInfo{
		abs("/p"): fakeDir{}, abs("/b"): fakeDir{}, abs("/b2"): fakeDir{},
		abs("/b/_quarantine"): fakeDir{}, abs("/b/qfile"): fakeFile{},
	}
	env := Env{
		Stat: func(p string) (os.FileInfo, error) {
			if fi, ok := files[p]; ok {
				return fi, nil
			}
			return nil, fs.ErrNotExist
		},
		VolumeID: func(p string) (string, error) {
			rel := strings.TrimPrefix(filepath.ToSlash(p), filepath.ToSlash(abs("/")))
			v := strings.Split(rel, "/")[0]
			if v == "b2" {
				v = "p"
			}
			return v, nil
		},
	}
	good := func() Windows {
		w := Defaults()
		w.Disks.Primary, w.Disks.Backup, w.Disks.Quarantine = abs("/p"), abs("/b"), abs("/b/_quarantine")
		w.Disks.PrimaryID, w.Disks.BackupID = pid, bid
		w.Logs.Dir = abs("/logs")
		return w
	}
	if err := good().ValidateDisks(env); err != nil {
		t.Fatalf("good config refused: %v", err)
	}
	newQ := good()
	newQ.Disks.Quarantine = abs("/b/_new_quarantine") // doesn't exist yet: fine
	if err := newQ.ValidateDisks(env); err != nil {
		t.Fatalf("not-yet-created quarantine refused: %v", err)
	}

	bad := map[string]func(*Windows){
		"no primary":               func(w *Windows) { w.Disks.Primary = "" },
		"relative backup":          func(w *Windows) { w.Disks.Backup = "b" },
		"missing backup":           func(w *Windows) { w.Disks.Backup = abs("/nope") },
		"same disk":                func(w *Windows) { w.Disks.Backup = w.Disks.Primary; w.Disks.Quarantine = abs("/p/_quarantine") },
		"backup inside primary":    func(w *Windows) { w.Disks.Backup = abs("/p/b") },
		"same volume":              func(w *Windows) { w.Disks.Backup = abs("/b2"); w.Disks.Quarantine = abs("/b2/_quarantine") },
		"quarantine on primary":    func(w *Windows) { w.Disks.Quarantine = abs("/p/_quarantine") },
		"quarantine is the backup": func(w *Windows) { w.Disks.Quarantine = abs("/b") },
		"quarantine nested":        func(w *Windows) { w.Disks.Quarantine = abs("/b/x/_quarantine") },
		"quarantine is a file":     func(w *Windows) { w.Disks.Quarantine = abs("/b/qfile") },
		"logs on primary":          func(w *Windows) { w.Logs.Dir = abs("/p/logs") },
		"logs on backup":           func(w *Windows) { w.Logs.Dir = abs("/b/logs") },
		"no primary ID":            func(w *Windows) { w.Disks.PrimaryID = "" },
		"bad backup ID":            func(w *Windows) { w.Disks.BackupID = "B" },
		"same IDs":                 func(w *Windows) { w.Disks.BackupID = w.Disks.PrimaryID },
	}
	for name, mod := range bad {
		w := good()
		mod(&w)
		if err := w.ValidateDisks(env); err == nil {
			t.Errorf("%s: accepted %+v", name, w.Disks)
		}
	}
}

func TestValidateDisksRealVolumes(t *testing.T) {
	// Two temp folders are on one volume, so the real check must refuse.
	dir := t.TempDir()
	p, b := filepath.Join(dir, "p"), filepath.Join(dir, "b")
	os.Mkdir(p, 0o755)
	os.Mkdir(b, 0o755)
	w := Defaults()
	w.Disks.Primary, w.Disks.Backup, w.Disks.Quarantine = p, b, filepath.Join(b, "_quarantine")
	w.Disks.PrimaryID, w.Disks.BackupID = pid, bid
	w.Logs.Dir = filepath.Join(dir, "logs")
	err := w.ValidateDisks(OSEnv())
	if err == nil || !strings.Contains(err.Error(), "same volume") {
		t.Fatalf("ValidateDisks = %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "config.toml")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Load = %v", err)
	}
}
