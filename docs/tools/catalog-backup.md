# catalog-backup

Weekly export of the Firestore catalog to Cloud Storage, and how to rebuild from it. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

The archive disks are the real backup of your photos; this protects the catalog. Archive facts could be rebuilt by re-hashing both disks (days), but card sessions, sightings and `ssd` facts exist only in the catalog.

## Export

- **Mechanism:** Firestore's managed export (`exportDocuments`), which writes the whole database to a Cloud Storage bucket. It runs entirely in Google Cloud, so it works even when the Mac and the Windows machine are off.
- **Trigger:** a Cloud Scheduler job, weekly, calling a small Cloud Run function that starts the export. This is Google's recommended pattern; a direct Scheduler call to the Firestore API may also work and would remove the function.
- **Permissions:** the function's own service account gets only the Cloud Datastore Import Export Admin role and write access to the backup bucket. The Windows service account key is not involved, so a leaked Windows key can't touch backups.
- **Bucket:** in the same region as the Firestore database, with:
  - a lifecycle rule deleting exports older than 8 weeks;
  - soft delete or a retention policy, so a mistake or a leaked credential can't wipe existing backups.
- **Status:** the function records the export's start, finish and outcome in `status/catalog-backup`. The Mac warns when the last successful export is older than about two weeks.
- **Cost:** an export is billed like reading every document once. At a few million documents that's roughly a dollar per export at current US pricing, so weekly costs a few dollars a month; daily would cost about seven times that. Storage for the exports is small.

## Restore

1. **Import** the newest good export into an empty Firestore database (`gcloud firestore import`). Exports can also be imported into a separate database to inspect them first.
2. **Reconcile the archive:** run `archive-hasher --reconcile`. It walks both disks, re-hashes any file whose size or mtime doesn't match its facts, and adds anything written since the export. Unlike a full hash, it only reads files that changed.
3. **Reconcile the Mac:** run `ssd-push --reconcile`. It re-records `ssd` facts for every shoot folder on the SSD, from `.originals/` and the working copies.
4. **Accept the small loss:** card sightings and card sessions created since the export are gone. A re-inserted card simply starts a new session, and nothing becomes falsely Safe, because Safe always comes from archive facts.
