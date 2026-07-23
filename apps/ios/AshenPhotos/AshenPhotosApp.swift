import SwiftUI
import BackgroundTasks
import UIKit

@main
struct AshenPhotosApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var auth = AuthStore()
    @StateObject private var settings = SettingsStore()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(auth)
                .environmentObject(settings)
        }
    }
}

/// Bridges background URLSession relaunch events + schedules background backup.
final class AppDelegate: NSObject, UIApplicationDelegate {
    static let bgTaskID = "test.ashen.photos.backup"
    private var keepAliveTask = UIBackgroundTaskIdentifier.invalid

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        // Register the background backup handler (dispatches new uploads when iOS grants time).
        BGTaskScheduler.shared.register(forTaskWithIdentifier: Self.bgTaskID, using: nil) { task in
            AppDelegate.handleBackupTask(task as! BGProcessingTask)
        }
        return true
    }

    func applicationDidEnterBackground(_ application: UIApplication) {
        Self.scheduleBackupTask()
        // Short extension so the current in-flight batch can finish dispatching.
        keepAliveTask = application.beginBackgroundTask(withName: "ashen.finish-batch") { [weak self] in
            self?.endKeepAlive()
        }
        Task {
            try? await Task.sleep(nanoseconds: 25_000_000_000)
            await MainActor.run { self.endKeepAlive() }
        }
    }

    private func endKeepAlive() {
        if keepAliveTask != .invalid {
            UIApplication.shared.endBackgroundTask(keepAliveTask)
            keepAliveTask = .invalid
        }
    }

    /// Schedules the next opportunistic background backup (iOS decides timing).
    static func scheduleBackupTask() {
        let req = BGProcessingTaskRequest(identifier: bgTaskID)
        req.requiresNetworkConnectivity = true
        req.requiresExternalPower = false
        try? BGTaskScheduler.shared.submit(req)
    }

    static func handleBackupTask(_ task: BGProcessingTask) {
        scheduleBackupTask() // chain the next window
        let work = Task { @MainActor in
            await BackupCoordinator.runBackgroundBackup()
            task.setTaskCompleted(success: true)
        }
        task.expirationHandler = { work.cancel() }
    }

    func application(_ application: UIApplication,
                     handleEventsForBackgroundURLSession identifier: String,
                     completionHandler: @escaping () -> Void) {
        UploadManager.shared.backgroundCompletion = completionHandler
        UploadManager.shared.start()
    }
}
