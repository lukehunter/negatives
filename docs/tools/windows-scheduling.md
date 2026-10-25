# Windows credentials and scheduling

How the Windows tools authenticate and when they run. archive-sync, archive-hasher and archive-scrub run in sequence from one scheduled task on the machine that hosts both archive disks. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

## Credentials

- A dedicated service account with only the Firestore user role, in a GCP project used for nothing else. Its key is readable only by the account that runs the task.
- No personal Google credentials on this machine.

## Scheduling

- Triggered by writes, not a timer: a small always-running watcher (ReadDirectoryChangesW on the primary root) waits until 10 minutes pass with no new writes, then starts the task, with "Do not start a new instance" if a run is still going.
- Watching costs no disk activity. Disks spin only when ssd-push is already writing to them, and for the run that follows.
- Safety net: one full run every night regardless, so a missed notification (an overflowed event buffer, a crashed watcher, a reboot) costs at most a day.
- Manual trigger: from the Mac, `card-check --sync-now` writes a request to `status/sync-request`. The watcher listens for it, starts the task right away (skipping the 10-minute wait), and clears the request. On the Windows machine itself, `schtasks /run /tn negatives` does the same.
- The watcher writes a heartbeat to the status document every few hours without touching the disks, so the Mac can tell "alive, nothing new" from "watcher down".
- Set the Windows power plan's "Turn off hard disk after" so idle disks actually spin down.
- The machine isn't always on, so the task also runs at startup, and the nightly run uses "Run task as soon as possible after a scheduled start is missed". When the heartbeat stops, the Mac shows the host as offline since its last heartbeat instead of raising an alarm.
- The first full hash of the existing archive runs by hand, outside the scheduler, to avoid the default 3-day task time limit. At typical hard-disk read speeds, expect very roughly a day or two per 20 TB; the two disks are hashed in parallel, and progress is saved so the run can stop and resume.
