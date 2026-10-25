# negatives: User Guide

Oct 24, 2026 · @Luke

How to install negatives and use it day to day. Commands shown are the planned interface and may change as the tools are built.

## What negatives does

negatives tells you when a memory card is safe to format: every file on it has a verified copy on both of your archive disks. It also copies cards to your Mac's SSD so you can start editing right away, backs that work up to the archive in the background, and frees SSD space only when you ask and it's provably safe.

You'll mostly interact with it through notifications: insert a card, read what it says, click Ingest if there's something to copy. Everything else runs on its own.

Behind the scenes, negatives keeps an online catalog of every file, identified by its content rather than its name, along with where each verified copy lives. That's why it can tell you about any card, even one you copied years ago, and why your Mac and the Windows machine stay in agreement even when you ingest offline. Offload apps check a copy once, when it's made; negatives remembers every copy and keeps checking them.

## What you need

- **A Mac** with a working SSD formatted as APFS. Check with `diskutil info /Volumes/YourSSD` and look for "APFS".
- **A Windows machine** at home holding your primary and backup disks, shared to the Mac over SMB.
- **A Google account** and a Google Cloud project used only for negatives, with Firestore turned on. The first load of an existing archive costs a small one-time fee; everyday use fits the free tier.
- **smartmontools** on the Windows machine, for disk health checks.
- About a day or two per 20 TB for the first full hash of your existing archive.

## Installing on Windows

Set up the Windows machine first; the Mac relies on it.

1. **Install** the negatives Windows tools (archive-sync, archive-hasher, archive-scrub and the watcher) and smartmontools.
2. **Set up catalog backups** in your Google Cloud project: a Cloud Storage bucket and a weekly export job (see `docs/tools/catalog-backup.md` for the exact setup).
3. **Create a service account key** in your Google Cloud project with only Firestore access. Save the key file somewhere only the account that runs the task can read it. Don't sign in with your own Google account on this machine.
4. **Pack old Lightroom catalogs** (optional, but recommended): run `lrcat-pack <archive folder>` to see what it finds, then `--apply` to pack them. See Other tasks.
5. **Configure the Windows tools:** run `negatives config init`. It asks for your primary and backup disks, the service account key and the nightly run time, and checks them. Leave your existing folders where they are; just check that none of your old shoot folders already contains a folder named Originals or Derived.
6. **Check the disks match:** run `archive-sync --report` and fix any differences it lists by hand.
7. **Nothing else to create.** Each new shoot gets its own folder at the root of the disk, with `Originals` and `Derived` inside it, the first time it's backed up.
8. **Run the first full hash** by hand: `archive-hasher --full`. It takes days for a large archive, and you can stop and resume it.
9. **Turn on automation:** `negatives install-task` creates the scheduled task, the startup run, the nightly run and the watcher.
10. **Set disks to spin down:** in Power Options, set "Turn off hard disk after" to a few minutes.

## Installing on the Mac

1. **Install** the negatives Mac tools: the card-watch app and the command-line tools (card-check, ingest, ssd-push, ssd-evict).
2. **Sign in** with your Google account when card-watch first starts. The sign-in is kept in your Keychain.
3. **Mount the primary share** so it reconnects automatically after restarts; the next step needs it mounted.
4. **Configure the Mac tools:** run `negatives config init`. It finds your working SSD and the mounted archive share, checks the SSD is APFS, and writes the settings file.
5. **Put existing working folders in `Shoots`** on the SSD, with the same names they have on the archive. negatives backs up anything missing or changed and lets you free them like any other shoot.
6. **Keep Lightroom catalogs out of `Shoots`** if you still use Lightroom.
7. **Allow notifications** for card-watch when macOS asks, so you see card summaries.

Don't create the `Shoots` folder or shoot folders yourself; the first ingest creates them.

## Everyday use

1. **Insert the card.** A notification shows how many files are Safe, Pending and Not copied, and whether the card is safe to format. For a card negatives hasn't seen before, it just shows how many files there are, without reading them, so the copy is the card's only read.
2. **Click Ingest** if there's anything to copy. Type an optional description for the shoot (such as "Coast"); the folder becomes `2026-10-18 Coast`. On a card's first use since formatting, you're also asked for its card ID. If the card has files from an earlier ingest plus new ones, you're asked whether to continue that shoot or start a new one; click Preview to open the first new file and decide.
3. **Start editing** in `Shoots/<shoot>` on the SSD as soon as the done notification appears. Rate, add sidecars, render panoramas and videos into the shoot folder as usual; it all gets backed up.
4. **Leave the card alone for now.** In the background, your files are copied to the Windows machine, mirrored to the backup disk and verified. This needs the Windows machine to be on.
5. **Insert the card again later.** When the notification says it's safe to format, format it in the camera.

Add new folders to a shoot whenever you like, but don't move or rename existing folders: paths are meant to match the archive. If you do, nothing is lost, but you'll get a notification and the archive keeps the old names.

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

It works on whole shoot folders, including ones from before negatives, and needs a connection.

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
| Original changed (only with ssd-watch) | A locked original in a shoot's hidden `.originals` folder changed, which shouldn't happen. | If the card isn't formatted, ingest it again. |
| SSD isn't APFS | The working SSD has the wrong format. | Reformat it as APFS once everything on it is archived. |
| Catalog backup overdue | The weekly catalog export hasn't succeeded for about two weeks. | Check the export job in the Google Cloud console. Your photos are unaffected. |
| Not enough space | The card's new files don't fit on the SSD. | Run `ssd-evict` and try again. |
| Paths differ from archive | A folder in a shoot was moved or renamed on the SSD. | Nothing is lost. Move or rename it back to keep the paths matching; otherwise the archive keeps the old names. |
