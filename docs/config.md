# Configuration

How negatives is configured on each machine. Part of [negatives](design.md).

**Contents**

- [Where settings live](#where-settings-live)
- [Mac config file](#mac-config-file)
- [Windows config file](#windows-config-file)
- [Shared settings](#shared-settings)
- [The config command](#the-config-command)
- [Validation](#validation)

## Where settings live

| Kind | Where | Examples |
| --- | --- | --- |
| Defaults | Built into the code and listed below | Everything has one |
| Machine-specific | A TOML file on each machine | Paths, key location, run times |
| Shared tunables | One Firestore document, `config/settings` | Settling time, exclude list |
| Fixed | Code only, on purpose | Path rules, schema version, robocopy flags |

- A setting lives in exactly one place: machine-specific keys only in the local file, shared keys only in Firestore. Nothing overrides anything else, so the two machines can never drift apart on a shared value.
- Missing values use the defaults.
- Secrets never go in config files or the catalog. The Windows file only points to the service account key.
- Offline, the Mac uses its cached copy of the shared settings. If Windows can't reach Firestore, it uses the last copy it saved locally, or the defaults.

## Mac config file

`~/Library/Application Support/negatives/config.toml`

```toml
[ssd]
shoots = "/Volumes/EditSSD/Shoots"      # the Shoots folder on the APFS working SSD

[archive]
share = "/Volumes/Archive"              # where the primary disk's share is mounted

[firebase]
config = "~/Library/Application Support/negatives/GoogleService-Info.plist"

[notifications]
card_summary = true                     # card-watch summaries
paths_differ = true                     # ssd-push: SSD paths no longer match the archive
```

## Windows config file

`C:\ProgramData\negatives\config.toml`. Windows paths use single-quoted TOML strings, which don't treat `\` as an escape.

```toml
[disks]
primary = 'P:\'
backup = 'B:\'
quarantine = 'B:\_quarantine'           # must be on the backup disk

[gcp]
project = "negatives-archive"
key = 'C:\ProgramData\negatives\service-account.json'

[schedule]
nightly = "03:00"                       # local time of the nightly safety-net run

[scrub]
budget_minutes = 45                     # time archive-scrub may spend each night
smart_disks = ["auto"]                  # or explicit disk numbers

[logs]
dir = 'C:\ProgramData\negatives\logs'
```

## Shared settings

The Firestore document `config/settings`. Each tool reads it at the start of every run, so a change takes effect on the next run on both machines.

| Field | Default | Used by |
| --- | --- | --- |
| `derivedSettleMinutes` | 60 | ssd-push: how long a derived file must be unchanged before it's pushed |
| `watcherQuietMinutes` | 10 | Windows watcher: quiet time after writes before a run |
| `commonShootThreshold` | 10 | archive-hasher: shoots before content is marked common |
| `heartbeatStaleHours` | 4 | Mac: warn when the Windows heartbeat is older than this |
| `nightlyStaleHours` | 36 | Mac: warn when the last full run is older than this |
| `backupStaleDays` | 14 | Mac: warn when the last catalog export is older than this |
| `exclude` | See [ssd-push](tools/ssd-push.md) | ssd-push: glob patterns never pushed (caches, proxies, `.DS_Store`, …) |

The document also carries `schema`, `updatedAt` and `updatedBy`.

## The config command

Both machines have a `negatives config` command (Swift on the Mac, Go on Windows) so you rarely edit anything by hand:

- `negatives config init`: walks through first-time setup. It suggests values it can detect (mounted volumes, which disk is APFS, drive letters), writes a commented `config.toml`, and validates it.
- `negatives config show`: prints every effective setting and where it came from: default, local file or shared.
- `negatives config set <key> <value>`: changes one setting. Local keys are edited in place in `config.toml`, keeping its comments; shared keys are written to Firestore.
- `negatives config exclude add|remove|list <pattern>`: manages the shared exclude list.
- `negatives config validate`: runs the checks below without doing anything else.

Hand-editing the TOML file is always fine too; every tool validates it on start.

## Validation

Every tool checks its configuration on start and refuses to run, with a clear message, if something is wrong:

- **Mac:** the `Shoots` folder exists and is on an APFS volume; the share path is a mounted network volume (when a tool needs it); the Firebase config file exists.
- **Windows:** primary and backup are different disks and both exist; the quarantine folder is on the backup disk; the key file exists and is readable only by the account running the task; the nightly time parses.
- **Shared:** numbers are positive and in a sensible range; exclude patterns parse and never match `.originals/`, which must always be pushed.
