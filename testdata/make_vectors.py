#!/usr/bin/env python3
"""Writes testdata/vectors.json, the golden vectors that the Go and Swift
test suites both load (docs/testing.md), so the two languages agree on path
normalization, case folding, pathKey, MD5 and the derived file states.

This is an independent reference implementation of the rules in
docs/schema.md (Conventions), using only Python's standard library, so the
expected values never come from the code under test. Run it from anywhere:

    python3 testdata/make_vectors.py

and commit the result. The output is deterministic.
"""

import hashlib
import json
import pathlib
import unicodedata

RESERVED = {"CON", "PRN", "AUX", "NUL"} | {f"COM{i}" for i in range(1, 10)} | {f"LPT{i}" for i in range(1, 10)}
FORBIDDEN = set(':*?"<>|\\')
MAX_LEN = 1024
LOCS = ("card", "ssd", "primary", "backup")


def normalize(p):
    """NFC, then the rejection rules. Returns None if p is rejected."""
    p = unicodedata.normalize("NFC", p)
    if p == "" or len(p) > MAX_LEN:
        return None
    for ch in p:
        if ord(ch) < 0x20 or ord(ch) == 0x7F or ch in FORBIDDEN:
            return None
    for seg in p.split("/"):
        if seg in ("", ".", ".."):
            return None
        if seg.endswith(" ") or seg.endswith("."):
            return None
        if seg.split(".", 1)[0].upper() in RESERVED:
            return None
    return p


def lower_simple(ch):
    """Unicode simple lowercase mapping of one code point. Python's
    str.lower() uses the full mapping, which differs only where the full
    mapping has more than one code point: U+0130 (full: i + U+0307)."""
    if ch == "İ":
        return "i"
    low = ch.lower()
    assert len(low) == 1, f"no simple mapping known for U+{ord(ch):04X}"
    return low


def fold(p):
    """Each code point to its simple lowercase mapping, then NFC."""
    return unicodedata.normalize("NFC", "".join(lower_simple(c) for c in p))


def path_key(loc, scope, path):
    n = normalize(path)
    if n is None or loc not in LOCS or (loc == "card") != (scope != ""):
        return None
    data = loc.encode() + b"\0" + scope.encode() + b"\0" + fold(n).encode()
    return hashlib.sha256(data).hexdigest()[:16]


def state(size, summary):
    if size == 0:
        return "ignored"
    if summary is None:
        return "not-copied"
    if summary["primaryCopies"] > 0 and summary["backupCopies"] > 0:
        return "safe"
    return "pending"


CARD = "1a2b3c4d-0000-4000-8000-000000000001"
CARD2 = "9f8e7d6c-0000-4000-8000-000000000002"
SHOOT = "2026-10-18 Coast"

normalize_inputs = [
    "DCIM/100MSDCF/DSC00001.ARW",
    "PRIVATE/M4ROOT/CLIP/C0001.MP4",
    "private/M4ROOT/CLIP/C0001.MP4",
    f"{SHOOT}/.originals/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW",
    f"{SHOOT}/Exports/coast pano.tif",
    f"{SHOOT}/.negatives",
    "Café/DSC00001.JPG",          # NFD é
    "Ångström/x.jpg",       # NFD Å and ö
    "한/x.jpg",         # NFD Hangul 한
    "CONSOLE.txt",
    "COM10/a",
    "LPT0",
    "a" * MAX_LEN,
    # rejected
    "",
    "/DCIM/a.jpg",
    "DCIM/a.jpg/",
    "DCIM//a.jpg",
    "./a.jpg",
    "DCIM/../a.jpg",
    "DCIM/.",
    "a:b",
    "a*b",
    "a?b",
    'a"b',
    "a<b",
    "a>b",
    "a|b",
    "a\\b",
    "a\u0001b",
    "a\tb",
    "a\u007fb",
    "a /b",
    "a./b",
    "Exports/final.",
    "CON",
    "dir/con.txt",
    "LPT9.log",
    "COM1",
    "nul",
    "aux.tar.gz",
    "a" * (MAX_LEN + 1),
]

fold_inputs = [
    "PRIVATE/M4ROOT/CLIP/C0001.MP4",
    "DCIM/100MSDCF/DSC00001.ARW",
    "CafÉ/X.JPG",
    "İstanbul",                    # İ: simple mapping is plain i
    "ΣΊΣΥΦΟΣ",  # ΣΊΣΥΦΟΣ: no final sigma
    "Straße",                      # ß is already lowercase
    "ǅ",                           # ǅ titlecase
    "K",                           # Kelvin sign
    "ÅNGSTRÖM",
]

path_key_inputs = [
    ("card", CARD, "DCIM/100MSDCF/DSC00001.ARW"),
    ("card", CARD2, "DCIM/100MSDCF/DSC00001.ARW"),
    ("card", CARD, "PRIVATE/M4ROOT/CLIP/C0001.MP4"),
    ("card", CARD, "private/M4ROOT/CLIP/C0001.MP4"),
    ("ssd", "", f"{SHOOT}/.originals/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW"),
    ("ssd", "", f"{SHOOT}/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW"),
    ("primary", "", f"{SHOOT}/Originals/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW"),
    ("backup", "", f"{SHOOT}/Originals/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW"),
    ("primary", "", "Café/DSC00001.JPG"),
    ("primary", "", "Café/DSC00001.JPG"),
    ("primary", "", "CAFÉ/dsc00001.jpg"),
    # rejected
    ("card", "", "DCIM/a.jpg"),
    ("ssd", CARD, "a.jpg"),
    ("archive", "", "a.jpg"),
    ("primary", "", "a/../b"),
]

md5_inputs = [
    {"text": ""},
    {"text": "abc"},
    {"text": "The quick brown fox jumps over the lazy dog"},
    {"text": "a", "repeat": 1_000_000},
    {"text": "été — \U0001f4f7"},
]

state_inputs = [
    (0, None),
    (0, {"primaryCopies": 1, "backupCopies": 1}),
    (10, None),
    (10, {"primaryCopies": 0, "backupCopies": 0}),
    (10, {"primaryCopies": 1, "backupCopies": 0}),
    (10, {"primaryCopies": 0, "backupCopies": 1}),
    (10, {"primaryCopies": 1, "backupCopies": 1}),
    (10, {"primaryCopies": 3, "backupCopies": 2}),
    (10, {"primaryCopies": -1, "backupCopies": 1}),
]


def main():
    out = {
        "about": "Golden vectors shared by the Go and Swift tests. Generated by testdata/make_vectors.py; don't edit by hand. Rules: docs/schema.md, Conventions.",
        "normalize": [
            {"in": p, "out": normalize(p)} if normalize(p) is not None else {"in": p, "error": True}
            for p in normalize_inputs
        ],
        "fold": [{"in": p, "out": fold(p)} for p in fold_inputs],
        "pathKey": [
            {"loc": loc, "scope": scope, "path": p, "key": path_key(loc, scope, p)}
            if path_key(loc, scope, p) is not None
            else {"loc": loc, "scope": scope, "path": p, "error": True}
            for loc, scope, p in path_key_inputs
        ],
        "md5": [
            {**m, "md5": hashlib.md5((m["text"] * m.get("repeat", 1)).encode()).hexdigest()}
            for m in md5_inputs
        ],
        "state": [{"size": size, "summary": summary, "state": state(size, summary)} for size, summary in state_inputs],
    }
    path = pathlib.Path(__file__).with_name("vectors.json")
    path.write_text(json.dumps(out, indent=2, ensure_ascii=True) + "\n")
    print(f"wrote {path}")


if __name__ == "__main__":
    main()
