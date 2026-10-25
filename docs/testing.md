# Testing

How negatives is tested. Part of [negatives](design.md). The tools decide when it's safe to format a card or delete a file, so testing concentrates on those decisions and on what happens when things fail halfway.

**Contents**

- [What must never break](#what-must-never-break)
- [Layers](#layers)
- [Test fixtures](#test-fixtures)
- [Failure injection](#failure-injection)
- [End-to-end scenarios](#end-to-end-scenarios)
- [Manual checks](#manual-checks)
- [Continuous integration](#continuous-integration)

## What must never break

These invariants get tests at every layer, and property-based tests: the framework generates thousands of random cases, checks the invariant after every step, and shrinks any failure to the shortest case that still breaks it.

- **From the start:** the core decisions are written as pure functions that take catalog facts and file state as input: is a file Safe, may a shoot be evicted, should a file be pushed and where, is a card safe to format. These are property-tested from day one.
- **Later:** once the tools work end to end, random sequences of real operations (ingest, rate a file, push, interrupt, go offline, rename, evict, restore) run against fake disks, a fake clock and the emulator, with the invariants checked after every step.

The invariants:

1. A card is never reported safe to format unless every non-empty file on it has a verified copy on both disks.
2. Nothing in `.originals/` is ever modified, and nothing in a shoot's `Originals/` on the archive is ever overwritten or deleted.
3. ssd-evict never removes a file that isn't Safe at the moment of deletion.
4. No tool deletes anything outside what its spec allows (ingest: its own temporary files; ssd-evict: confirmed Safe shoots; reconcile: size-mismatched backup copies, moved to quarantine).
5. Swift and Go agree exactly on path normalization, `pathKey`, MD5 and the Safe/Pending/Not copied rules.

## Layers

**Unit tests** (fast, no disks or network)

- Path normalization: NFD to NFC, reserved names and characters, length limits, `..` and empty segments.
- Case-insensitive path matching (`private/` and `PRIVATE/` are the same folder).
- `pathKey`, MD5 and derived states, checked against **shared golden vectors** in `testdata/vectors.json`, which both the Swift and the Go test suites load. This is what keeps the two languages in agreement.
- Shoot naming from capture dates, card folder counters, mixed-camera cards, Windows-unsafe names.
- Exclude-pattern matching, including the rule that `.originals/` can never be excluded.
- Config parsing and validation for both TOML files and the shared settings.
- robocopy exit-code handling, with Pester tests for the PowerShell wrapper.

**Security rules tests**

- Run in the Firebase emulator with the rules unit-testing library: at least one allowed and one denied case for every `allow` in `firestore/firestore.rules`, plus a check that an unknown user and an unknown collection are denied.

**Integration tests** (against the Firestore emulator)

- The Swift tools through the Firebase SDK and the Go tools through `FIRESTORE_EMULATOR_HOST`, sharing one emulator.
- Cross-language round trips: Go publishes archive facts and Swift reports the file as Safe; Swift records card and SSD copies and Go's hasher counts them correctly.
- Offline queueing: writes made with the emulator stopped land correctly after a restart, including after the Mac tool itself restarts.

## Test fixtures

- **Fake cards as disk images.** A script builds card images with `hdiutil create -fs ExFAT`, so they mount under `/Volumes` with a real volume UUID like a card, and card-watch's mount trigger is exercised for real. "Formatting" a card means recreating its image, which gives it a new UUID. Contents:
  - DCIM folders including a 9999 rollover (`100MSDCF` into `101MSDCF`), and the same names on two cards;
  - Sony-style `PRIVATE/M4ROOT` clips with their XML sidecars and index files;
  - zero-byte files, files duplicated across cards, and common files repeated on every card;
  - real sample files from each camera body in use (a JPEG, a raw and a short video clip with its sidecars), stored in `testdata/samples/<camera>/` with Git LFS, since raws and clips are large;
  - small synthetic JPEGs with EXIF `Model` and capture dates set by exiftool, for edge cases such as mixed camera models on one card;
  - one large file (several GB) to exercise streaming and temporary files.
- **Sample file privacy:** before committing samples, strip GPS and other personal metadata with exiftool, so the repo is safe to make public later.
- **Fake working SSD:** an APFS disk image, so clones and `chflags uchg` behave as they do on the real SSD. An HFS+ or exFAT image checks that ingest refuses non-APFS volumes.
- **Fake archive:** on the Mac, a local folder stands in for the primary share. On Windows, two virtual disks (VHDX) mounted as separate drive letters stand in for primary and backup.

## Failure injection

Each of these is a scripted test that interrupts an operation and then checks the invariants:

- ingest killed mid-copy: the next run removes its own leftover `.partial` files and finishes; the card stays Pending.
- ssd-push killed mid-copy, or the share disconnected: the archive has only a `.partial`, which archive-hasher ignores and the next push replaces.
- robocopy killed mid-file: the partial backup copy is quarantined and recopied (this also verifies how robocopy marks partial files).
- A file in `.originals/` changed after clearing its lock: ssd-push refuses it and ssd-watch reports it.
- A file deleted from the primary disk: archive-hasher reports it missing and it stops counting as Safe.
- The Mac offline for an ingest, then back online.
- A folder renamed in a shoot: nothing is duplicated, and the "paths differ" notification fires.
- The catalog wiped and restored from an export, followed by both reconcile steps.
- Two tools racing on one shoot (push and evict): the per-shoot lock serializes them.

## End-to-end scenarios

Scripted walkthroughs that run the real tools in order, matching the design doc:

- A new card: ingest, push, sync and hash, then the card reports safe to format.
- A card with old and new files, online and offline (the [walkthrough](design.md#walkthrough-a-card-with-old-and-new-files)).
- An old card already archived: ingest copies nothing.
- Rating a JPEG: the working copy becomes a derived file and is pushed after the settling time.
- Evicting a shoot, then restoring it with ssd-restore: edits come back on top of the originals.
- A legacy shoot on the SSD: missing files archived by path, an edited file pushed to `Derived/`.

## Manual checks

Things that need real hardware or real apps, done once and then whenever something changes:

- Each editing app with a test shoot: Lightroom, Bridge, Resolve, Catalyst and Gyroflow ignore the hidden `.originals/` folder, write sidecars and renders where expected, and Bridge ratings on JPEGs show up as derived files.
- A real camera card from each body, including a video card.
- **Card volume UUID behavior**, which card sessions depend on. With `diskutil info /Volumes/<card> | grep "Volume UUID"`, confirm that the UUID:
  - stays the same across unmounting and re-inserting;
  - is the same in every card reader you use (built-in SD slot and USB readers);
  - stays the same after shooting more and after deleting photos in camera;
  - changes when the card is formatted in camera, and when it's formatted on the Mac;
  - differs between two cards formatted in the same camera on the same day.
  Do this for each card type (SD and CFexpress) and each camera brand.
- SMART readings through the actual disk enclosures, and disk spin-down on the Windows machine.
- Firestore offline writes on macOS (build order step 1).

## Continuous integration

- Decided: GitHub Actions from the start, on every push: Go unit and integration tests on Linux and Windows runners, Swift tests on a macOS runner, and the rules tests with the Firestore emulator.
- Disk-image fixtures run on the macOS runner; Windows VHDX tests and robocopy checks run on the Windows runner.
- Tests never touch the real Firebase project: they use the emulator only.
