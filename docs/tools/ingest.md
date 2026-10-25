# ingest

Card to SSD. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Picks the shoot folder (latest capture date + optional description) and the card folder (camera name from `cameraNames`, or the EXIF model, plus a counter). A card re-inserted before formatting is recognized by its volume UUID; if it has both old and new files, ingest asks whether to continue the previous shoot or start a new one.
- Reads each file from the card exactly once. Files the card's session already knows (same path, size and mtime, from cached hashes) are skipped without being read.
- Every other file is written to a temporary `.partial` file inside the shoot's `.originals/` while it's hashed. When the read finishes:
  - new content is renamed to `<shoot>/.originals/<card>/<path on card>`, keeping the card's structure;
  - content that's already archived, or already on the SSD in another shoot, is discarded and reported as already archived.
- Holding files in memory instead was considered and rejected: video clips can be larger than the Mac's memory, and temporary files keep reading and writing overlapped. Discarded duplicates only happen with an unrecognized old card, which is rare.
- An old card whose files are all archived copies nothing; ingest says so and points to ssd-restore for editing that shoot again.
- Zero-byte files are copied to keep the card's structure intact but aren't cataloged.
- Locks each copy in `.originals/` with the user immutable flag (chflags uchg). No editing app ever opens `.originals/`.
- Clones each original into the same path in the shoot folder (`<shoot>/<card>/<path on card>`) with `cp -c`, with no plain-copy fallback: ingest checks the SSD is APFS before copying anything and stops if cloning isn't possible. Working copies are not locked, so apps can rate and write metadata freely.
- Records `cards` and `ssd` facts, including each working copy's size and mtime right after cloning.
- Writes only to the local SSD, never to the network.
- Checks free space before copying. If the card's new files won't fit, it copies nothing. It reports how much space the card needs and suggests running ssd-evict. ingest never deletes anything except its own unfinished temporary files: never anything on the card, existing originals, or your work.
- Holds the shoot's lock while copying into it, so ssd-push never pushes from a shoot mid-ingest.
- On start, removes any `.partial` files it left behind from an interrupted run before copying again.
