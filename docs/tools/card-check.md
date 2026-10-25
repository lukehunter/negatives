# card-check

Read-only report. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Hashes every file on a mounted card, including video sidecar and index files, and reports each one as Safe, Pending or Not copied, plus one line: "safe to format" or not. Zero-byte files are listed as ignored and never block formatting; common files are folded into one summary line.
- Writes nothing except a `cards` sighting, so it's useful from day one against the existing archive.
- Offline: still hashes and reports "Not copied" vs "seen before", but never says "safe to format".
