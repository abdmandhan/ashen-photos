import SwiftUI

struct BackupView: View {
    @ObservedObject var coordinator: BackupCoordinator
    @EnvironmentObject private var settings: SettingsStore

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                progress

                VStack(spacing: 8) {
                    stat("Backed up", coordinator.done, .green)
                    stat("Remaining", coordinator.remaining, .blue)
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

                Button {
                    Task { await coordinator.run() }
                } label: {
                    Label(coordinator.running ? "Backing up…" : "Back up now",
                          systemImage: "icloud.and.arrow.up")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .disabled(coordinator.running)

                Spacer()
            }
            .padding()
            .navigationTitle("Backup")
            .task { await coordinator.run() }
        }
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
