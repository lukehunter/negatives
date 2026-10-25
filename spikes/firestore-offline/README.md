# Spike: Firestore offline writes on macOS

Build order step 1 in [docs/design.md](../../docs/design.md#build-order). Throwaway: it proves
the Firebase Apple SDK queues a write made with the network off, keeps it across quitting
the app, and sends it after a relaunch with the network on.

It writes only to a temporary `spikes/{id}` collection (owner-only, create-only; see
`firestore/firestore.rules`). It never touches the catalog collections, and deletes nothing
in Firestore or on disk.

## What it checks

1. **Sign in** with Google (GoogleSignIn, then Firebase Auth) using
   `~/Library/Application Support/negatives/GoogleService-Info.plist`.
2. **Rules check, online:** writes `spikes/online-<uuid>` and waits for the server to accept it.
3. **Offline write:** with the network off, it confirms Firestore can't reach the server,
   writes `spikes/<uuid>`, then reads it back from the local cache (`hasPendingWrites = true`),
   so the write is in the on-disk queue before you quit. Optionally it also queues
   `spikes/<uuid>-reject`, which the rules refuse, to see what happens to a rejected queued write.
4. **Relaunch check, online:** on the next launch it turns Firestore's network off, reads the
   cache (the write should still be pending), turns the network back on, waits for pending
   writes, then reads `spikes/<uuid>` from the server and compares `runId`, `writerPid` and
   `writtenAt` with what the first process recorded.

The run is invalid, and the app says so, if the write syncs before you quit.

Files it keeps:

- `~/Library/Application Support/negatives-spike/spike.log` and `state.json`
- Firestore's cache: `~/Library/Application Support/firestore/negatives-spike/…` (a named
  Firebase app, so the real tools' default cache is never shared with the spike)

## Why it's an Xcode app

Firebase Auth on macOS always keeps the signed-in user in the data protection keychain.
That needs an app signed with a provisioning profile; an ad-hoc signed binary gets
`-34018 A required entitlement isn't present`. So the spike is an app with the Keychain
Sharing entitlement, signed by Xcode with your Personal Team. The real Mac tools will need
the same (see Findings).

`Package.swift` only typechecks the sources with the Command Line Tools (`swift build`);
a binary built that way can't sign in.

## Steps

### A. Install Xcode and add your Apple ID (once)

1. Open **App Store**, search for **Xcode**, click **Get**, then **Install**. It's a large
   download.
2. Open **Xcode**. Click **Agree** on the license and enter your Mac password if asked. If it
   shows a list of platforms to download, leave iOS and the others unticked and click
   **Download & Install** (or **Continue**): macOS support is built in.
3. Choose **Xcode › Settings…** (⌘,), open the **Accounts** tab, click **+** at the bottom
   left, choose **Apple Account**, click **Continue**, and sign in with your Apple ID. A team
   named **<your name> (Personal Team)** appears. Close Settings.

### B. Open, sign and run

4. From the repo root, generate the project and open it:

   ```bash
   spikes/firestore-offline/setup.sh && open spikes/firestore-offline/FirestoreOfflineSpike.xcodeproj
   ```

5. Wait until the status bar at the top of the Xcode window stops showing **Resolving Package
   Graph** / **Fetching** (a few minutes the first time).
6. In the left sidebar (Project navigator, ⌘1) click the blue **FirestoreOfflineSpike** project
   icon. In the middle pane, under **TARGETS**, click **FirestoreOfflineSpike**, then the
   **Signing & Capabilities** tab.
   - **Automatically manage signing:** ticked.
   - **Team:** choose **<your name> (Personal Team)**.
   - **Bundle Identifier:** `net.lukehunter.negatives` (leave it).
   - A **Keychain Sharing** section should list `net.lukehunter.negatives`.
   - If a **Register Device** button appears, click it. Any red error here must be gone before
     you continue.
7. In the toolbar, check the run destination reads **FirestoreOfflineSpike › My Mac**, then
   press **⌘R** (Product › Run). The first build compiles Firebase and takes a few minutes.
   If macOS asks for your login password so `codesign` can use your signing key, enter it and
   click **Always Allow**.

### C. First sign-in

8. In the app window, click **Sign in with Google**. When macOS asks whether the app may use
   **google.com** to sign in, click **Continue**.
9. Pick your Gmail account. If Google warns **Google hasn't verified this app** (the OAuth app
   is in Testing), click **Continue**; then click **Continue** on the access screen.
10. The window closes and section 1 shows your **UID**. The log says
    `Firebase sign-in OK. Your UID: …`. The UID is also in `spike.log`.

### D. Put the UID in the rules and deploy them

11. Replace `OWNER_UID` in `firestore/firestore.rules` with the UID.
12. Install the Firebase CLI (`brew install firebase-cli`) and sign in. Login opens the
    browser: choose the same Google account and click **Allow**; in the terminal, answer **n**
    to the usage-collection question.

    ```bash
    firebase login
    ```

13. Deploy, from the repo root (`firebase.json` points at `firestore/firestore.rules`, and
    `.firebaserc` at project `negatives-8c3e2`):

    ```bash
    firebase deploy --only firestore:rules
    ```

14. Check it in the [Firebase console](https://console.firebase.google.com/project/negatives-8c3e2/firestore/rules):
    **Firestore Database › Rules** shows the new rules with today's publish time.

### E. The offline test

15. Network **on**. In section 2, click **Write a test document online**. The log should say
    `rules check OK`. If it says *Missing or insufficient permissions*, the rules aren't
    deployed or the UID is wrong.
16. Turn the network **off**: click **Control Center** in the menu bar, then click **Wi-Fi** so it
    turns grey (off). Also unplug any Ethernet cable or USB/Thunderbolt network adapter, and
    disconnect iPhone tethering. The dot in section 3 turns red: **Network OFF**.
17. Leave **Also queue a write the rules will reject** ticked and click **Write test document
    offline**. The log should say `Firestore can't reach the server … Good.` and then
    `Queued spikes/<id> offline … Now quit with ⌘Q`.
18. Quit the app with **⌘Q**, with the network still off.
19. Turn **Wi-Fi** back on in Control Center and wait until it shows connected.
20. In Xcode press **⌘R** again. The app checks on its own; the log should end with
    `PASS: spikes/<id>, written offline by pid …, reached the server after a relaunch
    (in local cache after relaunch, hasPendingWrites = true)`.
21. Confirm in the [console](https://console.firebase.google.com/project/negatives-8c3e2/firestore/databases/-default-/data):
    **Firestore Database › Data**, collection **spikes**, document `<id>` has
    `note: "offline write"`. There's no `<id>-reject` document.

Optional second run, for a crash instead of a clean quit: click **Start over**, repeat steps
16–17, then click the **Stop** button (■) in Xcode instead of ⌘Q, and continue from step 19.

### F. After the spike

- Remove the `spikes` block from `firestore/firestore.rules` and deploy again (step 13).
- Free Personal Team profiles expire after about 7 days; Xcode renews them when you build.

## Findings

Confirmed while building the spike:

- **Mac tools need a provisioned app bundle.** Firebase Auth keeps the user in the data
  protection keychain (`kSecUseDataProtectionKeychain`, no fallback), which an ad-hoc signed
  binary can't use (`-34018`). Plain command-line tools can't sign in on their own; they must
  run inside, or be, an app signed with a provisioning profile and the Keychain Sharing
  entitlement.
- **One cache per Firebase app name.** Firestore's offline store lives in
  `~/Library/Application Support/firestore/<app name>/<project>/<database>/`, a LevelDB that
  one process holds at a time. Separate tool processes using the same app name would compete
  for it; with different names they wouldn't see each other's cache or queue. CatalogKit has
  to decide this (one long-running process, or one tool at a time).
- **The queue survives quitting and crashing, not necessarily power loss.** Firestore writes
  LevelDB with its default `sync = false`: the data is in the OS page cache when the write
  returns, so it survives the app exiting but not a kernel panic or power cut before the OS
  flushes it.
- **Queued writes belong to the signed-in user.** The relaunched app must be signed in as the
  same user before the queue is sent.

From the run on 2026-10-25 (Firebase Apple SDK 13.1.1, macOS 26.6.2, clean quit with ⌘Q):

- **PASS.** `spikes/1f5badd5-86da-45e3-83ba-a95b7a09f2ce` was written by pid 2974 with Wi-Fi
  off (Firestore itself reported *the client is offline*). After quitting and relaunching
  with Wi-Fi on, the new process (pid 3115) found it in the local cache with
  `hasPendingWrites = true`. Firestore sent it, and a server read matched `runId`,
  `writerPid` and `writtenAt`.
- **Fast:** pending writes were acknowledged 0.7 s after the check started, about 1 s after
  launch.
- **A rejected queued write disappears without telling the app.** The `-reject` probe never
  reached the server. The relaunched process had no callback or error for it; the only trace
  is a default-level unified log line from the SDK
  (`Write at spikes/…-reject failed: Missing or insufficient permissions`). So the Mac tools
  must never rely on the queue to report a rule violation: validate documents against the
  schema before writing, and re-read later to confirm that facts landed.

Crash variant, same day: **PASS.** `spikes/1172bb46-3034-431a-9f94-e519c136a1af` was queued
offline by pid 3115, which Xcode's Stop then killed 13 s later (the unified log shows
debugserver sending `PT_KILL`, so no shutdown code ran). After relaunching with Wi-Fi on,
the write was still pending in the cache, was acknowledged in 0.6 s, and matched on the
server. The reject probe was again dropped silently. So once a cache read returns the
document, the queued write survives the process being killed. Power loss is still untested
(see `sync = false` above).
