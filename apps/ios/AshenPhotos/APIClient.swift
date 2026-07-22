import Foundation

enum APIError: LocalizedError {
    case status(Int, String)
    case decoding

    var errorDescription: String? {
        switch self {
        case let .status(code, body):
            // Server sends {"error":"..."}; surface that message when present.
            if let data = body.data(using: .utf8),
               let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
               let msg = obj["error"] as? String {
                return "HTTP \(code): \(msg)"
            }
            return "HTTP \(code)"
        case .decoding:
            return "Bad response from server"
        }
    }
}

/// Thin async wrapper over the Ashen API. `tokenProvider` supplies the current
/// bearer token (nil before login).
final class APIClient {
    private let base: URL
    private let tokenProvider: () -> String?
    private let session = URLSession(configuration: .default)

    init(base: URL = Config.apiBaseURL, tokenProvider: @escaping () -> String?) {
        self.base = base
        self.tokenProvider = tokenProvider
    }

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return d
    }()

    // MARK: Auth

    func register(email: String, password: String) async throws -> TokenResponse {
        try await post("/auth/register", body: Credentials(email: email, password: password), authed: false)
    }

    func login(email: String, password: String) async throws -> TokenResponse {
        try await post("/auth/login", body: Credentials(email: email, password: password), authed: false)
    }

    // MARK: Devices

    func registerDevice(name: String, platform: String = "ios") async throws -> DeviceResponse {
        struct Body: Encodable { let name: String; let platform: String }
        return try await post("/devices", body: Body(name: name, platform: platform))
    }

    // MARK: Uploads

    func check(_ items: [CheckItem]) async throws -> [CheckResult] {
        let resp: CheckResponse = try await post("/uploads/check", body: CheckRequest(items: items))
        return resp.results
    }

    func createUpload(_ req: CreateUploadRequest) async throws -> CreateUploadResponse {
        try await post("/uploads", body: req)
    }

    func completeUpload(id: String) async throws {
        _ = try await request(path: "/uploads/\(id)/complete", method: "POST", body: Optional<Data>.none, authed: true)
    }

    // MARK: Core

    private func post<B: Encodable, R: Decodable>(_ path: String, body: B, authed: Bool = true) async throws -> R {
        let data = try await request(path: path, method: "POST", body: try Self.encoder.encode(body), authed: authed)
        do {
            return try Self.decoder.decode(R.self, from: data)
        } catch {
            throw APIError.decoding
        }
    }

    @discardableResult
    private func request(path: String, method: String, body: Data?, authed: Bool) async throws -> Data {
        var req = URLRequest(url: base.appendingPathComponent(path))
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if authed, let token = tokenProvider() {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        req.httpBody = body

        let (data, resp) = try await session.data(for: req)
        guard let http = resp as? HTTPURLResponse else { throw APIError.status(-1, "no response") }
        guard (200..<300).contains(http.statusCode) else {
            throw APIError.status(http.statusCode, String(data: data, encoding: .utf8) ?? "")
        }
        return data
    }
}
