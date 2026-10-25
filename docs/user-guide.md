# negatives: User Guide

Oct 24, 2026 · @Luke

How to install negatives and use it day to day. Commands shown are the planned interface and may change as the tools are built.

## What negatives does

negatives tells you when a memory card is safe to format: every file on it has a verified copy on both of your archive disks. It also copies cards to your Mac's SSD so you can start editing right away, backs that work up to the archive in the background, and frees SSD space only when you ask and it's provably safe.

You'll mostly interact with it through notifications: insert a card, read what it says, click Ingest if there's something to copy. Everything else runs on its own.

## What you need

- **A Mac** with a working SSD formatted as APFS. Check with `diskutil info /Volumes/YourSSD` and look for "APFS".
- **A Windows machine** at home holding your primary and backup disks, shared to the Mac over SMB.
- **A Google account** and a Google Cloud project used only for negatives, with Firestore turned on. The first load of an existing archive costs a small one-time fee; everyday use fits the free tier.
- **smartmontools** on the Windows machine, for disk health checks.
- About a day or two per 20 TB for the first full hash of your existing archive.

## Installing on Windows

Set up the Windows machine first; the Mac relies on it.

1. **Install** the negatives Windows tools (archive-sync, archive-hasher, archive-scrub and the watcher) and smartmontools.
2. **Create a service account key** in your Google Cloud project with only Firestore access. Save the key file somewhere only the account that runs the task can read it. Don't sign in with your own Google account on this machine.
3. **Pack old Lightroom catalogs** (optional, but recommended): run `lrcat-pack <archive folder>` to see what it finds, then `--apply` to pack them. See Other tasks.
4. **Configure your disks** in the negatives config file: the primary and backup disk roots. Leave your existing folders where they are; just check that none of your old shoot folders already contains a folder named Originals or Derived.
5. **Check the disks match:** run `archive-sync --report` and fix any differences it lists by hand.
6. **Nothing else to create.** Each new shoot gets its own folder at the root of the disk, with `Originals` and `Derived` inside it, the first time it's backed up.
7. **Run the first full hash** by hand: `archive-hasher --full`. It takes days for a large archive, and you can stop and resume it.
8. **Turn on automation:** `negatives install-task` creates the scheduled task, the startup run, the nightly run and the watcher.
9. **Set disks to spin down:** in Power Options, set "Turn off hard disk after" to a few minutes.

## Installing on the Mac

1. **Install** the negatives Mac tools: the card-watch app and the command-line tools (card-check, ingest, ssd-push, ssd-evict).
2. **Sign in** with your Google account when card-watch first starts. The sign-in is kept in your Keychain.
3. **Choose your working SSD.** negatives checks that it's APFS and refuses anything else.
4. **Move existing working folders** on the SSD into a folder named `Legacy` at its root. negatives leaves them alone until you clean them up with `ssd-evict --legacy`.
5. **Keep Lightroom catalogs out of `Shoots`** if you still use Lightroom.
6. **Allow notifications** for card-watch when macOS asks, so you see card summaries.
7. **Mount the primary share** and let ssd-push reconnect to it automatically.

Don't create the `Shoots` or `.staging` folders yourself; the first ingest creates them.

## Everyday use

1. **Insert the card.** A notification shows how many files are Safe, Pending and Not copied, and whether the card is safe to format.
2. **Click Ingest** if there's anything to copy. Type an optional description for the shoot (such as "Coast"); the folder becomes `2026-10-18 Coast`. On a card's first use since formatting, you're also asked for its card ID. If the card has files from an earlier ingest plus new ones, you're asked whether to continue that shoot or start a new one; click Preview to open the first new file and decide.
3. **Start editing** in `Shoots/<shoot>` on the SSD as soon as the done notification appears. Rate, add sidecars, render panoramas and videos into the shoot folder as usual; it all gets backed up.
4. **Leave the card alone for now.** In the background, your files are copied to the Windows machine, mirrored to the backup disk and verified. This needs the Windows machine to be on.
5. **Insert the card again later.** When the notification says it's safe to format, format it in the camera.

What the states mean:

| State | Meaning |
| --- | --- |
| Safe | Verified on both archive disks. Fine to format the card or free it from the SSD. |
| Pending | Copied, but not yet verified on both disks. Wait. |
| Not copied | Only on the card. Click Ingest. |

## Freeing SSD space

negatives never frees space on its own. When you need room:

1. Run `ssd-evict`. It lists your shoot folders with their size and status.
2. Choose shoots to remove. Only shoots where everything is Safe can be chosen; the others show why they aren't ready yet.
3. Review the summary and confirm. It re-checks every file against the archive right before deleting, and skips anything that changed.

It works on whole shoot folders and needs a connection. Use `ssd-evict --legacy` to clean up folders in `Legacy` from before negatives.

If a card won't fit, ingest copies nothing and tells you how much space you need; free some with `ssd-evict` and try again.

## Other tasks

**Checking old cards.** Insert any card, even one from years ago. The notification tells you whether its files are already archived, however they got there.

**Editing an archived shoot again.** Run `ssd-restore` and pick the shoot. It copies the shoot back to the SSD with your latest edits on top, checks every file, and puts it in `Shoots`. Old shoots from before negatives come back the same way. Edit as usual; changes are backed up, and you can free the shoot again with `ssd-evict` whenever you like.

**Labeling cards.** Give each card a short ID with a fine permanent marker: a letter for the type and a number, such as S01 for SD or C01 for CFexpress. Never reuse a number. Write it on the label away from the contacts, or on the case for tiny cards. Enter it when ingest asks; you can skip, but it lets negatives show a card's history and spot one going bad.

**Syncing now.** To skip the wait for the Windows machine to notice new files, run `card-check --sync-now` on the Mac, or `schtasks /run /tn negatives` on Windows.

**Working offline.** Ingest works without internet: files are copied and recorded, and everything syncs once you're back online. Two things wait for a connection: being told a card is safe to format, and freeing SSD space. An old, unformatted card ingested offline may have some files copied twice; ssd-evict cleans up the extras later.

**Packing old Lightroom catalogs.** On the Windows machine, run `lrcat-pack <folder>` to list the catalogs it finds, then `lrcat-pack --apply <folder>` to turn each into a single zip on both disks. It asks before removing the loose files, and only after both zips verify. To restore a catalog, unzip it next to where it was. Close Lightroom first; catalogs that look open are skipped.

## Warnings and what to do

| You see | What it means | What to do |
| --- | --- | --- |
| Windows host offline since … | The Windows machine is off or asleep. | Turn it on. Files stay Pending until it is. |
| Archive check overdue | The Windows machine is on but its tasks aren't running. | Check the scheduled task and the watcher on Windows. |
| Disk health warning | A disk's SMART counters got worse, or a sampled file no longer matches its hash. | Replace the disk soon. Repair the named file from the other disk or the online backup. |
| Conflict reported | A file on the archive changed where it shouldn't, or a copy never matched. | Look at the named file and fix it by hand. |
| Original changed (only with ssd-watch) | A file in `.staging` changed, which shouldn't happen. | If the card isn't formatted, ingest it again. |
| SSD isn't APFS | The working SSD has the wrong format. | Reformat it as APFS once everything on it is archived. |
| Not enough space | The card's new files don't fit on the SSD. | Run `ssd-evict` and try again. |
