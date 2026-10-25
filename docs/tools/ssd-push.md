# ssd-push

SSD to primary over SMB, in the background. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Runs from launchd on a schedule and whenever the primary share is reachable; can also be run by hand.
- **Originals:** copies each file in `.originals/` to `<shoot>/Originals/<card>/<path on card>`, additive only: never deletes or overwrites. A different file already at the target path is reported as a conflict. The first push of a shoot also writes its `.negatives` marker.
- Before pushing an original, re-hashes the `.originals/` copy against the card hash. A mismatch means corruption: it isn't pushed, it's reported, and the card stays Pending.
- A file whose hash is already verified in the archive, at any path, is never pushed again, so re-copied, moved or renamed files never create duplicates. When such a file sits at a different path than its archived copy, it's treated as moved: nothing is copied, and the shoot is reported (see [Moving and renaming](../design.md#folder-layout)).
- Posts one macOS notification per shoot (through the card-watch app) when paths on the SSD no longer match the archive, listing them.
- **Working copies:** if size and mtime match what ingest recorded, there's nothing to push. If they changed, it hashes the file; a hash still equal to the card hash also means nothing to push. A different hash makes it a derived file.
- **Derived files** (changed working copies, plus new files such as panoramas, renders, .gyroflow files and XMP sidecars) go to the same path inside the shoot's `Derived/` folder once unchanged for an hour, replacing any previous version. Later changes are pushed the same way: each run compares the file with the version it last pushed, and one that changed and has settled for an hour replaces it. Caches, proxies and temp files are skipped via a configurable exclude list.
- **Legacy shoot folders** (no `.originals/`): pushed by path, since names and paths match the archive. Each file is hashed once and recorded in the catalog. A file missing from the same path in the archive shoot folder is copied there, additive only, unless its content is already archived elsewhere. New folders added to a legacy shoot are archived this way. One already there with the same hash needs nothing. One there with different content (edited on the SSD) is never overwritten: it goes to the same path inside the shoot's `Derived/` folder instead, and the archive folder gets a `.negatives` marker. The same one-hour settling period applies.
- **Exclude list** (starting point; confirm with a test shoot):
  - Skip `Proxy/` folders. DaVinci Resolve puts proxies there when set to [proxy subfolders in media file locations](https://ymcinema.com/2022/11/18/creating-proxies-in-davinci-resolve-18-tips-tricks/), and the [Blackmagic Proxy Generator](https://jayaretv.com/edit/davinci-resolve-proxy-workflow-explained/) does by default. Also skip Resolve's cache, proxy and optimized-media working folders if they ever point inside `Shoots/`.
  - Skip Bridge's exported caches: `.BridgeCache`, `.BridgeCacheT` and `.BridgeSharedCache/` ([Adobe community](https://community.adobe.com/t5/bridge-discussions/does-adobe-bridge-remove-the-old-bridgecache-when-a-new-cache-location-is-specified-in-the/m-p/9734618)).
  - Skip each shoot's `.originals/` folder: those files are pushed as originals, never as derived. Also confirm during the test shoot that editing apps don't import hidden folders.
  - Skip `.DS_Store`, `._*` files and `*.partial` temp files.
  - Keep `.BridgeSort`: it holds Bridge's manual sort order and stacks ([Adobe community](https://community.adobe.com/t5/bridge-discussions/location-for-information-about-stacks/td-p/8887142)), so it's real work.
  - Keep XMP sidecars, `.gyroflow` projects and stabilized renders, which Gyroflow writes [next to the video by default](https://docs.gyroflow.xyz/app/advanced-usage/render-queue).
  - Lightroom keeps its catalog and previews in the catalog folder (approximate, from memory), so keep the catalog outside `Shoots/`.
  - Catalyst can edit clip metadata and export metadata sidecars; test whether edits rewrite the card's XML files, which would make them derived files.
- Writes each file under a temporary `.partial` name and renames it when complete, so archive-hasher never hashes a half-written file.
- Records `pushedAt` on the file's `ssd` entry, so card-check can show "pushed, awaiting verification".
- Share unavailable or offline: does nothing and retries next run.
- Takes each shoot's lock while working on it. A shoot locked by another tool (for example ssd-evict or ingest) is skipped and retried on the next run.
