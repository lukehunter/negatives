package archivesync

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRobocopy is a small model of robocopy, for tests on any OS. It
// understands the flags archive-sync uses: /E, /XC /XN /XO (all three: skip
// any file already on the destination), /XD full paths, /XF name patterns
// and /L. Without /XC /XN /XO it copies a file whose size or time differs.
// The real robocopy is checked against the same expectations on Windows in
// robocopy_windows_test.go.
type fakeRobocopy struct {
	calls [][]string
	// kill names a file: copying it stops after killAfter bytes, as if
	// robocopy were killed mid-file.
	kill      string
	killAfter int64
}

func (f *fakeRobocopy) Robocopy(ctx context.Context, args []string) (int, error) {
	f.calls = append(f.calls, args)
	src, dst := args[0], args[1]
	var recurse, list bool
	skips := 0
	var xd, xf []string
	var into *[]string
	for _, a := range args[2:] {
		if !strings.HasPrefix(a, "/") {
			if into != nil {
				*into = append(*into, a)
			}
			continue
		}
		into = nil
		switch strings.ToUpper(a) {
		case "/E":
			recurse = true
		case "/XC", "/XN", "/XO":
			skips++
		case "/L":
			list = true
		case "/XD":
			into = &xd
		case "/XF":
			into = &xf
		}
	}
	if !list {
		// robocopy creates the destination folder.
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return 16, nil
		}
	}
	copied := false
	var walk func(s, d string) error
	walk = func(s, d string) error {
		ents, err := os.ReadDir(s)
		if err != nil {
			return err
		}
		for _, e := range ents {
			sp, dp := filepath.Join(s, e.Name()), filepath.Join(d, e.Name())
			if e.IsDir() {
				excluded := false
				for _, x := range xd {
					excluded = excluded || strings.EqualFold(x, sp)
				}
				if !recurse || excluded {
					continue
				}
				if !list {
					if err := os.MkdirAll(dp, 0o755); err != nil {
						return err
					}
				}
				if err := walk(sp, dp); err != nil {
					return err
				}
				continue
			}
			excluded := false
			for _, pat := range xf {
				m, _ := filepath.Match(strings.ToLower(pat), strings.ToLower(e.Name()))
				excluded = excluded || m
			}
			if excluded {
				continue
			}
			si, _ := e.Info()
			if di, err := os.Stat(dp); err == nil {
				if skips == 3 || (di.Size() == si.Size() && di.ModTime().Equal(si.ModTime())) {
					continue
				}
			}
			copied = true
			if list {
				continue
			}
			if err := f.copyFile(sp, dp, si); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(src, dst); err != nil {
		if errors.Is(err, errKilled) {
			return -1, err
		}
		return 8, nil
	}
	if copied {
		return 1, nil
	}
	return 0, nil
}

var errKilled = errors.New("robocopy killed")

func (f *fakeRobocopy) copyFile(src, dst string, si fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if f.kill != "" && filepath.Base(src) == f.kill {
		_, _ = io.CopyN(out, in, f.killAfter)
		out.Close()
		f.kill = ""
		return errKilled
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(dst, si.ModTime(), si.ModTime())
}

// hasherFacts stands in for archive-hasher: it hashes the primary file
// and says it's verified and matches a card, unless told otherwise.
type hasherFacts struct {
	primary string
	modify  func(rel string, f *Facts)
	lookups []string
}

func (h *hasherFacts) Lookup(_ context.Context, rel string, p FileInfo) (Facts, error) {
	h.lookups = append(h.lookups, rel)
	sum, err := md5File(filepath.Join(h.primary, filepath.FromSlash(rel)))
	if err != nil {
		return Facts{}, err
	}
	f := Facts{Known: true, Verified: true, Size: p.Size, MtimeNs: p.MtimeNs, MD5: sum, CardMatch: true}
	if h.modify != nil {
		h.modify(rel, &f)
	}
	return f, nil
}

func md5File(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	s := md5.Sum(b)
	return hex.EncodeToString(s[:]), nil
}

var baseTime = time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

// put writes a file with the given content and modification time offset.
func put(t *testing.T, root, rel, content string, mtimeHours int) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mt := baseTime.Add(time.Duration(mtimeHours) * time.Hour)
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

// snapshot records every file and folder under root with its size, time
// and content hash, to prove a disk wasn't changed.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		st, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			m[rel] = "dir"
			return nil
		}
		sum, err := md5File(p)
		if err != nil {
			return err
		}
		m[rel] = fmt.Sprintf("%d %d %s", st.Size(), st.ModTime().UnixNano(), sum)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func sameSnapshot(t *testing.T, what string, before, after map[string]string) {
	t.Helper()
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s changed: %s was %q, now %q", what, k, v, after[k])
		}
	}
	for k, v := range after {
		if _, ok := before[k]; !ok {
			t.Errorf("%s changed: %s added (%s)", what, k, v)
		}
	}
}
