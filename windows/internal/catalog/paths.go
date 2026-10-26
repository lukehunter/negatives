// Package catalog holds the catalog schema rules the Windows tools share
// with the Mac tools: path normalization, case folding, pathKey, MD5 and the
// derived file states. See docs/schema.md (Conventions). The Swift side
// implements the same rules, and both are checked against the golden
// vectors in testdata/vectors.json, so the two never disagree.
package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// MaxPathLen is the longest path accepted, in code points after NFC.
const MaxPathLen = 1024

// Loc is where a copy lives.
type Loc string

const (
	LocCard    Loc = "card"
	LocSSD     Loc = "ssd"
	LocPrimary Loc = "primary"
	LocBackup  Loc = "backup"
)

// Valid reports whether l is one of the four locations.
func (l Loc) Valid() bool {
	switch l {
	case LocCard, LocSSD, LocPrimary, LocBackup:
		return true
	}
	return false
}

// forbidden are characters Windows doesn't allow in names; the backslash
// too, since it's a separator there.
const forbidden = `:*?"<>|\`

var reservedNames = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for i := 1; i <= 9; i++ {
		m[fmt.Sprintf("COM%d", i)] = true
		m[fmt.Sprintf("LPT%d", i)] = true
	}
	return m
}()

// ErrInvalidPath is wrapped by every error NormalizePath returns.
var ErrInvalidPath = errors.New("invalid path")

// NormalizePath returns p as the catalog stores it: "/"-separated, relative,
// Unicode NFC, case preserved. It rejects paths that can't exist on every
// disk the tools use (see docs/schema.md): empty, ".", ".." or empty
// segments (so no leading, trailing or doubled "/"); control characters;
// : * ? " < > | and backslash; segments ending in a space or period;
// Windows reserved device names, with or without an extension; more than
// MaxPathLen code points. Callers on Windows convert separators first
// (filepath.ToSlash).
func NormalizePath(p string) (string, error) {
	if !utf8.ValidString(p) {
		return "", fmt.Errorf("%w: not UTF-8", ErrInvalidPath)
	}
	p = norm.NFC.String(p)
	if p == "" {
		return "", fmt.Errorf("%w: empty", ErrInvalidPath)
	}
	if n := utf8.RuneCountInString(p); n > MaxPathLen {
		return "", fmt.Errorf("%w: %d characters, more than %d", ErrInvalidPath, n, MaxPathLen)
	}
	for _, r := range p {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%w: control character U+%04X in %q", ErrInvalidPath, r, p)
		}
		if strings.ContainsRune(forbidden, r) {
			return "", fmt.Errorf("%w: %q contains %q", ErrInvalidPath, p, r)
		}
	}
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "" || seg == "." || seg == "..":
			return "", fmt.Errorf("%w: %q has an empty, . or .. segment", ErrInvalidPath, p)
		case strings.HasSuffix(seg, " ") || strings.HasSuffix(seg, "."):
			return "", fmt.Errorf("%w: segment %q ends in a space or period", ErrInvalidPath, seg)
		}
		base, _, _ := strings.Cut(seg, ".")
		if reservedNames[strings.ToUpper(base)] {
			return "", fmt.Errorf("%w: %q is a Windows reserved name", ErrInvalidPath, seg)
		}
	}
	return p, nil
}

// FoldPath is the case-folded form used for pathKey and every path
// comparison: each code point mapped to its Unicode simple lowercase
// mapping, then NFC. Simple mappings are one code point to one, with no
// context, so U+0130 becomes a plain "i" and a final sigma stays σ. p
// should already be normalized.
func FoldPath(p string) string {
	return norm.NFC.String(strings.Map(unicode.ToLower, p))
}

// SamePath reports whether two normalized paths name the same file on a
// case-insensitive disk.
func SamePath(a, b string) bool { return FoldPath(a) == FoldPath(b) }

// PathKey is the document ID of a copy in files/{md5}/paths: the first 16
// hex characters of SHA-256 over loc, a NUL byte, the scope, a NUL byte and
// the folded, normalized path. The scope is the card's cardKey (its volume
// UUID, lowercase) for card copies, and must be empty otherwise.
func PathKey(loc Loc, scope, path string) (string, error) {
	if !loc.Valid() {
		return "", fmt.Errorf("unknown loc %q", loc)
	}
	if (loc == LocCard) != (scope != "") {
		return "", fmt.Errorf("loc %q with scope %q: card copies need the cardKey as scope, other copies none", loc, scope)
	}
	n, err := NormalizePath(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(loc))
	h.Write([]byte{0})
	h.Write([]byte(scope))
	h.Write([]byte{0})
	h.Write([]byte(FoldPath(n)))
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}
