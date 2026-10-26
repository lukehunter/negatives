# Catalog schema and security

The field-level contract between the Mac tools (Swift) and the Windows tools (Go). Both sides must follow it exactly; when in doubt, this file wins. Part of [negatives](design.md); the concepts behind it are in the [data model](design.md#data-model).

**Contents**

- [Conventions](#conventions)
- [Collections](#collections)
- [Who can write what](#who-can-write-what)
- [Security rules](#security-rules)
- [Changing the schema](#changing-the-schema)

## Conventions

**File identity**

- `md5`: lowercase hex MD5 of the file's full contents (32 characters). Zero-byte files are never cataloged.

**Paths**

- Stored relative to a root: the card's root, the SSD's `Shoots/` folder, or the archive disk's root. The first segment of an SSD or archive path is the shoot folder.
- `/` separators, Unicode NFC, case preserved exactly as the file system reports it.
- Paths are **compared case-insensitively**, because APFS, exFAT and NTFS are all case-insensitive by default: `private/M4ROOT` and `PRIVATE/M4ROOT` are the same folder (the sample cards show both). The stored `path` keeps its original case; `pathKey` and every comparison use the case-folded path.
- **Case folding** is exact, so Swift and Go agree: map each code point to its Unicode *simple* lowercase mapping (one code point to one, no context), then apply NFC. So `İ` (U+0130) becomes a plain `i`, and a final `Σ` becomes `σ`, never `ς`. Swift's `lowercased()` uses the full, context-sensitive mapping, so the Swift code maps per scalar and special-cases U+0130; Go's `unicode.ToLower` is already the simple mapping.
- Rejected (tools refuse to create or record them): empty, `.` or `..` segments; leading, trailing or doubled `/`; control characters (U+0000–U+001F, U+007F); `: * ? " < > |` and backslash; segments ending in a space or period; Windows reserved names (`CON`, `PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`, in any case, with or without an extension: `con.txt` and `aux.tar.gz` too); more than 1024 code points after NFC.
- Shared code: Go in `windows/internal/catalog`; the Swift side will mirror it. Both are tested against the golden vectors in `testdata/vectors.json`, written by `testdata/make_vectors.py`, an independent reference implementation of these rules.

**Keys**

- `pathKey`: the first 16 hex characters (lowercase) of SHA-256 over the UTF-8 bytes of `loc`, a NUL byte, the scope, a NUL byte and the normalized, case-folded path. The scope is the `cardKey` for card copies and empty otherwise; a card copy without a `cardKey`, or any other copy with a scope, is an error. Each copy gets a stable document ID; collisions are negligible within one hash.
- `cardKey`: the card's volume UUID, lowercase.

**Values**

- Event times (`seenAt`, `verifiedAt`, …) are Firestore Timestamps in UTC, taken from the writer's clock when the event happened, so writes queued offline keep their real times.
- `mtimeNs`: the file's modification time in integer nanoseconds since the Unix epoch, as the local file system reports it. It's only ever compared with an earlier reading from the same machine, never across machines.
- Sizes are integer bytes.
- Every document carries `schema: 1`. Readers ignore fields they don't know.

## Collections

**`files/{md5}`**: summary, one per unique content

| Field | Type | Meaning |
| --- | --- | --- |
| `schema` | int | Always 1 |
| `size` | int | Bytes, greater than 0 |
| `primary` | map | `{ firstVerifiedAt: timestamp, copies: int }`: verified copies on the primary disk |
| `backup` | map | Same, for the backup disk |
| `shoots` | int | Distinct shoot folders the content appears in on the archive |
| `common` | bool | `shoots` is at or above the common threshold (default 10) |
| `updatedAt` | timestamp | Last change by archive-hasher |

A file is **Safe** when `primary.copies > 0` and `backup.copies > 0`. Only archive-hasher writes `primary`, `backup`, `shoots`, `common` and `updatedAt`, recomputing them from its local state. A Mac tool may create the document with just `schema` and `size`.

**`files/{md5}/paths/{pathKey}`**: one per copy

Common fields:

| Field | Type | Meaning |
| --- | --- | --- |
| `schema` | int | Always 1 |
| `loc` | string | `card`, `ssd`, `primary` or `backup`. Never changes. |
| `path` | string | Normalized relative path. Never changes. |
| `size` | int | Bytes |
| `mtimeNs` | int | As observed by the writer |
| `seenAt` | timestamp | Last time the writer saw this copy |

Extra fields by `loc`:

- `card`: `card: { cardKey: string, sessionId: string }`. card-check finds a session's cached hashes with a collection-group query on `paths` where `card.sessionId` equals the session; enable the single-field index for that collection group.
- `ssd`: `ssd: { originalsPath: string, workingPath: string, workingSize: int, workingMtimeNs: int, copiedAt: timestamp, pushedAt: timestamp|null, modified: bool }`. `path` equals `originalsPath` for new-style shoots and `workingPath` for legacy ones.
- `primary` and `backup`: `verifiedAt: timestamp`, `missingSince: timestamp|null`. A copy that disappears gets `missingSince` and stops counting toward `copies`.

**`cardSessions/{sessionId}`**: one per card session; `sessionId` is a random ID

| Field | Type | Meaning |
| --- | --- | --- |
| `schema` | int | Always 1 |
| `volumeUuid` | string | The `cardKey` |
| `shoot` | string | Shoot folder name |
| `cardFolder` | string | Card folder name, such as `ILCE-7M4-01` |
| `cameraModel` | string | From EXIF |
| `cardLabel` | string\|null | Physical ID such as `S01`, if entered |
| `capacityBytes` | int | Card capacity, used to suggest the label |
| `firstIngestAt`, `lastSeenAt` | timestamp | |
| `samples` | list | Up to 5 `{ path, size }` used to confirm a re-inserted card |
| `stats` | map | `{ readErrors: int, hashMismatches: int }` for card health |

**`status/{name}`**

- `archive-hasher`: `heartbeatAt`, `lastRunStartedAt`, `lastSuccessAt`, `lastFullRunAt`, `filesPublished`.
- `archive-scrub`: `lastRunAt`, `filesChecked`, `mismatches`, `smart: map of disk → { healthy: bool, changed: list }`.
- `catalog-backup`: `lastStartedAt`, `lastFinishedAt`, `lastOutcome` (`success` or an error), `exportPath`.
- `sync-request`: `requestedAt` (set by the Mac, cleared by the Windows watcher).

**`config/settings`**: shared tunables. Fields and defaults are in [docs/config.md](config.md#shared-settings). The Mac may write it (validated by the rules); Windows writes it with its service account.

**`conflicts/{id}`**: random ID

`schema`, `kind` (`content-changed`, `backup-mismatch`, `missing`, `bit-rot`, `path-conflict`), `disk`, `path`, `md5`, `detectedAt`, `details`, plus `resolvedAt` and `resolution` once handled.

## Who can write what

| Writer | Identity | Enforced by | May write |
| --- | --- | --- | --- |
| Mac tools | Your Google account through Firebase Auth | Security rules | `files/{md5}` (`schema` and `size` only); `paths` docs with `loc` `card` or `ssd`; deleting `ssd` copies; `cardSessions`; `status/sync-request`; `config/settings`; marking `conflicts` resolved |
| Windows tools | Service account key | IAM role only (server SDKs bypass rules) | Everything except deleting `files` documents, by convention |
| catalog-backup | Its own service account | IAM | Exports only; no document writes except `status/catalog-backup` |

The Windows service account has the Cloud Datastore User role, which security rules can't narrow. Its protections are the separate GCP project, a key file only the task's account can read, and key rotation if it ever leaks. Firestore deletes by the Windows tools are limited by code review, not by the database.

## Security rules

The rules live in [`firestore/firestore.rules`](../firestore/firestore.rules). They:

- allow only one Firebase Auth user, the owner, identified by UID (set in `isOwner()`);
- let the owner read everything;
- restrict the owner's writes to the table above, validating field names, types, `schema == 1`, `loc` values and path shape, and keeping `loc` and `path` unchanged on update;
- deny everything else, including any collection not listed.

The rules check path shape only loosely (no `..`, no leading or trailing `/`, no `: * ? " < > |`, at most 1024 characters); the tools apply the full path rules above before writing. Keep it that way: the rules must never be stricter than the tools, because a queued offline write that the rules reject is dropped without any error reaching the tool (found in the [Firestore spike](../spikes/firestore-offline/README.md#findings)).

Test them with the Firebase emulator and the rules unit-testing library before deploying: one passing and one failing case for each `allow`. The tests are in `firestore/tests/`; how to run them is in [docs/testing.md](testing.md#layers).

## Changing the schema

- Adding an optional field: no version change; readers ignore unknown fields.
- Anything else (renaming, changing a type or meaning): bump `schema`, teach readers both versions, migrate with a one-off script, then drop the old version.
