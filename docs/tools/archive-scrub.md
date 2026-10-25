# archive-scrub

Go, disk health checks. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Runs at the end of the nightly run, while the disks are already spinning, within a fixed time budget so it never keeps them spinning for hours.
- Samples random files from both disks, favoring files checked longest ago, re-hashes them and compares against the catalog. Over many nights it works through the whole archive, tracking when each file was last checked in its local state.
- A mismatch is reported to `conflicts` as possible bit rot, naming which disk is bad. Repair is by hand, from the other disk or the online backup.
- Reads each disk's SMART data with smartctl (smartmontools runs on Windows): overall health, reallocated, pending and uncorrectable sectors, interface CRC errors and temperature. It reports any counter that rose since the last run. Some USB enclosures don't pass SMART data through; if so, it says so rather than silently skipping.
- Writes results to `status/archive-scrub`, and the Mac shows a warning on any failure, new SMART error, or a scrub that hasn't run recently.
