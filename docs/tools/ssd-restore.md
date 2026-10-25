# ssd-restore

Bring a shoot back to the SSD. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Run by hand when you want to edit an archived shoot again: `ssd-restore <shoot>`, or pick from a list. Needs a connection and the primary share.
- Checks free space first and copies nothing if the shoot won't fit.
- **New-layout shoots:** copies the originals from `<shoot>/Originals` into `Shoots/<shoot>/.originals/` (locked) and clones them into the shoot folder, then lays the latest files from `<shoot>/Derived` on top at the same paths. A rated JPEG comes back rated, a panorama comes back in `Exports/`.
- Always restores the archive's paths. If the shoot's folders were moved or renamed on the SSD before it was removed, that layout isn't recreated.
- **Legacy shoots:** the whole folder is treated as originals: copied into `Shoots/<shoot>/.originals/` and cloned into the shoot folder. Later edits are pushed to a `Derived/` folder inside the legacy shoot, with a `.negatives` marker. That's the only thing negatives ever adds to an old folder; the rest of it is never changed.
- Hashes every file as it lands on the SSD and compares it with the catalog's verified hash, so a bad read over SMB is caught. A mismatch is retried and then reported.
- Records `ssd` facts like ingest does. Restored files are already Safe, so ssd-push sends nothing until you change something, and ssd-evict can free the shoot again at any time.
- Holds the shoot's lock until the whole shoot is restored and recorded, so ssd-push never sees a half-restored shoot.
