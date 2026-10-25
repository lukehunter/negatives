# ingest

Card to SSD. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Picks the shoot folder (latest capture date + optional description) and the card folder (camera model + counter). A card re-inserted before formatting is recognized by its volume UUID; if it has both old and new files, ingest asks whether to continue the previous shoot or start a new one.
- Copies each new file from the card to `<shoot>/.originals/<card>/<path on card>`, keeping the card's structure and hashing as it reads. Files already ingested or archived are skipped.
- Locks each copy in `.originals/` with the user immutable flag (chflags uchg). No editing app ever opens `.originals/`.
- Clones each original into the same path in the shoot folder (`<shoot>/<card>/<path on card>`) with `cp -c`, with no plain-copy fallback: ingest checks the SSD is APFS before copying anything and stops if cloning isn't possible. Working copies are not locked, so apps can rate and write metadata freely.
- Records `cards` and `ssd` facts, including each working copy's size and mtime right after cloning.
- Writes only to the local SSD, never to the network.
- Checks free space before copying. If the card's new files won't fit, it copies nothing. It reports how much space the card needs and suggests running ssd-evict. ingest never deletes anything, on the card or the SSD.
