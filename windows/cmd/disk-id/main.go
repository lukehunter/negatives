// disk-id creates and shows the identity file (.negatives-disk) at the
// root of an archive disk. archive-sync refuses to run unless both disks
// carry one matching the config, and never creates them itself.
//
//	disk-id create --role primary|backup <disk root>
//	disk-id show <disk root>
//
// Run create once per disk at setup, and for a replacement backup disk;
// negatives config init will do the same. It never overwrites an existing
// identity file.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lukehunter/negatives/windows/internal/diskid"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	usage := func() int {
		fmt.Fprintln(stderr, "usage: disk-id create --role primary|backup <disk root>\n       disk-id show <disk root>")
		return 1
	}
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "create":
		fs := flag.NewFlagSet("create", flag.ContinueOnError)
		fs.SetOutput(stderr)
		role := fs.String("role", "", "primary or backup")
		if err := fs.Parse(args[1:]); err != nil || fs.NArg() != 1 {
			return usage()
		}
		root := fs.Arg(0)
		id, err := diskid.Create(root, *role, time.Now())
		if err != nil {
			fmt.Fprintln(stderr, "disk-id:", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s is now the %s disk. Add this to [disks] in config.toml:\n%s_id = %q\n", root, id.Role, id.Role, id.ID)
		return 0
	case "show":
		if len(args) != 2 {
			return usage()
		}
		id, err := diskid.Read(args[1])
		if err != nil {
			fmt.Fprintln(stderr, "disk-id:", err)
			return 1
		}
		fmt.Fprintf(stdout, "%s: %s disk, created %s\n%s_id = %q\n", args[1], id.Role, id.Created.Format(time.RFC3339), id.Role, id.ID)
		return 0
	}
	return usage()
}
