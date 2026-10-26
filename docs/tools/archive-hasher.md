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
