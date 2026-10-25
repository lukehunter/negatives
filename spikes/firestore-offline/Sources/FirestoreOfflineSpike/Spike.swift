import AppKit
import FirebaseAuth
import FirebaseCore
import FirebaseFirestore
import Foundation
import GoogleSignIn
import Network
import SystemConfiguration

/// Build order step 1: does a Firestore write made with the network off survive
/// quitting the app, and reach the server after a relaunch with the network on?
///
/// Throwaway. It writes only to `spikes/{id}`, which the temporary rule in
/// firestore/firestore.rules allows; it never touches the catalog collections.
@MainActor
final class Spike: ObservableObject {
    /// A named Firebase app, so the spike's Firestore cache
    /// (~/Library/Application Support/firestore/negatives-spike/…) is kept apart
    /// from the default one the real tools will use.
    static let appName = "negatives-spike"
    static let supportDir = FileManager.default.homeDirectoryForCurrentUser
        .appending(path: "Library/Application Support", directoryHint: .isDirectory)
    static let plistURL = supportDir.appending(path: "negatives/GoogleService-Info.plist")
    static let spikeDir = supportDir.appending(path: "negatives-spike", directoryHint: .isDirectory)
    static let stateURL = spikeDir.appending(path: "state.json")
    static let logURL = spikeDir.appending(path: "spike.log")

    /// One offline write, persisted so the relaunched process knows what to check.
    struct Run: Codable {
        var runId: String
        var uid: String
        var writtenAt: Date
        var writerPid: Int32
        var rejectProbe: Bool
        var syncedBeforeQuit = false
        var result: String?
    }

    @Published private(set) var lines: [String] = []
    @Published private(set) var uid: String?
    @Published private(set) var email: String?
    @Published private(set) var networkUp: Bool?
    @Published private(set) var run: Run?
    @Published private(set) var busy = false
    @Published var rejectProbe = true

    private(set) var configured = false
    private var db: Firestore!
    private var auth: Auth!
    private let monitor = NWPathMonitor()
    private var authHandle: AuthStateDidChangeListenerHandle?
    private var listener: ListenerRegistration?
    private var checkedThisLaunch = false
    private let pid = ProcessInfo.processInfo.processIdentifier

    init() {
        try? FileManager.default.createDirectory(at: Self.spikeDir, withIntermediateDirectories: true)
        log("---- launch, pid \(pid)")
        loadState()
        configure()
        startMonitor()
    }

    var awaitingRelaunch: Bool { run != nil && run?.result == nil }

    var stage: String {
        guard let run else { return "No offline write yet." }
        if let result = run.result { return result }
        if run.syncedBeforeQuit {
            return "Invalid run: the write reached the server before you quit. Click Start over."
        }
        if run.writerPid == pid {
            return "spikes/\(run.runId) is queued offline. Quit now with ⌘Q, turn the network on, then relaunch."
        }
        return "Relaunched. Waiting for sign-in and network to check spikes/\(run.runId)…"
    }

    // MARK: Setup

    private func configure() {
        guard let options = FirebaseOptions(contentsOfFile: Self.plistURL.path),
              let clientID = options.clientID else {
            log("ERROR: can't read \(Self.plistURL.path), or it has no CLIENT_ID")
            return
        }
        FirebaseApp.configure(name: Self.appName, options: options)
        guard let app = FirebaseApp.app(name: Self.appName) else { return }
        auth = Auth.auth(app: app)
        db = Firestore.firestore(app: app)
        let settings = db.settings
        settings.cacheSettings = PersistentCacheSettings()
        db.settings = settings
        GIDSignIn.sharedInstance.configuration = GIDConfiguration(clientID: clientID)
        configured = true
        log("Firebase configured: project \(options.projectID ?? "?"), app \(Self.appName)")

        authHandle = auth.addStateDidChangeListener { [weak self] _, user in
            MainActor.assumeIsolated { self?.authChanged(user) }
        }
    }

    private func startMonitor() {
        monitor.pathUpdateHandler = { [weak self] path in
            let up = path.status == .satisfied
            Task { @MainActor in self?.networkChanged(up) }
        }
        monitor.start(queue: .main)
    }

    private func authChanged(_ user: User?) {
        uid = user?.uid
        email = user?.email
        if let user {
            log("signed in: uid \(user.uid) (\(user.email ?? "no email"))")
        } else {
            log("not signed in")
        }
        maybeCheckAfterRelaunch()
    }

    private func networkChanged(_ up: Bool) {
        guard up != networkUp else { return }
        networkUp = up
        log("network \(up ? "ON" : "OFF") (macOS path monitor)")
        maybeCheckAfterRelaunch()
    }

    // MARK: Actions

    func signIn() {
        guard let window = NSApp.keyWindow ?? NSApp.windows.first else {
            log("ERROR: no window to present sign-in from")
            return
        }
        work("sign in") {
            let result = try await GIDSignIn.sharedInstance.signIn(withPresenting: window)
            guard let idToken = result.user.idToken?.tokenString else {
                throw SpikeError("Google returned no ID token")
            }
            let credential = GoogleAuthProvider.credential(
                withIDToken: idToken, accessToken: result.user.accessToken.tokenString)
            let authResult = try await self.auth.signIn(with: credential)
            self.log("Firebase sign-in OK. Your UID: \(authResult.user.uid)")
        }
    }

    func signOut() {
        do {
            try auth.signOut()
            GIDSignIn.sharedInstance.signOut()
        } catch {
            log("ERROR (sign out): \(describe(error))")
        }
    }

    /// Online write with a server acknowledgement: proves the deployed rules accept spike documents.
    func checkRules() {
        work("rules check") {
            let id = "online-" + Self.newId()
            let ref = self.db.collection("spikes").document(id)
            let data = self.fields(runId: id, writtenAt: Date(), note: "online rules check")
            try await self.withTimeout(30, "server write") { done in
                ref.setData(data) { done($0.map { .failure($0) } ?? .success(())) }
            }
            self.log("rules check OK: server accepted spikes/\(id)")
        }
    }

    func writeOffline() {
        guard networkUp == false else {
            log("Turn the network off first.")
            return
        }
        guard let uid else { return }
        work("offline write") {
            // macOS says the network is off; make sure Firestore can't reach the server either.
            let probe = self.db.collection("spikes").document("probe")
            do {
                _ = try await self.withTimeout(20, "server probe") { done in
                    probe.getDocument(source: .server) { snap, error in
                        done(snap.map { .success($0) } ?? .failure(error ?? SpikeError("no snapshot")))
                    }
                }
                throw SpikeError("Firestore still reached the server. Turn off Wi-Fi and every other network, then try again.")
            } catch let error as SpikeError where !error.isTimeout {
                throw error
            } catch {
                self.log("Firestore can't reach the server (\(self.describe(error))). Good.")
            }

            let runId = Self.newId()
            let writtenAt = Date()
            let ref = self.db.collection("spikes").document(runId)
            // The completion only fires when the server acknowledges the write.
            ref.setData(self.fields(runId: runId, writtenAt: writtenAt, note: "offline write")) { [weak self] error in
                Task { @MainActor in
                    self?.log(error.map { "server rejected spikes/\(runId) in the writing process: \(self?.describe($0) ?? "")" }
                              ?? "server acknowledged spikes/\(runId) in the writing process")
                }
            }
            if self.rejectProbe {
                var bad = self.fields(runId: runId, writtenAt: writtenAt, note: "write the rules reject")
                bad["notAllowed"] = true
                self.db.collection("spikes").document(runId + "-reject").setData(bad) { [weak self] error in
                    Task { @MainActor in
                        self?.log("reject probe answered in the writing process: \(error.map { self?.describe($0) ?? "" } ?? "accepted?!")")
                    }
                }
            }

            // A cache read is queued behind the write, so once it returns the write is in the
            // local LevelDB store and will survive the app quitting.
            let snap = try await ref.getDocument(source: .cache)
            guard snap.exists, snap.metadata.hasPendingWrites else {
                throw SpikeError("the write isn't in the local cache as a pending write")
            }
            self.run = Run(runId: runId, uid: uid, writtenAt: writtenAt, writerPid: self.pid,
                           rejectProbe: self.rejectProbe)
            self.saveState()
            self.watchForEarlySync(ref)
            self.log("Queued spikes/\(runId) offline; it's in the local cache with hasPendingWrites = true.")
            self.log("Now quit with ⌘Q (don't turn the network on first).")
        }
    }

    func checkAfterRelaunch() {
        guard let run = self.run else { return }
        guard run.writerPid != pid else {
            log("Quit and relaunch first: this is the process that made the write.")
            return
        }
        guard uid == run.uid else {
            log("Sign in as \(run.uid) first: Firestore keeps queued writes per user.")
            return
        }
        guard networkUp == true else {
            log("Turn the network on first.")
            return
        }
        checkedThisLaunch = true
        work("relaunch check") {
            let ref = self.db.collection("spikes").document(run.runId)
            let started = Date()

            // Look at the local store before the queued write can be sent.
            try await self.db.disableNetwork()
            var cacheState: String
            do {
                let cached = try await ref.getDocument(source: .cache)
                cacheState = cached.exists
                    ? "in local cache after relaunch, hasPendingWrites = \(cached.metadata.hasPendingWrites)"
                    : "NOT in local cache after relaunch"
            } catch {
                cacheState = "NOT in local cache after relaunch (\(self.describe(error)))"
            }
            self.log(cacheState)

            try await self.db.enableNetwork()
            try await self.withTimeout(120, "waiting for pending writes") { done in
                self.db.waitForPendingWrites { done($0.map { .failure($0) } ?? .success(())) }
            }
            let syncSeconds = Date().timeIntervalSince(started)
            self.log(String(format: "pending writes acknowledged %.1fs after the check started", syncSeconds))

            var failures: [String] = []
            let server = try await ref.getDocument(source: .server)
            if !server.exists {
                failures.append("spikes/\(run.runId) is not on the server")
            } else {
                let data = server.data() ?? [:]
                if data["runId"] as? String != run.runId { failures.append("runId differs") }
                if (data["writerPid"] as? NSNumber)?.int32Value != run.writerPid { failures.append("writerPid differs") }
                let written = (data["writtenAt"] as? Timestamp)?.dateValue()
                if written.map({ abs($0.timeIntervalSince(run.writtenAt)) > 0.001 }) ?? true {
                    failures.append("writtenAt differs")
                }
            }

            var rejectNote = ""
            if run.rejectProbe {
                let rejected = try await self.db.collection("spikes").document(run.runId + "-reject")
                    .getDocument(source: .server)
                rejectNote = rejected.exists
                    ? " Reject probe: UNEXPECTEDLY on the server."
                    : " Reject probe: not on the server, and nothing in this process was told (dropped silently)."
                if rejected.exists { failures.append("the reject probe reached the server") }
            }

            let result = failures.isEmpty
                ? "PASS: spikes/\(run.runId), written offline by pid \(run.writerPid), reached the server after a relaunch (\(cacheState)).\(rejectNote)"
                : "FAIL: \(failures.joined(separator: "; ")). \(cacheState).\(rejectNote)"
            var finished = run
            finished.result = result
            self.run = finished
            self.saveState()
            self.log(result)
        }
    }

    func startOver() {
        listener?.remove()
        listener = nil
        run = nil
        checkedThisLaunch = false
        try? FileManager.default.removeItem(at: Self.stateURL)
        log("state cleared")
    }

    // MARK: Helpers

    private func maybeCheckAfterRelaunch() {
        guard let run, run.result == nil, !run.syncedBeforeQuit, run.writerPid != pid,
              uid == run.uid, networkUp == true, !checkedThisLaunch, !busy else { return }
        log("relaunched with a queued write from pid \(run.writerPid); checking it")
        checkAfterRelaunch()
    }

    /// If the network comes back before the app quits, the write syncs in this process
    /// and the run no longer tests anything.
    private func watchForEarlySync(_ ref: DocumentReference) {
        listener = ref.addSnapshotListener(includeMetadataChanges: true) { [weak self] snap, _ in
            MainActor.assumeIsolated {
                guard let self, let snap, snap.exists, !snap.metadata.hasPendingWrites,
                      var run = self.run, run.writerPid == self.pid else { return }
                run.syncedBeforeQuit = true
                self.run = run
                self.saveState()
                self.log("INVALID: the write synced before you quit. Click Start over.")
            }
        }
    }

    private func fields(runId: String, writtenAt: Date, note: String) -> [String: Any] {
        [
            "schema": 1,
            "runId": runId,
            "writtenAt": Timestamp(date: writtenAt),
            "writerPid": Int(pid),
            "host": Self.computerName,
            "note": note,
        ]
    }

    private func work(_ name: String, _ body: @escaping @MainActor () async throws -> Void) {
        busy = true
        Task {
            do {
                try await body()
            } catch {
                log("ERROR (\(name)): \(describe(error))")
            }
            busy = false
        }
    }

    /// Firestore's completion-handler calls never return while offline, so wait with a deadline.
    private func withTimeout<T>(_ seconds: Double, _ what: String,
                                _ start: (@escaping (Result<T, Error>) -> Void) -> Void) async throws -> T {
        try await withCheckedThrowingContinuation { continuation in
            let once = Once()
            start { result in once.run { continuation.resume(with: result) } }
            DispatchQueue.main.asyncAfter(deadline: .now() + seconds) {
                once.run { continuation.resume(throwing: SpikeError.timeout(what, seconds)) }
            }
        }
    }

    private func describe(_ error: Error) -> String {
        let ns = error as NSError
        var text = "\(ns.localizedDescription) [\(ns.domain) \(ns.code)]"
        if let underlying = ns.userInfo[NSUnderlyingErrorKey] as? NSError {
            text += " underlying: \(underlying.localizedDescription) [\(underlying.domain) \(underlying.code)]"
        }
        if let reason = ns.userInfo[NSLocalizedFailureReasonErrorKey] as? String { text += " reason: \(reason)" }
        return text
    }

    private func log(_ message: String) {
        let line = "\(Date().formatted(.iso8601)) \(message)"
        lines.append(line)
        guard let data = (line + "\n").data(using: .utf8) else { return }
        if let handle = try? FileHandle(forWritingTo: Self.logURL) {
            handle.seekToEndOfFile()
            handle.write(data)
            try? handle.close()
        } else {
            try? data.write(to: Self.logURL)
        }
    }

    private func loadState() {
        guard let data = try? Data(contentsOf: Self.stateURL) else { return }
        run = try? JSONDecoder().decode(Run.self, from: data)
        if let run { log("state: run \(run.runId) from pid \(run.writerPid), result \(run.result ?? "none yet")") }
    }

    private func saveState() {
        guard let run, let data = try? JSONEncoder().encode(run) else { return }
        try? data.write(to: Self.stateURL, options: .atomic)
    }

    private static func newId() -> String { UUID().uuidString.lowercased() }

    /// ProcessInfo.hostName can block on DNS while offline; the computer name can't.
    private static let computerName = SCDynamicStoreCopyComputerName(nil, nil) as String? ?? "unknown"
}

struct SpikeError: LocalizedError {
    let message: String
    var isTimeout = false
    init(_ message: String) { self.message = message }
    static func timeout(_ what: String, _ seconds: Double) -> SpikeError {
        var error = SpikeError("\(what) got no answer in \(Int(seconds))s")
        error.isTimeout = true
        return error
    }
    var errorDescription: String? { message }
}

private final class Once: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false
    func run(_ body: () -> Void) {
        lock.lock()
        let first = !done
        done = true
        lock.unlock()
        if first { body() }
    }
}
