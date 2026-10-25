# negatives

Tools that tell a photographer when a memory card is safe to format, keep a
working SSD for editing, and back everything up to two archive disks, with
every copy verified by MD5 hash.

Status: design only, no code yet. Start with the build order in
`docs/design.md`.

## Docs

- `docs/design.md`: the core design (source of truth for decisions): data
  model, folder layout, safety rules, build order.
- `docs/schema.md`: field-level catalog schema, path rules and who may write
  what; the contract between the Swift and Go code. Rules are in
  `firestore/firestore.rules`.
- `docs/config.md`: local TOML files, shared settings in Firestore
  (`config/settings`), and the `negatives config` command.
- `docs/testing.md`: invariants, test layers, fixtures (card and SSD disk
  images), failure injection, CI. Shared golden vectors keep Swift and Go in
  agreement.
- `docs/tools/*.md`: one spec per tool. Read the relevant one, plus the core
  design, before working on a tool.
- `docs/user-guide.md`: how the finished tools are installed and used.
- Both were exported on 2026-10-24 from Claude Docs, where they were written:
  design https://claude.ai/artifact/HkVKdewVEUbnjE9YWkYQSC,
  user guide https://claude.ai/artifact/9VRFCXy1DtmpM1Mng7CkvR.
  If you change a decision, update `docs/design.md` here; treat the repo
  copy as current. Its system overview is a Mermaid diagram.

## Setup this is for

- Mac (Apple Silicon) for editing, with an APFS working SSD.
- Windows machine at home hosting the primary and backup archive disks
  (plain file copies, no RAID), shared to the Mac over SMB. Usually on,
  not always.
- Existing online backup software watches the archive; it is hash-based,
  so moving or renaming files doesn't re-upload them.
- Archive: millions of files, tens of terabytes. Photo and video from
  several camera brands (mostly Sony).
- Editing apps: Lightroom (rarely now), Adobe Bridge, DaVinci Resolve,
  Sony Catalyst, Gyroflow. Bridge writes ratings into JPEGs.

## Key decisions (details and reasons in docs/design.md)

- Identity is the file's MD5. Catalog is Firestore: a summary doc per hash
  plus a `paths` subcollection doc per copy; append-only facts written with
  set-with-merge, no transactions. Zero-byte files are never cataloged.
- Mac tools in Swift (Firebase Apple SDK, offline write queue, sign-in with
  the user's Google account). Windows tools in Go plus PowerShell/robocopy,
  using a service account key (no personal Google credentials on Windows).
- Mac SSD: each shoot is one folder under `Shoots/`. A hidden `.originals/`
  inside it holds pristine originals locked with `chflags uchg`; the card
  folders beside it hold APFS clones (`cp -c`) to edit. No plain-copy fallback:
  ingest refuses non-APFS volumes.
- ingest writes only to the local SSD. ssd-push copies to the primary disk
  in the background. Card structure (DCIM, PRIVATE/M4ROOT) is kept inside a
  per-card folder named `<camera model>-<nn>`; shoot folder is
  `<latest capture date> <optional description>`.
- Archive: each shoot folder at the disk root holds `Originals/` (never
  overwritten) and `Derived/` (replaceable), plus a `.negatives` marker.
  Unmarked folders are legacy: never moved or overwritten. On the SSD, legacy
  shoot folders (no `.originals/`) sit beside new ones; ssd-push archives them
  by path, which today matches between the SSD and the archive.
- Existing folders in a shoot must never be moved or renamed (new folders are
  fine). If they are, nothing is copied twice: the catalog keeps files Safe by
  hash, and the user is notified that paths differ.
- Windows runs are triggered by writes (watcher, 10 quiet minutes), plus a
  nightly safety-net run. Avoid spinning disks unnecessarily.
- Nothing frees SSD space automatically: ssd-evict is user-run, works on
  whole shoot folders, re-hashes before deleting.
- A card is safe to format only when every file is verified on both disks,
  checked online.

## Working preferences

- Keep each component small and single-purpose; tool names as in the design.
- Safety over convenience: nothing deletes automatically, and any doubt
  means "not safe yet".
- Explain anything that could delete or overwrite data, and how it's kept safe.
- Write core decisions (Safe, evict, push, safe to format) as pure functions
  so they can be property-tested from day one.

## To verify early

- Firestore offline writes on macOS (build order step 1 spike).
- How robocopy handles an interrupted copy with /XC /XN /XO.
- Lightroom catalog file layout (for lrcat-pack) and whether Catalyst
  metadata edits rewrite card XML files.
