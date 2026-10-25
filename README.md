# negatives

Verified photo and video ingest: know when a memory card is safe to format.

> **Status:** design phase. There's nothing to install yet; the design and
> user guide describe what's being built.

## What it does

- **Tells you when a card is safe to format:** every file on it has a
  hash-verified copy on two archive disks, however it got there.
- **Lets you edit right away** from your Mac's SSD while backups happen in
  the background.
- **Backs up your edits too:** ratings, sidecars, panoramas and rendered
  videos are archived alongside the untouched originals.
- **Frees SSD space only when you ask,** a whole shoot at a time, after
  re-checking every file against the archive.
- **Works offline:** ingest on the road; everything syncs when you're back.

## How it works

Small single-purpose tools on two machines share one catalog of file hashes
in Firestore. The Mac copies cards to its SSD and pushes them to the archive
over SMB; the Windows machine holding the archive disks mirrors and hashes
them locally, which is the only thing that can mark a file Safe. See the
system overview diagram in [the design doc](docs/design.md#system-overview).

The catalog is what sets negatives apart. Offload tools verify a copy once,
at the moment it's made; importers spot duplicates by filename and date;
backup tools copy files without checking them against the card. negatives
keeps a lasting record of every verified copy by content hash, keeps
re-checking the archive, and so can answer, for any card at any time, even
one copied years ago or half-copied last week: is it safe to format?

## Requirements

- A Mac with an APFS-formatted working SSD.
- A Windows machine with two archive disks (primary and backup), shared to
  the Mac over SMB.
- A Google Cloud project with Firestore, used only for negatives.

## Repo layout

Planned, as code lands:

- `mac/`: Swift package (CatalogKit) and the Mac tools: card-check,
  card-watch, ingest, ssd-push, ssd-evict, ssd-restore, ssd-watch.
- `windows/`: Go and PowerShell tools: archive-sync, archive-hasher,
  archive-scrub and the watcher.
- `tools/lrcat-pack/`: manual utility for packing old Lightroom catalogs.
- `docs/`: design and user guide.

## Docs

- [Design](docs/design.md): how it works and why.
- [Tool specs](docs/tools/): one file per tool.
- [User guide](docs/user-guide.md): installing and using the tools.

## Not yet

- Network attached storage (Synology, TrueNAS and similar) as the archive
  host instead of a Windows machine is planned for a future version.
