import Foundation

// MARK: - API DTOs

struct Credentials: Encodable {
    let email: String
    let password: String
}

struct TokenResponse: Decodable {
    let token: String
    let userID: String

    enum CodingKeys: String, CodingKey {
        case token
        case userID = "user_id"
    }
}

struct CheckItem: Encodable {
    let sha256: String
    let byteSize: Int64

    enum CodingKeys: String, CodingKey {
        case sha256
        case byteSize = "byte_size"
    }
}

struct CheckRequest: Encodable {
    let items: [CheckItem]
}

struct CheckResult: Decodable {
    let sha256: String
    let exists: Bool
    let assetID: String?

    enum CodingKeys: String, CodingKey {
        case sha256, exists
        case assetID = "asset_id"
    }
}

struct CheckResponse: Decodable {
    let results: [CheckResult]
}

struct CreateUploadRequest: Encodable {
    let sha256: String
    let mediaType: String
    let byteSize: Int64
    let capturedAt: Date?
    let deviceID: String?
    let ext: String?

    enum CodingKeys: String, CodingKey {
        case sha256
        case mediaType = "media_type"
        case byteSize = "byte_size"
        case capturedAt = "captured_at"
        case deviceID = "device_id"
        case ext
    }
}

struct CreateUploadResponse: Decodable {
    let uploadID: String
    let assetID: String
    let storageKey: String
    let putURL: String

    enum CodingKeys: String, CodingKey {
        case uploadID = "upload_id"
        case assetID = "asset_id"
        case storageKey = "storage_key"
        case putURL = "put_url"
    }
}

struct DeviceResponse: Decodable {
    let id: String
    let name: String
}

// MARK: - Local backup state

enum BackupState: String, Codable {
    case pending
    case uploading
    case done
    case failed
    case skipped // already on server (dedup)
}

struct BackupItem: Codable, Identifiable {
    let id: String          // PHAsset.localIdentifier
    var sha256: String?
    var mediaType: String   // "photo" | "video"
    var uploadID: String?
    var state: BackupState
}
