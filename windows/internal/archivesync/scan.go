package archivesync

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
)

// Everything in this file only reads. It lists folders and reads file sizes
// and times; it never opens a file's contents and never writes.

// ListRoot lists the primary disk's top-level entries and, for each folder,
// whether it holds a .negatives marker and what its Derived folder is
// called.
func ListRoot(primary string) ([]RootEntry, error) {
	des, err := os.ReadDir(primary)
	if err != nil {
		return nil, err
	}
	var out []RootEntry
	for _, de := range des {
		e := RootEntry{Name: de.Name(), IsDir: de.IsDir()}
		if e.IsDir {
			dir := filepath.Join(primary, de.Name())
			if st, err := os.Lstat(filepath.Join(dir, MarkerName)); err == nil && st.Mode().IsRegular() {
				e.Marked = true
			}
			if e.Marked {
				sub, err := os.ReadDir(dir)
				if err != nil {
					return nil, err
				}
				for _, s := range sub {
					if s.IsDir() && Fold(s.Name()) == Fold(DerivedDir) {
						e.DerivedName = s.Name()
					}
				}
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// ShootsOf returns the negatives shoots among the root entries.
func ShootsOf(entries []RootEntry) Shoots {
	s := Shoots{}
	for _, e := range entries {
		if e.IsDir && e.Marked {
			s[Fold(e.Name)] = true
		}
	}
	return s
}

// Found is one path the walk looked at, with its verdict.
type Found struct {
	Pair
	Verdict Verdict
	Facts   Facts
	Err     error // reading this path failed
}

// WalkInput is everything Walk needs.
type WalkInput struct {
	Primary, Backup string
	Shoots          Shoots
	QuarantineName  string
	// Lookup asks the fact source about a primary copy. It's called only
	// for smaller backup copies.
	Lookup func(rel string, primary FileInfo) (Facts, error)
}

// Walk compares the two disks folder by folder, reading each folder's
// listing on both disks once, and calls visit for every path that isn't
// identical on both. Same files are counted, not visited. A folder missing
// from the backup is reported once, not file by file, and not descended.
// Derived folders of negatives shoots are skipped: pass 2 handles them.
func Walk(in WalkInput, visit func(Found)) (same int, err error) {
	var walk func(rel string) error
	walk = func(rel string) error {
		pdir := filepath.Join(in.Primary, filepath.FromSlash(rel))
		bdir := filepath.Join(in.Backup, filepath.FromSlash(rel))
		pents, err := os.ReadDir(pdir)
		if err != nil {
			return err
		}
		bents, err := os.ReadDir(bdir)
		if err != nil {
			return err
		}
		backup := make(map[string]fs.DirEntry, len(bents))
		for _, b := range bents {
			backup[Fold(b.Name())] = b
		}
		for _, p := range pents {
			name := p.Name()
			child := path.Join(rel, name)
			b, onBackup := backup[Fold(name)]
			delete(backup, Fold(name))
			if rel == "" && SkipTop(name, in.QuarantineName) {
				continue
			}
			kind := Classify(child, in.Shoots)
			switch {
			case p.Type()&fs.ModeType != 0 && !p.IsDir():
				visit(Found{Pair: Pair{Rel: child, Kind: kind}, Verdict: NotRegular})
			case p.IsDir():
				switch {
				case IsDerivedDir(child, in.Shoots):
					// pass 2
				case !onBackup:
					visit(Found{Pair: Pair{Rel: child, Kind: kind}, Verdict: MissingDir})
				case !b.IsDir():
					visit(Found{Pair: Pair{Rel: child, Kind: kind}, Verdict: TypeMismatch})
				default:
					if err := walk(child); err != nil {
						visit(Found{Pair: Pair{Rel: child, Kind: kind}, Err: err})
					}
				}
			case IsPartial(name):
				visit(Found{Pair: Pair{Rel: child, Kind: kind}, Verdict: Replaceable})
			default:
				pi, err := infoOf(p)
				if err != nil {
					visit(Found{Pair: Pair{Rel: child, Kind: kind}, Err: err})
					continue
				}
				pair := Pair{Rel: child, Kind: kind, Primary: pi}
				if onBackup {
					if b.IsDir() || b.Type()&fs.ModeType != 0 {
						// A folder, link or junction where the primary has
						// a file.
						pair.BackupIsDir = true
					} else if bi, err := infoOf(b); err != nil {
						visit(Found{Pair: pair, Err: err})
						continue
					} else {
						pair.Backup = &bi
					}
				}
				f := Found{Pair: pair}
				if NeedsFacts(pair) {
					f.Facts, f.Err = in.Lookup(child, pi)
				}
				f.Verdict = Decide(pair, f.Facts)
				if f.Verdict == Same && f.Err == nil {
					same++
					continue
				}
				visit(f)
			}
		}
		// Whatever is left is only on the backup disk.
		var extra []string
		for _, b := range backup {
			if rel == "" && SkipTop(b.Name(), in.QuarantineName) {
				continue
			}
			extra = append(extra, b.Name())
		}
		sort.Strings(extra)
		for _, name := range extra {
			child := path.Join(rel, name)
			visit(Found{Pair: Pair{Rel: child, Kind: Classify(child, in.Shoots)}, Verdict: Extra})
		}
		return nil
	}
	return same, walk("")
}

// Stat reads one file's size and time. It reports a folder or a link as an
// error, since only regular files are ever compared or moved.
func Stat(p string) (FileInfo, error) {
	st, err := os.Lstat(p)
	if err != nil {
		return FileInfo{}, err
	}
	if !st.Mode().IsRegular() {
		return FileInfo{}, errors.New(p + ": not a regular file")
	}
	return FileInfo{Size: st.Size(), MtimeNs: st.ModTime().UnixNano()}, nil
}

func infoOf(de fs.DirEntry) (FileInfo, error) {
	st, err := de.Info()
	if err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Size: st.Size(), MtimeNs: st.ModTime().UnixNano()}, nil
}
