import AppKit
import GoogleSignIn
import SwiftUI

@main
struct SpikeApp: App {
    @NSApplicationDelegateAdaptor private var delegate: AppDelegate
    @StateObject private var spike = Spike()

    var body: some Scene {
        Window("Firestore offline spike", id: "main") {
            ContentView(spike: spike)
                .frame(minWidth: 720, minHeight: 620)
                .onOpenURL { url in _ = GIDSignIn.sharedInstance.handle(url) }
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
}
