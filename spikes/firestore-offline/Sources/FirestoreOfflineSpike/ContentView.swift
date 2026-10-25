import AppKit
import SwiftUI

struct ContentView: View {
    @ObservedObject var spike: Spike

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            GroupBox("1 · Sign in") {
                HStack {
                    if let uid = spike.uid {
                        Text("UID").foregroundStyle(.secondary)
                        Text(uid).monospaced().textSelection(.enabled)
                        Button("Copy UID") {
                            NSPasteboard.general.clearContents()
                            NSPasteboard.general.setString(uid, forType: .string)
                        }
                        Spacer()
                        Text(spike.email ?? "").foregroundStyle(.secondary)
                        Button("Sign out") { spike.signOut() }
                    } else {
                        Button("Sign in with Google") { spike.signIn() }
                            .disabled(!spike.configured || spike.busy)
                        Spacer()
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            GroupBox("2 · Rules check (network on)") {
                HStack {
                    Button("Write a test document online") { spike.checkRules() }
                        .disabled(spike.uid == nil || spike.networkUp != true || spike.busy)
                    Text("Confirms the deployed rules accept spike writes.")
                        .foregroundStyle(.secondary)
                    Spacer()
                }
            }

            GroupBox("3 · Offline write (network off)") {
                VStack(alignment: .leading, spacing: 6) {
                    HStack {
                        Circle()
                            .fill(spike.networkUp == true ? Color.green : Color.red)
                            .frame(width: 10, height: 10)
                        Text(spike.networkUp == true ? "Network ON" : "Network OFF")
                        Spacer()
                    }
                    Toggle("Also queue a write the rules will reject", isOn: $spike.rejectProbe)
                    Button("Write test document offline") { spike.writeOffline() }
                        .disabled(spike.uid == nil || spike.networkUp != false || spike.busy
                                  || spike.awaitingRelaunch)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            GroupBox("4 · Relaunch check (network on)") {
                VStack(alignment: .leading, spacing: 6) {
                    Text(spike.stage).fixedSize(horizontal: false, vertical: true)
                    HStack {
                        Button("Check now") { spike.checkAfterRelaunch() }
                            .disabled(spike.run == nil || spike.busy)
                        Button("Start over") { spike.startOver() }
                            .disabled(spike.run == nil || spike.busy)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            Text("Log  ·  \(Spike.logURL.path)").font(.caption).foregroundStyle(.secondary)
            ScrollView {
                Text(spike.lines.joined(separator: "\n"))
                    .font(.system(.caption, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .defaultScrollAnchor(.bottom)
            .background(Color(nsColor: .textBackgroundColor))
        }
        .padding()
    }
}
