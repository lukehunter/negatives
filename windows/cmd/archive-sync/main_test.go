package main

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
	"github.com/lukehunter/negatives/windows/internal/config"
	"github.com/lukehunter/negatives/windows/internal/diskid"
)

type countingRunner struct{ calls int }

func (r *countingRunner) Robocopy(context.Context, []string) (int, error) {
	r.calls++
	return 0, nil
}

type setup struct {
	dir, primary, backup, logs, fallback, configPath string
	primaryID, backupID                              string
	runner                                           *countingRunner
	stdout, stderr                                   bytes.Buffer
}

func newSetup(t *testing.T) *setup {
	s := &setup{dir: t.TempDir(), runner: &countingRunner{}}
	s.primary, s.backup = filepath.Join(s.dir, "P"), filepath.Join(s.dir, "B")
	s.logs, s.fallback = filepath.Join(s.dir, "logs"), filepath.Join(s.dir, "fallback-logs")
	s.configPath = filepath.Join(s.dir, "config.toml")
	for _, p := range []string{s.primary, s.backup} {
		if err := os.Mkdir(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pid, err := diskid.Create(s.primary, diskid.Primary, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	bid, err := diskid.Create(s.backup, diskid.Backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s.primaryID, s.backupID = pid.ID, bid.ID
	os.WriteFile(filepath.Join(s.primary, "a.txt"), []byte("a"), 0o644)
	s.writeConfig(t, s.primary, s.backup)
	return s
}

func (s *setup) writeConfig(t *testing.T, primary, backup string) {
	t.Helper()
	text := fmt.Sprintf("[disks]\nprimary = '%s'\nbackup = '%s'\nprimary_id = %q\nbackup_id = %q\n[logs]\ndir = '%s'\n",
		primary, backup, s.primaryID, s.backupID, s.logs)
	if err := os.WriteFile(s.configPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (s *setup) run(args ...string) int {
	return run(context.Background(), append([]string{"--config", s.configPath}, args...), deps{
		// Temp folders share a volume; tell P and B apart by name.
		env: config.Env{Stat: os.Stat, VolumeID: func(p string) (string, error) {
			rel, _ := filepath.Rel(s.dir, p)
			return strings.Split(filepath.ToSlash(rel), "/")[0], nil
		}},
		runner:         s.runner,
		facts:          archivesync.NoFacts{},
		fallbackLogDir: s.fallback,
		now:            time.Now,
		stdout:         &s.stdout,
		stderr:         &s.stderr,
	})
}

func logText(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "archive-sync-"+time.Now().Format("2006-01-02")+".log"))
	if err != nil {
		t.Fatalf("no log in %s: %v", dir, err)
	}
	return string(b)
}

func listing(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		st, _ := d.Info()
		b, _ := os.ReadFile(p)
		m[p] = fmt.Sprintf("%v %d %d %s", d.IsDir(), st.Size(), st.ModTime().UnixNano(), b)
		return nil
	})
	return m
}

func TestRunSucceeds(t *testing.T) {
	s := newSetup(t)
	if code := s.run(); code != 0 {
		t.Fatalf("exit %d: %s", code, s.stderr.String())
	}
	if s.runner.calls == 0 {
		t.Fatal("robocopy never ran")
	}
	if !strings.Contains(logText(t, s.logs), "done:") {
		t.Fatal("no summary in the log")
	}
}

func TestOverlappingRunExitsZero(t *testing.T) {
	s := newSetup(t)
	held, err := archivesync.AcquireLock(filepath.Join(s.dir, "archive-sync.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	if code := s.run(); code != 0 {
		t.Fatalf("exit %d: %s", code, s.stderr.String())
	}
	if s.runner.calls != 0 {
		t.Fatal("robocopy ran while another run held the lock")
	}
	if !strings.Contains(logText(t, s.logs), "another archive-sync run is in progress") {
		t.Fatal("overlap not logged")
	}
}

func TestLockFileErrorFails(t *testing.T) {
	s := newSetup(t)
	// A folder where the lock file should be: it can't be opened.
	if err := os.Mkdir(filepath.Join(s.dir, "archive-sync.lock"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := s.run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(logText(t, s.logs), "ERROR") {
		t.Fatal("lock error not logged")
	}
}

func TestConfigErrorLoggedToFallback(t *testing.T) {
	s := newSetup(t)
	os.WriteFile(s.configPath, []byte("[disks]\nbakup = 'B'\n"), 0o644)
	if code := s.run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if l := logText(t, s.fallback); !strings.Contains(l, "ERROR config:") || !strings.Contains(l, "disks.bakup") {
		t.Fatalf("log: %q", l)
	}
}

// Drive letters swapped: the config's primary is really the backup disk.
// Nothing may be read past the identity check or written, in either mode.
func TestSwappedDisksRefused(t *testing.T) {
	for _, mode := range [][]string{nil, {"--report"}} {
		s := newSetup(t)
		s.writeConfig(t, s.backup, s.primary)
		disks := func() map[string]string {
			m := listing(t, s.primary)
			for k, v := range listing(t, s.backup) {
				m[k] = v
			}
			return m
		}
		before := disks()
		if code := s.run(mode...); code != 1 {
			t.Fatalf("%v: exit %d", mode, code)
		}
		if s.runner.calls != 0 {
			t.Fatalf("%v: robocopy ran", mode)
		}
		if l := logText(t, s.logs); !strings.Contains(l, "swapped") {
			t.Fatalf("%v: log %q", mode, l)
		}
		after := disks()
		for p, v := range before {
			if after[p] != v {
				t.Errorf("%v: %s changed", mode, p)
			}
		}
		for p := range after {
			if _, ok := before[p]; !ok {
				t.Errorf("%v: %s created", mode, p)
			}
		}
	}
}

func TestMissingIdentityRefused(t *testing.T) {
	s := newSetup(t)
	os.Remove(filepath.Join(s.backup, diskid.FileName))
	if code := s.run(); code != 1 || s.runner.calls != 0 {
		t.Fatalf("exit %d, %d robocopy calls", code, s.runner.calls)
	}
	if !strings.Contains(logText(t, s.logs), "no "+diskid.FileName) {
		t.Fatal("missing identity not logged")
	}
	if _, err := os.Stat(filepath.Join(s.backup, diskid.FileName)); err == nil {
		t.Fatal("archive-sync created an identity file")
	}
}

func TestQuarantineClashLogged(t *testing.T) {
	s := newSetup(t)
	os.Mkdir(filepath.Join(s.primary, "_quarantine"), 0o755)
	if code := s.run(); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(logText(t, s.logs), "quarantine folder's name") {
		t.Fatal("clash not logged")
	}
}
