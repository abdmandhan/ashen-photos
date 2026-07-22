import SwiftUI

struct RootView: View {
    @EnvironmentObject private var auth: AuthStore
    @EnvironmentObject private var settings: SettingsStore

    var body: some View {
        if auth.isAuthenticated {
            MainView(auth: auth, settings: settings)
        } else {
            LoginView()
        }
    }
}

struct MainView: View {
    @StateObject private var coordinator: BackupCoordinator

    init(auth: AuthStore, settings: SettingsStore) {
        _coordinator = StateObject(wrappedValue: BackupCoordinator(auth: auth, settings: settings))
    }

    var body: some View {
        TabView {
            BackupView(coordinator: coordinator)
                .tabItem { Label("Backup", systemImage: "icloud.and.arrow.up") }
            SettingsView()
                .tabItem { Label("Settings", systemImage: "gear") }
        }
    }
}
