# ssd-evict

Guided cleanup, only when you run it. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- Never runs on its own. Nothing is freed unless you start it and confirm.
- Lists the SSD grouped by shoot folder, with each group's size, status (Safe or Pending), and the space it would free.
- Works on whole shoot folders only: removing a shoot removes its one shoot folder, including `.originals/`. A shoot can be removed only when every original in it is Safe, and every working or derived file is either unchanged from a Safe original or Safe under Derived/.
- You choose what to remove. Only groups where every file is Safe can be selected; Pending groups are shown, with the reason they can't be selected yet.
- Before deleting, it re-checks every selected file online: it re-hashes the file and confirms that hash has verified primary and backup entries in a fresh catalog. A working copy changed since its version was archived is skipped and listed.
- Clears the lock (chflags nouchg) on each confirmed file in `.originals/` right before deleting it.
- Shows a final summary (files, size, space freed) and asks for confirmation, then deletes and removes the `ssd` facts.
- Offline or with a stale catalog, it lists only and deletes nothing.
- Holds each shoot's lock from the final re-check through the deletion, so ssd-push can't be copying from a shoot while it's being removed. A shoot that's locked by another tool is shown as busy and skipped.
- **Legacy shoot folders:** a folder without `.originals/` is checked by hash alone and listed alongside new-style shoots. It can be removed once every file in it is Safe somewhere in the archive.
- A shoot whose paths no longer match the archive (a folder was moved or renamed) is marked "paths differ from archive". It can still be removed once everything is Safe by hash; restoring it later brings back the archive's paths.
