import SwiftUI

struct BackupView: View {
    @ObservedObject var coordinator: BackupCoordinator
    @EnvironmentObject private var settings: SettingsStore

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 20) {
                    progress

                    VStack(spacing: 8) {
                        stat("Uploaded", coordinator.uploaded, .green)
                        stat("Already saved", coordinator.skipped, .teal)
                        stat("Uploading", coordinator.uploading, .blue)
                        stat("Remaining", coordinator.remaining, .gray)
                        if coordinator.failed > 0 { stat("Failed", coordinator.failed, .red) }
                    }

                    Text(coordinator.statusLine)
                        .font(.footnote)
                        .foregroundStyle(.secondary)

                    if !settings.canUpload {
                        Label("Waiting for \(settings.wifiOnly ? "Wi-Fi" : "power")",
                              systemImage: "pause.circle")
                            .font(.footnote)
                            .foregroundStyle(.orange)
                    }

                    HStack(spacing: 12) {
                        Button {
                            Task { await coordinator.run() }
                        } label: {
                            Label(coordinator.running ? "Backing up…" : "Back up now",
                                  systemImage: "icloud.and.arrow.up")
                                .frame(maxWidth: .infinity)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(coordinator.running || coordinator.paused)

                        if coordinator.paused {
                            Button {
                                coordinator.resume()
                            } label: {
                                Label("Resume", systemImage: "play.fill").frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                        } else {
                            Button {
                                coordinator.pause()
                            } label: {
                                Label("Pause", systemImage: "pause.fill").frame(maxWidth: .infinity)
                            }
                            .buttonStyle(.bordered)
                            .disabled(!coordinator.running && coordinator.remaining == 0)
                        }
                    }

                    if !coordinator.failedItems.isEmpty {
                        failedSection
                    }
                }
                .padding()
            }
            .navigationTitle("Backup")
            .task { await coordinator.run() }
        }
    }

    private var failedSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Failed (\(coordinator.failedItems.count))")
                    .font(.headline)
                Spacer()
                Button("Retry all") { Task { await coordinator.retryFailedNow() } }
                    .font(.footnote)
                    .disabled(coordinator.running)
            }
            ForEach(coordinator.failedItems) { item in
                VStack(alignment: .leading, spacing: 2) {
                    HStack {
                        Image(systemName: item.mediaType == "video" ? "video" : (item.mediaType == "live" ? "livephoto" : "photo"))
                            .foregroundStyle(.secondary)
                        Text(shortID(item.id))
                            .font(.subheadline.monospaced())
                        Spacer()
                        if item.retryCount > 0 {
                            Text("retried \(item.retryCount)×")
                                .font(.caption2)
                                .foregroundStyle(.secondary)
                        }
                    }
                    Text(item.errorMessage ?? "Unknown error")
                        .font(.caption)
                        .foregroundStyle(.red)
                }
                .padding(10)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Color.red.opacity(0.08))
                .clipShape(RoundedRectangle(cornerRadius: 8))
            }
        }
    }

    private func shortID(_ id: String) -> String {
        String(id.prefix(8)) + "…"
    }

    private var progress: some View {
        let fraction = coordinator.total == 0 ? 0 : Double(coordinator.done) / Double(coordinator.total)
        return VStack {
            ProgressView(value: fraction)
            Text("\(coordinator.done) / \(coordinator.total)")
                .font(.title3.monospacedDigit())
        }
    }

    private func stat(_ label: String, _ value: Int, _ color: Color) -> some View {
        HStack {
            Circle().fill(color).frame(width: 8, height: 8)
            Text(label)
            Spacer()
            Text("\(value)").monospacedDigit()
        }
    }
}
