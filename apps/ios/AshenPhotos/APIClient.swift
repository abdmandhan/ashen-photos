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
    private let deviceIDProvider: () -> String?
    private let session = URLSession(configuration: .default)

    init(base: URL = Config.apiBaseURL,
         tokenProvider: @escaping () -> String?,
         deviceIDProvider: @escaping () -> String? = { nil }) {
        self.base = base
        self.tokenProvider = tokenProvider
        self.deviceIDProvider = deviceIDProvider
    }

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    private static let isoFractional: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return f
    }()
    private static let isoPlain = ISO8601DateFormatter()

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        // Server timestamps may or may not include fractional seconds — accept both.
        d.dateDecodingStrategy = .custom { decoder in
            let s = try decoder.singleValueContainer().decode(String.self)
            if let date = isoFractional.date(from: s) ?? isoPlain.date(from: s) { return date }
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath,
                                                    debugDescription: "Bad date: \(s)"))
        }
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
        let body = CheckRequest(items: items, deviceID: deviceIDProvider())
        let resp: CheckResponse = try await post("/uploads/check", body: body)
        return resp.results
    }

    func createUpload(_ req: CreateUploadRequest) async throws -> CreateUploadResponse {
        try await post("/uploads", body: req)
    }

    func completeUpload(id: String, thumb: Bool = false) async throws {
        struct Body: Encodable { let thumb: Bool }
        _ = try await request(path: "/uploads/\(id)/complete", method: "POST",
                              body: try Self.encoder.encode(Body(thumb: thumb)), authed: true)
    }

    // MARK: Thumbnail backfill

    func missingThumbs() async throws -> [String] {
        struct R: Decodable { let shas: [String] }
        let data = try await request(path: "/thumbnails/missing", method: "GET", body: Optional<Data>.none, authed: true)
        return (try Self.decoder.decode(R.self, from: data)).shas
    }

    func presignThumb(sha256: String) async throws -> String {
        struct Body: Encodable { let sha256: String }
        struct R: Decodable {
            let thumbPutURL: String
            enum CodingKeys: String, CodingKey { case thumbPutURL = "thumb_put_url" }
        }
        let data = try await request(path: "/thumbnails/presign", method: "POST",
                                     body: try Self.encoder.encode(Body(sha256: sha256)), authed: true)
        return (try Self.decoder.decode(R.self, from: data)).thumbPutURL
    }

    func commitThumb(sha256: String) async throws {
        struct Body: Encodable { let sha256: String }
        _ = try await request(path: "/thumbnails/commit", method: "POST",
                              body: try Self.encoder.encode(Body(sha256: sha256)), authed: true)
    }

    /// PUTs a JPEG thumbnail to a presigned storage URL. Returns success.
    func putThumbnail(to urlString: String, data: Data) async -> Bool {
        guard let url = URL(string: urlString) else { return false }
        var req = URLRequest(url: url)
        req.httpMethod = "PUT"
        req.setValue("image/jpeg", forHTTPHeaderField: "Content-Type")
        do {
            let (_, resp) = try await session.upload(for: req, from: data)
            return ((resp as? HTTPURLResponse).map { (200..<300).contains($0.statusCode) }) ?? false
        } catch {
            return false
        }
    }

    // MARK: Library

    func listAssets(query: String = "") async throws -> [RemoteAsset] {
        let data = try await request(path: "/assets\(query)", method: "GET", body: Optional<Data>.none, authed: true)
        return (try Self.decoder.decode(AssetsResponse.self, from: data)).assets
    }

    func stats() async throws -> RemoteStats {
        let data = try await request(path: "/stats", method: "GET", body: Optional<Data>.none, authed: true)
        return try Self.decoder.decode(RemoteStats.self, from: data)
    }

    func listAlbums() async throws -> [RemoteAlbum] {
        let data = try await request(path: "/albums", method: "GET", body: Optional<Data>.none, authed: true)
        return (try Self.decoder.decode(AlbumsResponse.self, from: data)).albums
    }

    func setFavorite(assetID: String, favorite: Bool) async throws {
        struct Body: Encodable { let favorite: Bool }
        _ = try await request(path: "/assets/\(assetID)/favorite", method: "PUT",
                              body: try Self.encoder.encode(Body(favorite: favorite)), authed: true)
    }

    func createAlbum(name: String) async throws {
        struct Body: Encodable { let name: String }
        _ = try await request(path: "/albums", method: "POST",
                              body: try Self.encoder.encode(Body(name: name)), authed: true)
    }

    func deleteAlbum(id: String) async throws {
        _ = try await request(path: "/albums/\(id)", method: "DELETE", body: Optional<Data>.none, authed: true)
    }

    func albumAssets(id: String) async throws -> [RemoteAsset] {
        let data = try await request(path: "/albums/\(id)/assets", method: "GET", body: Optional<Data>.none, authed: true)
        return (try Self.decoder.decode(AssetsResponse.self, from: data)).assets
    }

    func addToAlbum(albumID: String, assetID: String) async throws {
        struct Body: Encodable { let assetID: String
            enum CodingKeys: String, CodingKey { case assetID = "asset_id" } }
        _ = try await request(path: "/albums/\(albumID)/assets", method: "POST",
                              body: try Self.encoder.encode(Body(assetID: assetID)), authed: true)
    }

    func removeFromAlbum(albumID: String, assetID: String) async throws {
        _ = try await request(path: "/albums/\(albumID)/assets/\(assetID)", method: "DELETE",
                              body: Optional<Data>.none, authed: true)
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

    /// Builds a full URL. Uses string concatenation (not appendingPathComponent,
    /// which percent-encodes "?" and breaks query strings). `path` starts with "/".
    private func url(for path: String) -> URL {
        var s = base.absoluteString
        if s.hasSuffix("/") { s.removeLast() }
        return URL(string: s + path) ?? base
    }

    @discardableResult
    private func request(path: String, method: String, body: Data?, authed: Bool) async throws -> Data {
        var req = URLRequest(url: url(for: path))
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if authed, let token = tokenProvider() {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        // Device liveness: server bumps last_seen_at from this header.
        if authed, let deviceID = deviceIDProvider() {
            req.setValue(deviceID, forHTTPHeaderField: "X-Device-Id")
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
