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
    @StateObject private var library: LibraryStore

    init(auth: AuthStore, settings: SettingsStore) {
        _coordinator = StateObject(wrappedValue: BackupCoordinator(auth: auth, settings: settings))
        _library = StateObject(wrappedValue: LibraryStore(auth: auth))
    }

    var body: some View {
        TabView {
            BackupView(coordinator: coordinator)
                .tabItem { Label("Backup", systemImage: "icloud.and.arrow.up") }
            LibraryView(store: library)
                .tabItem { Label("Library", systemImage: "photo.on.rectangle") }
            SettingsView()
                .tabItem { Label("Settings", systemImage: "gear") }
        }
    }
}
