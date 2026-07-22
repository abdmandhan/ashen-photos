import Foundation
import Network
import UIKit

/// User backup preferences + current network/power gating.
@MainActor
final class SettingsStore: ObservableObject {
    @Published var wifiOnly: Bool {
        didSet { UserDefaults.standard.set(wifiOnly, forKey: "wifi_only") }
    }
    @Published var chargingOnly: Bool {
        didSet { UserDefaults.standard.set(chargingOnly, forKey: "charging_only") }
    }
    @Published private(set) var onWifi = false

    private let monitor = NWPathMonitor()

    init() {
        wifiOnly = UserDefaults.standard.object(forKey: "wifi_only") as? Bool ?? true
        chargingOnly = UserDefaults.standard.object(forKey: "charging_only") as? Bool ?? false
        UIDevice.current.isBatteryMonitoringEnabled = true
        monitor.pathUpdateHandler = { [weak self] path in
            let wifi = path.usesInterfaceType(.wifi)
            Task { @MainActor in self?.onWifi = wifi }
        }
        monitor.start(queue: DispatchQueue(label: "net.monitor"))
    }

    /// Whether uploads are permitted under current settings.
    var canUpload: Bool {
        if wifiOnly && !onWifi { return false }
        if chargingOnly {
            let s = UIDevice.current.batteryState
            if s != .charging && s != .full { return false }
        }
        return true
    }
}
