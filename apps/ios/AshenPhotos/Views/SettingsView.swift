import SwiftUI
import Photos

struct SettingsView: View {
    @EnvironmentObject private var auth: AuthStore
    @EnvironmentObject private var settings: SettingsStore
    @State private var limited = PhotoScanner.isLimited

    var body: some View {
        NavigationStack {
            Form {
                Section("Photos") {
                    if limited {
                        Text("Ashen can only see the photos you selected.")
                            .font(.footnote).foregroundStyle(.secondary)
                        Button {
                            PhotoScanner.presentLimitedPicker()
                        } label: {
                            Label("Select more photos", systemImage: "photo.badge.plus")
                        }
                    } else {
                        Text("Full photo library access.")
                            .font(.footnote).foregroundStyle(.secondary)
                    }
                }
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
