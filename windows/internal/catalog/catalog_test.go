package catalog

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"
)

// vectors mirrors testdata/vectors.json, which testdata/make_vectors.py
// writes from an independent reference implementation.
type vectors struct {
	Normalize []struct {
		In    string `json:"in"`
		Out   string `json:"out"`
		Error bool   `json:"error"`
	} `json:"normalize"`
	Fold []struct {
		In  string `json:"in"`
		Out string `json:"out"`
	} `json:"fold"`
	PathKey []struct {
		Loc   Loc    `json:"loc"`
		Scope string `json:"scope"`
		Path  string `json:"path"`
		Key   string `json:"key"`
		Error bool   `json:"error"`
	} `json:"pathKey"`
	MD5 []struct {
		Text   string `json:"text"`
		Repeat int    `json:"repeat"`
		MD5    string `json:"md5"`
	} `json:"md5"`
	State []struct {
		Size    int64    `json:"size"`
		Summary *Summary `json:"summary"`
		State   State    `json:"state"`
	} `json:"state"`
}

func (s *Summary) UnmarshalJSON(b []byte) error {
	var v struct{ PrimaryCopies, BackupCopies int }
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = Summary{PrimaryCopies: v.PrimaryCopies, BackupCopies: v.BackupCopies}
	return nil
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	b, err := os.ReadFile("../../../testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Normalize) == 0 || len(v.Fold) == 0 || len(v.PathKey) == 0 || len(v.MD5) == 0 || len(v.State) == 0 {
		t.Fatal("vectors.json is missing a section")
	}
	return v
}

func TestGoldenNormalize(t *testing.T) {
	for _, c := range loadVectors(t).Normalize {
		got, err := NormalizePath(c.In)
		switch {
		case c.Error && err == nil:
			t.Errorf("NormalizePath(%q) = %q, want an error", c.In, got)
		case c.Error && !errors.Is(err, ErrInvalidPath):
			t.Errorf("NormalizePath(%q) error %v doesn't wrap ErrInvalidPath", c.In, err)
		case !c.Error && err != nil:
			t.Errorf("NormalizePath(%q): %v, want %q", c.In, err, c.Out)
		case !c.Error && got != c.Out:
			t.Errorf("NormalizePath(%q) = %q, want %q", c.In, got, c.Out)
		}
	}
}

func TestGoldenFold(t *testing.T) {
	for _, c := range loadVectors(t).Fold {
		if got := FoldPath(c.In); got != c.Out {
			t.Errorf("FoldPath(%q) = %q, want %q", c.In, got, c.Out)
		}
	}
}

func TestGoldenPathKey(t *testing.T) {
	for _, c := range loadVectors(t).PathKey {
		got, err := PathKey(c.Loc, c.Scope, c.Path)
		switch {
		case c.Error && err == nil:
			t.Errorf("PathKey(%q, %q, %q) = %q, want an error", c.Loc, c.Scope, c.Path, got)
		case !c.Error && err != nil:
			t.Errorf("PathKey(%q, %q, %q): %v", c.Loc, c.Scope, c.Path, err)
		case !c.Error && got != c.Key:
			t.Errorf("PathKey(%q, %q, %q) = %q, want %q", c.Loc, c.Scope, c.Path, got, c.Key)
		}
	}
}

func TestGoldenMD5(t *testing.T) {
	for _, c := range loadVectors(t).MD5 {
		n := c.Repeat
		if n == 0 {
			n = 1
		}
		got, err := MD5Hex(strings.NewReader(strings.Repeat(c.Text, n)))
		if err != nil || got != c.MD5 {
			t.Errorf("MD5Hex(%q x %d) = %q, %v; want %q", c.Text, n, got, err, c.MD5)
		}
	}
}

func TestGoldenState(t *testing.T) {
	for _, c := range loadVectors(t).State {
		if got := StateOf(c.Size, c.Summary); got != c.State {
			t.Errorf("StateOf(%d, %+v) = %q, want %q", c.Size, c.Summary, got, c.State)
		}
	}
}

// pathGen makes paths from pieces likely to matter: case, NFD accents,
// spaces, dots, separators and characters that must be rejected.
func pathGen() *rapid.Generator[string] {
	pieces := []string{"DCIM", "dcim", "100MSDCF", "DSC00001", ".ARW", ".jpg", "PRIVATE", "M4ROOT",
		"Café", "Café", "Å", "Å", " ", ".", "/", "-", "_", "İ", "Σ",
		"CON", "a", "Z", ":", "\\", "\x01"}
	return rapid.Custom(func(t *rapid.T) string {
		n := rapid.IntRange(1, 12).Draw(t, "n")
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(rapid.SampledFrom(pieces).Draw(t, "piece"))
		}
		return b.String()
	})
}

func TestNormalizeProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen().Draw(t, "path")
		n, err := NormalizePath(p)
		if err != nil {
			return
		}
		if again, err := NormalizePath(n); err != nil || again != n {
			t.Fatalf("not idempotent: %q -> %q -> %q, %v", p, n, again, err)
		}
		if !norm.NFC.IsNormalString(n) || utf8.RuneCountInString(n) > MaxPathLen {
			t.Fatalf("%q: not NFC or too long", n)
		}
		if strings.ContainsAny(n, forbidden) || strings.HasPrefix(n, "/") || strings.HasSuffix(n, "/") || strings.Contains(n, "//") {
			t.Fatalf("%q accepted but breaks a rule", n)
		}
		// NFD input gives the same stored path.
		if d, err := NormalizePath(norm.NFD.String(p)); err != nil || d != n {
			t.Fatalf("NFD of %q gave %q, %v; want %q", p, d, err, n)
		}
	})
}

func TestPathKeyProperties(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		p := pathGen().Draw(t, "path")
		loc := rapid.SampledFrom([]Loc{LocSSD, LocPrimary, LocBackup}).Draw(t, "loc")
		k, err := PathKey(loc, "", p)
		if err != nil {
			return
		}
		if len(k) != 16 || strings.Trim(k, "0123456789abcdef") != "" {
			t.Fatalf("key %q isn't 16 lowercase hex characters", k)
		}
		// Case and normalization form never change the key.
		for _, variant := range []string{strings.ToUpper(p), strings.ToLower(p), norm.NFD.String(p)} {
			if _, err := NormalizePath(variant); err != nil {
				continue // upper-casing can make a reserved name valid or not
			}
			if kv, _ := PathKey(loc, "", variant); kv != k {
				t.Fatalf("PathKey(%q) = %q but PathKey(%q) = %q", p, k, variant, kv)
			}
		}
		// The location is part of the key.
		other := LocPrimary
		if loc == LocPrimary {
			other = LocBackup
		}
		if ko, _ := PathKey(other, "", p); ko == k {
			t.Fatalf("%s and %s share key %q for %q", loc, other, k, p)
		}
	})
}

func TestStateIsSafeOnlyWithBothDisks(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		size := rapid.Int64Range(0, 10).Draw(t, "size")
		var s *Summary
		if rapid.Bool().Draw(t, "has summary") {
			s = &Summary{PrimaryCopies: rapid.IntRange(-2, 3).Draw(t, "p"), BackupCopies: rapid.IntRange(-2, 3).Draw(t, "b")}
		}
		got := StateOf(size, s)
		safe := size > 0 && s != nil && s.PrimaryCopies > 0 && s.BackupCopies > 0
		if (got == Safe) != safe {
			t.Fatalf("StateOf(%d, %+v) = %s", size, s, got)
		}
	})
}

func TestPathKeyScopeRules(t *testing.T) {
	if _, err := PathKey(LocCard, "", "DCIM/a.jpg"); err == nil {
		t.Error("card copy without a cardKey accepted")
	}
	if _, err := PathKey(LocPrimary, "1a2b", "a.jpg"); err == nil {
		t.Error("archive copy with a scope accepted")
	}
	a, _ := PathKey(LocCard, "1a2b", "DCIM/a.jpg")
	b, _ := PathKey(LocCard, "3c4d", "DCIM/a.jpg")
	if a == b {
		t.Error("two cards share a key for the same path")
	}
}
