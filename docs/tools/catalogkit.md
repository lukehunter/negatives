# CatalogKit

Shared Swift package. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Firestore access through the Firebase Apple SDK with offline persistence on, so writes queue on disk while offline and sync later.
- Sign-in with your Google account through Firebase Auth. The token lives in the Keychain, and Security Rules limit the Mac to its own fields.
- The document schema, path normalization (relative, `/`, NFC), streaming MD5, and the derived Safe / Pending / Not copied states.
- Staleness check: warns when `status/archive-hasher` shows the Windows watcher's heartbeat or nightly run is overdue.
- **Per-shoot locks:** an advisory file lock (`flock`) on `~/Library/Application Support/negatives/locks/<shoot>.lock`, outside the shoot folder so it's never archived. The operating system releases it automatically if a tool crashes. ingest, ssd-push, ssd-evict and ssd-restore take it before touching a shoot.
