# archive-sync: manual tests on the Windows host

Checks for [archive-sync](archive-sync.md) that need Windows and robocopy. They answer the open item in `CLAUDE.md`, "how robocopy handles an interrupted copy with /XC /XN /XO", and the failure test in [docs/testing.md](../testing.md#failure-injection), "robocopy killed mid-file".

**Use test disks, never the real archive disks**, except for the read-only steps in section G. Record what you see in the results table at the end, then update this file and `docs/design.md` if anything differs from what's expected.

## Setup

1. **Build** on the Mac, from `windows/`:

   ```bash
   GOOS=windows GOARCH=amd64 go build -o archive-sync.exe ./cmd/archive-sync
   ```

   Copy `archive-sync.exe` to `C:\negatives-test\` on the Windows machine. To run the automated robocopy tests (section A) the Windows machine also needs Go and a copy of the repo.

2. **Make two test disks** as virtual disks, so they're separate volumes like the real ones. In an administrator prompt run `diskpart`, then:

   ```text
   create vdisk file="C:\negatives-test\primary.vhdx" maximum=60000 type=expandable
   attach vdisk
   create partition primary
   format fs=ntfs quick label=TESTPRIMARY
   assign letter=T
   create vdisk file="C:\negatives-test\backup.vhdx" maximum=60000 type=expandable
   attach vdisk
   create partition primary
   format fs=ntfs quick label=TESTBACKUP
   assign letter=U
   ```

   For realistic speeds in section C, put the `.vhdx` files on a spinning disk that isn't an archive disk, or use a spare USB disk instead.

3. **Test config** at `C:\negatives-test\config.toml`:

   ```toml
   [disks]
   primary = 'T:\'
   backup = 'U:\'
   quarantine = 'U:\_quarantine'

   [logs]
   dir = 'C:\negatives-test\logs'
   ```

   Every command below uses `--config C:\negatives-test\config.toml`; the lock file goes next to it.

4. **Robocopy version:** run `robocopy /?` and note the version line at the top in the results table.

## A. Automated robocopy tests

From `windows\` in the repo, with the temporary folder on the test primary disk so the files go through NTFS on a real volume:

```text
set TMP=T:\tmp
mkdir T:\tmp
set NEGATIVES_BIG_MB=4096
go test -tags robocopy -v ./internal/archivesync -run Robocopy
```

- [ ] `TestRobocopyAdditiveFlags` passes: new files copied; files already on the backup kept whatever their size, time or content; backup-only files kept; `.partial` not copied; Derived replaced by pass 2; `/L` changed nothing.
- [ ] `TestRobocopyInterruptedCopy` passes, or fails with a clear message. Copy its two `OBSERVED:` lines (sizes and times of the original and the partial) into the results table.
- [ ] If it skips with "robocopy finished before it was killed", raise `NEGATIVES_BIG_MB` and run it again.

Both tests use temporary folders on one volume; sections B and C repeat the important parts on two volumes by hand.

## B. What /XC /XN /XO do

1. On `T:` create `T:\Legacy\` with these files, and on `U:` create `U:\Legacy\` with the backup versions. Set times with PowerShell, for example `(Get-Item U:\Legacy\newer.txt).LastWriteTime = '2026-01-01'`.

   | File | On T: (primary) | On U: (backup) | Expected after a run |
   | --- | --- | --- | --- |
   | `new.txt` | exists | missing | copied |
   | `same.txt` | same content and time | same content and time | untouched |
   | `changed.txt` | same time, longer | same time, shorter | **backup kept** |
   | `newer.txt` | newer, same size | older | **backup kept** |
   | `older.txt` | older, same size | newer | **backup kept** |
   | `lookalike.txt` | `AAAA` | `BBBB`, same time | **backup kept** |
   | `extra.txt` | missing | exists | **kept** |
   | `x.MP4.partial` | exists | missing | not copied |

2. See how robocopy classifies each one, without copying:

   ```text
   robocopy T:\Legacy U:\Legacy /E /XC /XN /XO /L /V /NJH
   echo %ERRORLEVEL%
   ```

   - [ ] Note the label robocopy prints for each file (New File, same, Changed, Newer, Older, *EXTRA File) and the exit code.

3. Run `archive-sync.exe --config C:\negatives-test\config.toml`.
   - [ ] Every row of the table matches "Expected".
   - [ ] The console and `C:\negatives-test\logs\archive-sync-<date>.log` list `same-size-time-differs` for `newer.txt` and `older.txt`, and `partial-awaiting-verification` for `changed.txt` (a shorter backup copy looks like a partial one); nothing was quarantined; exit code (`echo %ERRORLEVEL%`) is 0.
   - [ ] `robocopy-<date>.log` opens in Notepad (it's UTF-16) and lists only `new.txt` for pass 1.
   - [ ] `attrib U:\` shows the backup root isn't hidden or system after the root files job (only if `T:\` had files at its root: add `T:\root.txt` first).

## C. Interrupted copy of a large video

Use a real, long video clip (the larger the better, at least 20 GB, for example a long Sony XAVC clip), since it gives time to interrupt and has realistic data.

1. Put it on the test primary as a negatives original, and record its hash:

   ```text
   mkdir "T:\2026-10-18 Coast\Originals\a7rv-01\PRIVATE\M4ROOT\CLIP"
   copy <clip> "T:\2026-10-18 Coast\Originals\a7rv-01\PRIVATE\M4ROOT\CLIP\C0001.MP4"
   type nul > "T:\2026-10-18 Coast\.negatives"
   certutil -hashfile "T:\2026-10-18 Coast\Originals\a7rv-01\PRIVATE\M4ROOT\CLIP\C0001.MP4" MD5
   ```

2. Interrupt a copy. Start `archive-sync.exe --config C:\negatives-test\config.toml` in one window. When `U:\2026-10-18 Coast\...\C0001.MP4` appears, stop it one of these ways (repeat steps 2 to 5 for each, removing the partial from `U:` by hand between variants):
   - [ ] (a) Ctrl-C in the archive-sync window.
   - [ ] (b) `taskkill /F /IM robocopy.exe` from a second window. A killed robocopy exits with 1, which reads as "files copied", so this run may exit 0; note which. Either way, step 4 must find the partial.
   - [ ] (c) `taskkill /F /IM archive-sync.exe`. Then check Task Manager: robocopy should have been killed with it (job object). Note if it keeps running.
   - [ ] (d) Optional, with a spare USB disk as the backup, never an archive disk: unplug it mid-copy.

3. Record what's left on `U:`, in PowerShell:

   ```powershell
   Get-Item "U:\2026-10-18 Coast\Originals\a7rv-01\PRIVATE\M4ROOT\CLIP\C0001.MP4" |
     Format-List Length, LastWriteTime, CreationTime, Attributes
   ```

   - [ ] Is the partial's `Length` smaller than the original's, or the full size (robocopy preallocated it)?
   - [ ] What is its `LastWriteTime`: the original's time, the time of the copy, or a fixed date such as 1980-01-01?
   - [ ] archive-sync's exit code (`echo %ERRORLEVEL%`) was 1 for (a) and (c).

4. Run archive-sync again, normally.
   - [ ] The log lists the clip as `partial-awaiting-verification` and says it's kept in place until archive-hasher verifies it.
   - [ ] The partial's size and time are unchanged: robocopy didn't recopy it by itself.
   - [ ] Nothing was created in `U:\_quarantine`.
   - [ ] Exit code 0.

5. Run `archive-sync.exe --config C:\negatives-test\config.toml --report`.
   - [ ] `logs\archive-sync-report-<run id>.tsv` lists the clip as `partial-awaiting-verification` with both sizes.
   - [ ] Nothing changed on either disk.

6. **If the partial was full-size** in step 3, stop here: reconcile's size rule can't recognize it, and the design has to change before archive-sync replaces the existing copy job (options: recognize robocopy's in-progress time stamp, or have archive-hasher treat a same-size backup file with a different hash as a partial when it matches nothing). Note the findings in `docs/design.md` and `CLAUDE.md`.

7. The quarantine and recopy themselves can't run by hand until archive-hasher exists (build step 3); `TestRobocopyInterruptedCopy` (section A) covers them with a stand-in for the hasher. After step 3, repeat steps 2 to 4 and check:
   - [ ] the partial is in `U:\_quarantine\<run id>\2026-10-18 Coast\Originals\...\C0001.MP4`, with a line in that run's `MANIFEST.tsv`;
   - [ ] the clip on `U:` has the same MD5 as on `T:` (`certutil -hashfile`).

## D. Report-only mode changes nothing

1. With differences on the test disks (sections B and C leave some), save listings of both disks:

   ```powershell
   foreach ($d in 'T:\','U:\') { Get-ChildItem $d -Recurse -Force | Select-Object FullName, Length, LastWriteTimeUtc | Export-Csv "C:\negatives-test\before-$($d[0]).csv" }
   ```

2. Run archive-sync with `--report`, then save the listings again as `after-*.csv` and compare them (`fc before-T.csv after-T.csv`, same for U).
   - [ ] Both disks are unchanged.
   - [ ] The report lists every difference: missing on backup, only on backup, same size but different time, smaller and larger backup copies.

## E. Lock

- [ ] Start a long run, then start a second one in another window: the second prints "another archive-sync run is in progress" and exits 1 at once.
- [ ] Kill the first with `taskkill /F`: the next run starts normally (no stale lock).

## F. Refusals

Each must stop with a clear message, exit 1, and change nothing:

- [ ] `backup = 'T:\'` (same disk as primary).
- [ ] `quarantine = 'T:\_quarantine'` (not on the backup disk), and `quarantine = 'U:\x\_quarantine'` (not at the root).
- [ ] `[logs] dir = 'U:\logs'` (on an archive disk).
- [ ] A misspelled key, such as `bakup = 'U:\'`.
- [ ] A folder `T:\_quarantine` on the primary.

## G. Real archive, read-only

Only `--report` touches the real disks, and it writes nothing to them.

- [ ] Run `archive-sync --report` with the real config before turning anything on (migration step 3 in the design). Note how long it takes and how many lines the report has; resolve the differences by hand.
- [ ] Watch for files with long paths (over 260 characters) and non-ASCII names in the report and robocopy's log: they should show correctly.

## Results

| Date | Windows and robocopy version | Check | Observed |
| --- | --- | --- | --- |
| | | A: OBSERVED lines | |
| | | B: robocopy labels and exit code | |
| | | C3: partial size and time, per variant (a) to (d) | |
| | | C2(b): exit code after robocopy was killed | |
| | | C2(c): robocopy kept running after archive-sync was killed? | |
| | | G: report duration, number of differences | |
