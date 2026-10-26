// archive-sync copies the primary archive disk to the backup disk, additive
// only. See docs/tools/archive-sync.md.
//
//	archive-sync [--config PATH] [--report]
//
// Exits 0 on success and 1 on any failure, for Task Scheduler. A run that
// finds another run in progress does nothing and exits 0.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
	"github.com/lukehunter/negatives/windows/internal/config"
)

// deps is what run uses from the outside world; tests replace it.
type deps struct {
	env    config.Env
	runner archivesync.Runner
	facts  archivesync.FactSource
	// fallbackLogDir gets errors that happen before the config's logs
	// folder is known, or when the config itself is wrong.
	fallbackLogDir string
	now            func() time.Time
	stdout, stderr io.Writer
}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], deps{
		env:    config.OSEnv(),
		runner: archivesync.ExecRunner{},
		// Until archive-hasher exists (build step 3) nothing is verified,
		// so partial copies are reported and kept, never quarantined.
		facts:          archivesync.NoFacts{},
		fallbackLogDir: config.Defaults().Logs.Dir,
		now:            time.Now,
		stdout:         os.Stdout,
		stderr:         os.Stderr,
	}))
}

func run(ctx context.Context, args []string, d deps) int {
	fs := flag.NewFlagSet("archive-sync", flag.ContinueOnError)
	fs.SetOutput(d.stderr)
	configPath := fs.String("config", config.DefaultPath, "config file")
	report := fs.Bool("report", false, "compare the disks and list what a run would copy, without copying or moving anything")
	fs.Usage = func() {
		fmt.Fprintln(d.stderr, "usage: archive-sync [--config PATH] [--report]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		if err == nil {
			fs.Usage()
		}
		return 1
	}

	// Printed and logged: under Task Scheduler nobody sees the console.
	fail := func(logDir, msg string) int {
		fmt.Fprintln(d.stderr, "archive-sync:", msg)
		if err := archivesync.LogError(logDir, d.now(), msg); err != nil {
			fmt.Fprintln(d.stderr, "archive-sync: also failed to write the log:", err)
		}
		return 1
	}

	cfg, err := config.Load(*configPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err == nil {
		err = cfg.ValidateDisks(d.env)
	}
	if err != nil {
		// Not the config's logs folder: the config is what's wrong, and a
		// bad logs.dir could even point at an archive disk.
		return fail(d.fallbackLogDir, "config: "+err.Error())
	}

	// Next to the config file, never on an archive disk.
	lock, err := archivesync.AcquireLock(filepath.Join(filepath.Dir(*configPath), "archive-sync.lock"))
	if errors.Is(err, archivesync.ErrLocked) {
		// The watcher's run and the nightly run colliding is normal: the
		// run in progress does the work, so this isn't a failure.
		msg := "another archive-sync run is in progress; this one does nothing"
		fmt.Fprintln(d.stdout, msg)
		if err := archivesync.LogInfo(cfg.Logs.Dir, d.now(), msg); err != nil {
			fmt.Fprintln(d.stderr, "archive-sync: failed to write the log:", err)
		}
		return 0
	}
	if err != nil {
		return fail(cfg.Logs.Dir, err.Error())
	}
	defer lock.Release()

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	res, err := archivesync.Run(ctx, archivesync.Options{
		Primary:    cfg.Disks.Primary,
		Backup:     cfg.Disks.Backup,
		PrimaryID:  cfg.Disks.PrimaryID,
		BackupID:   cfg.Disks.BackupID,
		Quarantine: cfg.Disks.Quarantine,
		LogDir:     cfg.Logs.Dir,
		Report:     *report,
		Facts:      d.facts,
		Runner:     d.runner,
		Now:        d.now,
		Out:        d.stdout,
	})
	if err != nil {
		// Run has logged it already, if it got as far as opening the log.
		fmt.Fprintln(d.stderr, "archive-sync:", err)
		return 1
	}
	if !res.OK() {
		return 1
	}
	return 0
}
