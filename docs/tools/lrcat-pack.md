# lrcat-pack

Go, run by hand on Windows or Mac. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

Old Lightroom catalogs saved inside shoot folders turn into tens of thousands of small files, mostly preview caches. lrcat-pack turns each catalog into one zip file so backups and hashing stop crawling through them.

- **What it looks for:** each `<name>.lrcat` file, grouped with its sibling `<name> Previews.lrdata`, `<name> Smart Previews.lrdata` and other `<name> … .lrdata` folders, its side files (such as `-wal`, `-shm`) and its `Backups/` folder. This layout is from memory; confirm it against a real catalog first.
- **Skips catalogs that may be open or unclean:** a `.lock` or `-wal` file next to the catalog means Lightroom is using it or didn't close it cleanly. Those are reported, not packed.
- **Dry run by default:** lists every catalog found, with its file count and size, and changes nothing.
- **`--apply`:** writes `<name>.lrcat-pack.zip` next to the catalog on the primary disk. Preview files are stored as-is (they're already compressed JPEGs) and the catalog database is compressed. Any Mac or Windows machine can open the zip without special tools, and unzipping it restores the catalog exactly.
- **Verifies before removing anything:** it re-reads the zip and compares every entry's hash with the original file, then copies the zip to the backup disk and verifies that copy the same way.
- **Removes the loose files only after you confirm**, on both disks, and only for catalogs whose zips verified on both. This is the one place negatives deletes from an archive disk, which is why it's manual, confirmed and verified first.
- Run it once, before the first full hash, so the catalog never sees the loose catalog files. If it's ever run after cataloging has started, archive-hasher reports the removed files as unexpected disappearances. The online backup should still have the loose files as older versions.
- Option worth considering: previews are caches Lightroom can rebuild, so `--drop-previews` could leave them out of the zip entirely and shrink it a lot.
