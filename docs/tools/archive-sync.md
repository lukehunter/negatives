# archive-sync

PowerShell wrapping robocopy. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Copies primary to backup in two passes, never with `/MIR` or `/PURGE`. Pass 1 copies everything additive-only (`/E /XC /XN /XO /R:2 /W:5 /LOG+:<log>`), excluding each negatives shoot's `Derived\` folder by full path (`/XD`). Pass 2 loops over those `Derived\` folders and copies them with overwrites allowed, so replaced renders reach the backup.
- Interprets robocopy exit codes (0 to 7 = success, 8+ = failure) and exits with a plain 0 or 1 for Task Scheduler.
- Reconcile step, the only time anything on an archive disk is moved: if robocopy is interrupted mid-file, it can leave a partial file on the backup disk, and the no-overwrite flags would then skip it forever. A backup file is set aside only when all of these hold: its size differs from the primary copy, the primary copy's hash is verified, and for originals that hash matches the card hash. It's moved to a quarantine folder on the backup disk, never deleted, so the next pass recopies the good version. Quarantine is emptied by hand. A same-size file with a different hash is a conflict for the hasher to report, never fixed automatically.
- Takes a lock file so a manual run can't overlap a scheduled one.
- **Legacy folders:** any shoot folder without a `.negatives` marker is legacy and covered by pass 1, additive-only, so old folders never need to move.
