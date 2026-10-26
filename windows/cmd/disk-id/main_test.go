package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCreateThenShow(t *testing.T) {
	d := t.TempDir()
	var out, errs bytes.Buffer
	if code := run([]string{"create", "--role", "backup", d}, &out, &errs); code != 0 {
		t.Fatalf("create: %d %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "backup_id = ") {
		t.Fatalf("create printed %q", out.String())
	}
	line := out.String()[strings.Index(out.String(), "backup_id"):]
	out.Reset()
	if code := run([]string{"show", d}, &out, &errs); code != 0 || !strings.Contains(out.String(), line) {
		t.Fatalf("show: %d %q, want %q", code, out.String(), line)
	}
	// A second create never replaces the identity.
	if code := run([]string{"create", "--role", "primary", d}, &out, &errs); code != 1 {
		t.Fatalf("second create: %d", code)
	}
	out.Reset()
	run([]string{"show", d}, &out, &errs)
	if !strings.Contains(out.String(), line) {
		t.Fatal("identity changed")
	}
}

func TestUsage(t *testing.T) {
	var out, errs bytes.Buffer
	for _, args := range [][]string{nil, {"create", "x"}, {"create", "--role", "primary"}, {"show"}, {"delete", "x"}} {
		if code := run(args, &out, &errs); code != 1 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}
