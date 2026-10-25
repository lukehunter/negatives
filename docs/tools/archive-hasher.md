# archive-hasher

Go, single binary. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Walks both disks and hashes any file whose path, size or mtime isn't in its local state file, skipping in-progress .partial files. Hashing reads local disks only.
- Publishes `primary` and `backup` facts to Firestore in batches of up to 500, using a service account key.
- Saves progress after each batch, so an interrupted run resumes instead of starting over.
- Reports conflicts (same path, new content) under Originals/ to `conflicts` and updates `status/archive-hasher` at the end of every run. Under Derived/, a changed file is a normal replacement: its fact moves from the old hash to the new one.
- Needs no offline queue: the disks are the source of truth, so a failed publish simply happens again next run.
- Hashes legacy roots like any other root, so card-check recognizes old cards whose files were archived before negatives existed. A file that disappears from a root (for example after lrcat-pack) has its fact removed.
