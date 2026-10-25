# Windows tools

Go code for the tools that run on the Windows machine holding the archive
disks. So far: [archive-sync](../docs/tools/archive-sync.md) (build order
step 2).

- `cmd/archive-sync/`: the command. `archive-sync [--config PATH] [--report]`;
  the config defaults to `C:\ProgramData\negatives\config.toml`
  ([docs/config.md](../docs/config.md)).
- `internal/archivesync/`: the decisions as pure functions (`layout.go`,
  `plan.go`, `robocopy.go`, `reconcile.go`), the read-only disk walk
  (`scan.go`), and the run that carries them out (`sync.go`), with the lock
  and the no-overwrite move.
- `internal/config/`: reads and validates the Windows config file.

## Build

```bash
GOOS=windows GOARCH=amd64 go build -o archive-sync.exe ./cmd/archive-sync
```

## Test

```bash
go test ./...
```

runs everywhere (macOS, Linux, Windows), using temporary folders as the two
disks and a small model of robocopy. On Windows,

```bash
go test -tags robocopy -v ./internal/archivesync -run Robocopy
```

also runs the real robocopy: what `/XC /XN /XO` do, and what an interrupted
copy leaves behind. The manual checks on the Windows host are in
[archive-sync-manual-tests.md](../docs/tools/archive-sync-manual-tests.md).
