package diskid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func TestCreateAndCheck(t *testing.T) {
	p, b := t.TempDir(), t.TempDir()
	pid, err := Create(p, Primary, now)
	if err != nil {
		t.Fatal(err)
	}
	bid, err := Create(b, Backup, now)
	if err != nil {
		t.Fatal(err)
	}
	if !ValidID(pid.ID) || pid.ID == bid.ID {
		t.Fatalf("ids %q %q", pid.ID, bid.ID)
	}
	if got, err := Read(p); err != nil || got != pid {
		t.Fatalf("Read = %+v, %v; want %+v", got, err, pid)
	}
	if err := Check(p, Primary, pid.ID); err != nil {
		t.Fatalf("primary: %v", err)
	}
	if err := Check(b, Backup, bid.ID); err != nil {
		t.Fatalf("backup: %v", err)
	}

	refused := map[string]error{
		// Drive letters swapped: the config's primary is really the backup.
		"swapped primary": Check(b, Primary, pid.ID),
		"swapped backup":  Check(p, Backup, bid.ID),
		"replaced disk":   Check(b, Backup, "0f0e0d0c-0b0a-4908-8706-050403020100"),
		"no id in config": Check(p, Primary, ""),
		"no file":         Check(t.TempDir(), Primary, pid.ID),
	}
	for name, err := range refused {
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if err := Check(b, Primary, pid.ID); !strings.Contains(err.Error(), "swapped") {
		t.Errorf("swap message: %v", err)
	}
}

func TestCreateNeverOverwrites(t *testing.T) {
	d := t.TempDir()
	first, err := Create(d, Backup, now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(d, FileName))
	if _, err := Create(d, Primary, now); err == nil {
		t.Fatal("created over an existing identity")
	}
	after, _ := os.ReadFile(filepath.Join(d, FileName))
	if string(before) != string(after) {
		t.Fatal("identity file changed")
	}
	if got, _ := Read(d); got != first {
		t.Fatalf("identity now %+v", got)
	}
}

func TestCreateRefuses(t *testing.T) {
	if _, err := Create(t.TempDir(), "archive", now); err == nil {
		t.Error("accepted role archive")
	}
	if _, err := Create(filepath.Join(t.TempDir(), "nope"), Primary, now); err == nil {
		t.Error("created on a missing folder")
	}
}

func TestReadRejectsBadFiles(t *testing.T) {
	for name, text := range map[string]string{
		"bad role":      `role = "primry"` + "\nid = \"0f0e0d0c-0b0a-4908-8706-050403020100\"\n",
		"bad id":        `role = "primary"` + "\nid = \"P\"\n",
		"unknown field": `role = "primary"` + "\nid = \"0f0e0d0c-0b0a-4908-8706-050403020100\"\nletter = \"P\"\n",
		"not toml":      "primary",
	} {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, FileName), []byte(text), 0o644)
		if _, err := Read(d); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
