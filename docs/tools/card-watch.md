# card-watch

Card mount trigger. Part of [negatives](../design.md). Shared concepts live in the design doc: [data model](../design.md#data-model), [folder layout](../design.md#folder-layout) and [safety rules](../design.md#safety-rules-and-failure-modes).

- A launchd agent that runs when any volume mounts and acts only on cards: removable volumes with a DCIM or PRIVATE folder at the root.
- Runs card-check automatically. It's read-only, so it's always safe to run unasked.
- For a card negatives has never seen (no matching card session), it doesn't hash anything. It posts "New card: N files, X GB" right away, so ingest does the only read of the card.
- Posts a notification summarizing the card: how many files are Safe, Pending and Not copied, and whether it's safe to format.
- When there are files to copy, the notification has an Ingest button. It asks for the optional shoot description, then runs ingest and posts a second notification when it's done. ingest never starts without that click.
- Notifications with buttons need a small app bundle rather than a bare command-line tool, so card-watch ships as a minimal background app.
