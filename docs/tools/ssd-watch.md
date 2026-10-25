# ssd-watch

Optional alerts. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Optional: build it only if you want an extra alarm. ssd-push and ssd-evict already re-hash `.originals/` copies before relying on them.
- A launchd agent that subscribes to FSEvents on each shoot's `.originals/` folder only. Those copies are locked and never opened by apps, so any change there means corruption or a cleared lock.
- Posts one macOS notification per shoot folder and records a modified flag on the file's `ssd` entry, so card-check and ssd-evict show it even if the notification is missed.
- Changes to working copies in `Shoots/` are expected (ratings, metadata) and become derived files through ssd-push, not alerts.
