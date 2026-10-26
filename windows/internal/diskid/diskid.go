// Package diskid reads and creates the identity file at the root of each
// archive disk, .negatives-disk, which says whether the disk is the primary
// or the backup. archive-sync checks it before every run, so swapped drive
// letters, a cloned disk or a replaced disk can never make it copy the
// backup onto the primary. See docs/tools/archive-sync.md.
package diskid

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/BurntSushi/toml"
)

// FileName is the identity file at a disk's root, named in the style of the
// .negatives shoot marker.
const FileName = ".negatives-disk"

// Roles a disk can have.
const (
	Primary = "primary"
	Backup  = "backup"
)

// Identity is the identity file's contents.
type Identity struct {
	Role    string    `toml:"role"`
	ID      string    `toml:"id"`
	Created time.Time `toml:"created"`
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ValidRole reports whether role is primary or backup.
func ValidRole(role string) bool { return role == Primary || role == Backup }

// ValidID reports whether id looks like an ID that Create makes (a random
// UUID, lowercase).
func ValidID(id string) bool { return idPattern.MatchString(id) }

// Read reads the identity file at the root of the disk.
func Read(root string) (Identity, error) {
	p := filepath.Join(root, FileName)
	b, err := os.ReadFile(p)
	if err != nil {
		return Identity{}, err
	}
	var id Identity
	md, err := toml.Decode(string(b), &id)
	if err != nil {
		return Identity{}, fmt.Errorf("%s: %w", p, err)
	}
	if und := md.Undecoded(); len(und) > 0 {
		return Identity{}, fmt.Errorf("%s: unknown field %s", p, und[0])
	}
	if !ValidRole(id.Role) || !ValidID(id.ID) {
		return Identity{}, fmt.Errorf("%s: role %q, id %q: not a valid identity file", p, id.Role, id.ID)
	}
	return id, nil
}

// Check confirms that the disk at root is the one the config names: its
// identity file exists, has the expected role, and has the expected ID.
func Check(root, role, wantID string) error {
	if wantID == "" {
		return fmt.Errorf("no %s disk ID in the config: run disk-id show %s and add it as disks.%s_id", role, root, role)
	}
	id, err := Read(root)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s has no %s file, so it can't be confirmed as the %s disk", root, FileName, role)
	}
	if err != nil {
		return err
	}
	if id.Role != role {
		return fmt.Errorf("%s is the %s disk (its %s says so), but the config uses it as the %s: were drive letters swapped?",
			root, id.Role, FileName, role)
	}
	if id.ID != wantID {
		return fmt.Errorf("%s is a %s disk with ID %s, but the config expects %s: is this a different or replaced disk?",
			root, role, id.ID, wantID)
	}
	return nil
}

// Create writes a new identity file at the root of the disk. It refuses if
// one is already there, so it can never change an existing disk's role or
// ID. Only run by hand (disk-id create), or by negatives config init.
func Create(root, role string, now time.Time) (Identity, error) {
	if !ValidRole(role) {
		return Identity{}, fmt.Errorf("role %q: must be %s or %s", role, Primary, Backup)
	}
	st, err := os.Stat(root)
	if err != nil {
		return Identity{}, err
	}
	if !st.IsDir() {
		return Identity{}, fmt.Errorf("%s is not a folder", root)
	}
	id := Identity{Role: role, ID: newID(), Created: now.UTC().Truncate(time.Second)}
	p := filepath.Join(root, FileName)
	// O_EXCL: fails if the file exists, never overwrites.
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		return Identity{}, fmt.Errorf("%s already exists; an existing disk identity is never changed", p)
	}
	if err != nil {
		return Identity{}, err
	}
	_, err = fmt.Fprintf(f, "# negatives archive disk identity. Don't edit, copy or delete this file.\n"+
		"role = %q\nid = %q\ncreated = %s\n", id.Role, id.ID, id.Created.Format(time.RFC3339))
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Identity{}, fmt.Errorf("writing %s: %w", p, err)
	}
	return id, nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand doesn't fail on supported systems
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
