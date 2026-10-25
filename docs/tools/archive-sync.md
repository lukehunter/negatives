# archive-sync

Go, driving robocopy. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes). Code in [`windows/`](../../windows/); manual tests in [archive-sync-manual-tests.md](archive-sync-manual-tests.md).

- **Why Go rather than PowerShell:** the decisions (which jobs to run, what a difference between the disks means, what may be quarantined) are pure Go functions, property-tested on any OS; reconcile needs archive-hasher's facts, which are Go code too; and the Windows tools share one language and one config reader.
- **Never writes to the primary disk.** It only lists folders and reads sizes and times there, and robocopy only reads from it. A last check before each robocopy run refuses any job whose source isn't on primary, whose destination isn't on backup or overlaps quarantine, or that has `/MIR`, `/PURGE`, `/MOV`, `/MOVE` or `/CREATE`.
- **Pass 1, additive only.** One robocopy job per top-level folder on primary, plus one for files at the root (no `/E`, `/DCOPY:T` so the root's hidden and system attributes aren't copied). Flags: `/E /XC /XN /XO`, so a file already on the backup is never overwritten whatever its size or time. A negatives shoot's `Derived\` folder is excluded by its full path (`/XD`). One job per folder keeps the command line short however many shoots there are; one robocopy run with every Derived folder in `/XD` would outgrow the Windows command-line limit (32,767 characters) after several hundred shoots.
- **Pass 2:** one job per negatives shoot with a `Derived\` folder, `/E` with overwrites allowed, so replaced renders reach the backup. This replaces older Derived versions on the backup; the online backup keeps older versions.
- **On every job:** `/R:2 /W:5`; `/XJ` (never follow junctions); `/XF *.partial` (ssd-push's in-progress files are never copied, so they can't pile up on the backup); `/NP /NDL /NJH /NJS /FP` (the log lists only files copied and errors, with full paths); `/UNILOG+:` (Unicode log, so any file name logs correctly). Never `/MIR` or `/PURGE`.
- **Skipped at the root:** `$RECYCLE.BIN`, `System Volume Information`, and the quarantine folder's name. A folder on primary with the quarantine folder's name stops the run with an error, so nothing is ever copied into quarantine.
- **Exit codes:** robocopy's 0 to 7 is success, 8 and above is failure, and so is a robocopy that archive-sync stopped (Ctrl-C, or the task being ended). archive-sync exits 0 when every job succeeded and nothing failed to read or move, else 1, for Task Scheduler. A failed job doesn't stop the others. A robocopy killed from outside (`taskkill /F`) exits with 1, which reads as success; the partial file it leaves is found by the next run's reconcile.
- **robocopy dies with archive-sync:** each robocopy runs in a Windows job object that is closed, killing robocopy, when archive-sync exits for any reason, so no robocopy keeps writing after the lock is released.
- **Order of a run:** reconcile, then pass 1, then pass 2. Reconcile first means a partial copy left by an interrupted earlier run is recopied by this run's pass 1.

## Reconcile

The only time anything on an archive disk is moved. If robocopy is interrupted mid-file, it can leave a partial file on the backup disk, and `/XC /XN /XO` would then skip it forever.

- **The walk:** compares the two disks folder by folder, reading each folder's listing once on each disk, case-insensitively. It reads sizes and times only, never file contents. Negatives shoots' `Derived\` folders are skipped (pass 2 overwrites them), as are `.partial` files and links.
- **A backup file is moved to quarantine only when all of these hold** (the pure function `Decide`):
  - it's smaller than the primary copy (a partial copy is never larger);
  - archive-hasher has verified the primary copy exactly as it is now (same size and time as it hashed);
  - archive-hasher has never seen other content at that primary path (so the backup can't be an original kept from before an in-place edit; see the failure table in the design);
  - the backup's content, if archive-hasher has hashed it, isn't real data known elsewhere (on a card, or verified at another path);
  - for originals, the primary hash matches the card hash.
- These are stricter than "size differs": a backup copy **larger** than primary, or one where the primary was edited in place, is never moved. Those, a **same-size file with a different time**, and a file on one disk that is a folder on the other are listed in the log for a person; archive-hasher compares hashes and reports `conflicts`. Nothing is fixed automatically.
- **Just before moving,** it reads both files' size and time again and asks for the facts again; if anything changed since the walk, the file is left in place and the run reports an error.
- **Quarantine:** the file is renamed (`MoveFileEx` with no flags: it fails rather than overwrite, and fails rather than copy across volumes) to `<quarantine>\<run id>\<same relative path>`, and a line is added to `<quarantine>\<run id>\MANIFEST.tsv` with both sizes and times and the primary MD5. The quarantine folder must sit directly at the backup disk's root. Quarantine is never emptied automatically: review it and empty it by hand. archive-hasher must skip it.
- **Until archive-hasher exists (build step 3),** nothing is verified, so partial copies are listed in every run's log ("partial-awaiting-verification") and kept as they are. They aren't Safe either, since their hash won't match. Step 3 supplies the facts through the `FactSource` interface, from archive-hasher's state and the catalog.

## Report-only mode

`archive-sync --report`, for the pre-migration check that primary and backup match. It writes nothing to either disk: reconcile only lists, and every robocopy job gets `/L`. It writes `archive-sync-report-<run id>.tsv` to the logs folder, listing every path that isn't identical on both disks (missing on backup, only on backup, same size but different time, backup larger or smaller, file versus folder), and robocopy's log lists what pass 1 and pass 2 would copy.

## Files

- `logs\archive-sync-<date>.log`: archive-sync's own log (UTF-8), one per day, appended. Also printed to the console.
- `logs\robocopy-<date>.log`: robocopy's log (UTF-16, from `/UNILOG+`), one per day, appended.
- `archive-sync.lock`, next to the config file: an operating-system lock held for the whole run, so a manual run can't overlap a scheduled one. It's released when the process exits for any reason, so a crash never leaves a stale lock.

## Legacy folders

Any shoot folder without a `.negatives` marker is legacy and covered by pass 1, additive only, so old folders never need to move. Folders named `Originals` or `Derived` inside a legacy folder get no special treatment.
