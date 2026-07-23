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
    @State private var tab: Int

    init(auth: AuthStore, settings: SettingsStore) {
        _coordinator = StateObject(wrappedValue: BackupCoordinator(auth: auth, settings: settings))
        _library = StateObject(wrappedValue: LibraryStore(auth: auth))
        // Debug: open a specific tab for screenshots.
        let t = ProcessInfo.processInfo.environment["ASHEN_DEBUG_TAB"]
        _tab = State(initialValue: t == "library" ? 1 : (t == "storage" ? 2 : 0))
    }

    var body: some View {
        TabView(selection: $tab) {
            BackupView(coordinator: coordinator)
                .tabItem { Label("Backup", systemImage: "icloud.and.arrow.up") }
                .tag(0)
            LibraryView(store: library)
                .tabItem { Label("Library", systemImage: "photo.on.rectangle") }
                .tag(1)
            FreeSpaceView(coordinator: coordinator)
                .tabItem { Label("Storage", systemImage: "internaldrive") }
                .tag(2)
            SettingsView()
                .tabItem { Label("Settings", systemImage: "gear") }
                .tag(3)
        }
    }
}
