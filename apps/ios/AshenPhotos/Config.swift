import Foundation

enum Config {
    /// API base URL. Simulator can reach the host LAN directly.
    /// Override at runtime via the ASHEN_API_URL environment variable.
    static var apiBaseURL: URL {
        if let s = ProcessInfo.processInfo.environment["ASHEN_API_URL"], let u = URL(string: s) {
            return u
        }
        return URL(string: "http://nuc.test:8080")!
    }

    /// Batch size for /uploads/check dedup requests.
    static let checkBatchSize = 200
}
