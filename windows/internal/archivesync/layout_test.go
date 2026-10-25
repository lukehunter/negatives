package archivesync

import "testing"

func TestClassify(t *testing.T) {
	shoots := Shoots{Fold("2026-10-18 Coast"): true}
	cases := []struct {
		rel  string
		want Kind
	}{
		{"desktop.ini", KindLegacy},
		{"2019-05-04 Beach/DSC00001.ARW", KindLegacy},
		{"2019-05-04 Beach/Originals/a.ARW", KindLegacy}, // unmarked: legacy whatever it's called
		{"2019-05-04 Beach/Derived/a.jpg", KindLegacy},
		{"2026-10-18 Coast/.negatives", KindShootOther},
		{"2026-10-18 Coast/Originals", KindShootOther}, // a file named Originals
		{"2026-10-18 Coast/Originals/ILCE-7M4-01/DCIM/100MSDCF/DSC09998.ARW", KindOriginal},
		{"2026-10-18 coast/originals/x.ARW", KindOriginal}, // case-insensitive
		{"2026-10-18 Coast/Derived/Exports/coast-pano.tif", KindDerived},
		{"2026-10-18 Coast/DERIVED/x.xmp", KindDerived},
		{"2026-10-18 Coast/Notes/x.txt", KindShootOther},
	}
	for _, c := range cases {
		if got := Classify(c.rel, shoots); got != c.want {
			t.Errorf("Classify(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
}

func TestIsDerivedDir(t *testing.T) {
	shoots := Shoots{Fold("S"): true}
	for rel, want := range map[string]bool{
		"S/Derived": true, "s/derived": true, "S/Derived/x": false, "S": false,
		"S/Originals": false, "Legacy/Derived": false, "S/x/Derived": false,
	} {
		if got := IsDerivedDir(rel, shoots); got != want {
			t.Errorf("IsDerivedDir(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestSkipTop(t *testing.T) {
	for name, want := range map[string]bool{
		"$RECYCLE.BIN": true, "$Recycle.Bin": true, "System Volume Information": true,
		"_quarantine": true, "_QUARANTINE": true, "2026-10-18 Coast": false, "_quarantined": false,
	} {
		if got := SkipTop(name, "_quarantine"); got != want {
			t.Errorf("SkipTop(%q) = %v, want %v", name, got, want)
		}
	}
	if SkipTop("anything", "") {
		t.Error("an empty quarantine name must not match")
	}
}

func TestIsPartial(t *testing.T) {
	for name, want := range map[string]bool{
		"C0001.MP4.partial": true, "x.PARTIAL": true, "partial": false, "x.partial.jpg": false,
	} {
		if got := IsPartial(name); got != want {
			t.Errorf("IsPartial(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestValidRel(t *testing.T) {
	for rel, want := range map[string]bool{
		"a": true, "a/b c/d.ARW": true, "": false, "/a": false, "a/": false, "a//b": false,
		"a/./b": false, "a/../b": false, "..": false, `a\b`: false, "C:/a": false,
	} {
		if got := ValidRel(rel); got != want {
			t.Errorf("ValidRel(%q) = %v, want %v", rel, got, want)
		}
	}
}
