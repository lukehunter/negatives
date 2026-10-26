# archive-hasher

Go, single binary. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Walks both disks and hashes any file whose path, size or mtime isn't in its local state file, skipping in-progress .partial files. Hashing reads local disks only.
- Skips zero-byte files entirely.
- Skips the backup disk's quarantine folder: files there are set aside partial copies, never archive copies.
- Skips each disk's identity file, `.negatives-disk`, at the root: it differs on each disk by design.
- Maintains each hash's summary document (first verified time and copy count per disk, number of distinct shoots, the `common` flag) from its local state.
- Publishes `primary` and `backup` facts to Firestore in batches of up to 500, using a service account key.
- Saves progress after each batch, so an interrupted run resumes instead of starting over.
- Reports conflicts (same path, new content) under Originals/ to `conflicts` and updates `status/archive-hasher` at the end of every run. Under Derived/, a changed file is a normal replacement: its fact moves from the old hash to the new one.
- Needs no offline queue: the disks are the source of truth, so a failed publish simply happens again next run.
- Hashes legacy roots like any other root, so card-check recognizes old cards whose files were archived before negatives existed. Files never disappear from the archive in normal operation, so a file that does is never silently dropped: its facts are kept but marked missing (so it's no longer Safe on that disk), and it's reported as an alert naming the files.
- **Facts for archive-sync's reconcile** (its `FactSource` interface, in `windows/internal/archivesync`): for a primary path, whether the copy as it is now (size and time) is verified and its MD5, whether that MD5 matches a card hash, whether the path ever held other content, and whether the backup file at the same path, as it is now, has content known elsewhere. See [archive-sync](archive-sync.md#reconcile).

## Decision core (`windows/internal/hasher`)

Pure functions, property-tested; the walking, hashing and publishing around them come next.

- **What to hash** (`NeedsHash`): a file with no entry, a new size or time, or one that was marked missing and is back. Zero-byte files never.
- **A new hash at a known path** (`Hashed`): the same content keeps its first-verified time. New content under `Originals/` or in a legacy folder (anything outside a negatives shoot) is a `content-changed` conflict; under `Derived/` or elsewhere in a negatives shoot it's a normal replacement. Either way the old hash is kept in the entry's history, which is what archive-sync's reconcile uses to tell that a primary path was edited in place, and the old content loses that copy, so it can stop being Safe. Nothing is fixed automatically.
- **Missing** (`MarkMissing`): a known file not found is marked missing only if the walk listed its whole top-level folder (or the disk root, for files there) without error. A folder that couldn't be read, or a run that stopped early, never makes anything missing.
- **Summaries** (`Summaries`): computed from the whole local state. `copies` counts only present copies; `firstVerifiedAt` is the earliest verification on that disk, kept even when the copy goes missing; `shoots` counts distinct top-level folders holding a present copy on either disk, and files at a disk root count toward none.
- **Publish only what changed** (`Updates`): only summaries that differ from the last published ones are written, plus, once, any hash the local state no longer holds at all (its only path got new content: a replaced Derived file, or an original edited in place), written with no copies on either disk. Without that, the catalog would keep showing a copy that's gone, and an edited original could still look Safe. A property test checks that after any sequence of runs the published copies equal the copies present on disk. A path fact is written only when it changes; its `seenAt` is the time of that change, not refreshed every run, since refreshing millions of facts every night would blow far past the free daily write quota.
- **Not yet:** `backup-mismatch` conflicts (a backup file whose hash never matches the primary at the same path, outside Derived) need a comparison across the two disks' entries; it belongs with the walk, which comes next.
- **Open:** archive-sync's `FactSource.Lookup` gets only the primary file's size and time. To say whether the backup file "as it is now" holds content known elsewhere, it also needs the backup file's size and time. Until the interface changes, the fact source should answer `BackupContentKnown` conservatively (true whenever any backup entry at that path has content known elsewhere), which can only keep a file in place, never move one.

