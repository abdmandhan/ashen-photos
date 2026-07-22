import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var auth: AuthStore
    @EnvironmentObject private var settings: SettingsStore

    var body: some View {
        NavigationStack {
            Form {
                Section("Upload conditions") {
                    Toggle("Wi-Fi only", isOn: $settings.wifiOnly)
                    Toggle("Charging only", isOn: $settings.chargingOnly)
                }
                Section("Network") {
                    HStack {
                        Text("Wi-Fi")
                        Spacer()
                        Text(settings.onWifi ? "Connected" : "Off")
                            .foregroundStyle(.secondary)
                    }
                }
                Section("Account") {
                    if let uid = auth.userID {
                        Text(uid).font(.footnote).foregroundStyle(.secondary)
                    }
                    Button("Log out", role: .destructive) { auth.logout() }
                }
            }
            .navigationTitle("Settings")
        }
    }
}
