// archive-sync copies the primary archive disk to the backup disk, additive
// only. See docs/tools/archive-sync.md.
//
//	archive-sync [--config PATH] [--report]
//
// Exits 0 on success and 1 on any failure, for Task Scheduler.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/lukehunter/negatives/windows/internal/archivesync"
	"github.com/lukehunter/negatives/windows/internal/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	configPath := flag.String("config", config.DefaultPath, "config file")
	report := flag.Bool("report", false, "compare the disks and list what a run would copy, without copying or moving anything")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: archive-sync [--config PATH] [--report]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() > 0 {
		flag.Usage()
		return 1
	}

	cfg, err := config.Load(*configPath)
	if err == nil {
		err = cfg.Validate()
	}
	if err == nil {
		err = cfg.ValidateDisks(config.OSEnv())
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "archive-sync: config:", err)
		return 1
	}

	// Next to the config file, never on an archive disk.
	lock, err := archivesync.AcquireLock(filepath.Join(filepath.Dir(*configPath), "archive-sync.lock"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "archive-sync:", err)
		return 1
	}
	defer lock.Release()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	res, err := archivesync.Run(ctx, archivesync.Options{
		Primary:    cfg.Disks.Primary,
		Backup:     cfg.Disks.Backup,
		Quarantine: cfg.Disks.Quarantine,
		LogDir:     cfg.Logs.Dir,
		Report:     *report,
		// Until archive-hasher exists (build step 3) nothing is verified,
		// so partial copies are reported and kept, never quarantined.
		Facts:  archivesync.NoFacts{},
		Runner: archivesync.ExecRunner{},
		Out:    os.Stdout,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "archive-sync:", err)
		return 1
	}
	if !res.OK() {
		return 1
	}
	return 0
}
