import Foundation
import UIKit

@MainActor
final class AuthStore: ObservableObject {
    @Published private(set) var token: String?
    @Published private(set) var userID: String?
    @Published var errorMessage: String?
    @Published var busy = false

    private(set) var deviceID: String?

    private lazy var api = APIClient(tokenProvider: { [weak self] in self?.token })

    init() {
        token = Keychain.get("token")
        userID = UserDefaults.standard.string(forKey: "user_id")
        deviceID = UserDefaults.standard.string(forKey: "device_id")
    }

    var isAuthenticated: Bool { token != nil }

    func client() -> APIClient { api }

    func register(email: String, password: String) async {
        await run { try await self.api.register(email: email, password: password) }
    }

    func login(email: String, password: String) async {
        await run { try await self.api.login(email: email, password: password) }
    }

    private func run(_ op: @escaping () async throws -> TokenResponse) async {
        busy = true; errorMessage = nil
        defer { busy = false }
        do {
            let resp = try await op()
            token = resp.token
            userID = resp.userID
            Keychain.set(resp.token, for: "token")
            UserDefaults.standard.set(resp.userID, forKey: "user_id")
            await ensureDevice()
        } catch let APIError.status(code, msg) {
            errorMessage = "HTTP \(code): \(msg)"
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// Registers this device once, caching the id locally.
    private func ensureDevice() async {
        if deviceID != nil { return }
        let name = UIDevice.current.name
        do {
            let dev = try await api.registerDevice(name: name)
            deviceID = dev.id
            UserDefaults.standard.set(dev.id, forKey: "device_id")
        } catch {
            // Non-fatal: uploads still work without a device id.
        }
    }

    func logout() {
        token = nil; userID = nil; deviceID = nil
        Keychain.delete("token")
        UserDefaults.standard.removeObject(forKey: "user_id")
        UserDefaults.standard.removeObject(forKey: "device_id")
    }
}
