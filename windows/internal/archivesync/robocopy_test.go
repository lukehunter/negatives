package archivesync

import (
	"testing"

	"pgregory.net/rapid"
)

func TestInterpret(t *testing.T) {
	cases := []struct {
		code int
		want Outcome
	}{
		{0, Outcome{Code: 0, OK: true}},
		{1, Outcome{Code: 1, OK: true, Copied: true}},
		{2, Outcome{Code: 2, OK: true, Extra: true}},
		{3, Outcome{Code: 3, OK: true, Copied: true, Extra: true}},
		{4, Outcome{Code: 4, OK: true, Mismatch: true}},
		{7, Outcome{Code: 7, OK: true, Copied: true, Extra: true, Mismatch: true}},
		{8, Outcome{Code: 8, Failed: true}},
		{9, Outcome{Code: 9, Copied: true, Failed: true}},
		{16, Outcome{Code: 16, Fatal: true}},
		{-1, Outcome{Code: -1, Fatal: true}},
		{259, Outcome{Code: 259, Fatal: true}},
	}
	for _, c := range cases {
		if got := Interpret(c.code); got != c.want {
			t.Errorf("Interpret(%d) = %+v, want %+v", c.code, got, c.want)
		}
	}
}

// Property: a run succeeds exactly when robocopy exits 0 to 7.
func TestInterpretOKExactlyZeroToSeven(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		code := rapid.IntRange(-1000, 1000).Draw(t, "code")
		o := Interpret(code)
		if o.OK != (code >= 0 && code <= 7) {
			t.Fatalf("Interpret(%d).OK = %v", code, o.OK)
		}
		if o.OK && (o.Failed || o.Fatal) {
			t.Fatalf("Interpret(%d) is OK but failed: %+v", code, o)
		}
	})
}

func TestForbidden(t *testing.T) {
	for _, args := range [][]string{{"a", "b", "/MIR"}, {"/purge"}, {"/MOV"}, {"/Move"}, {"/CREATE"}} {
		if _, bad := Forbidden(args); !bad {
			t.Errorf("Forbidden(%v) = false", args)
		}
	}
	if f, bad := Forbidden([]string{"P:\\", "B:\\", "/E", "/XC", "/XN", "/XO", "/XD", "P:\\MIR"}); bad {
		t.Errorf("flagged %q", f)
	}
}
