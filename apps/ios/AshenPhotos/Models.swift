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
    let deviceID: String?

    enum CodingKeys: String, CodingKey {
        case items
        case deviceID = "device_id"
    }
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
    let livePhotoGroupID: String?

    enum CodingKeys: String, CodingKey {
        case sha256
        case mediaType = "media_type"
        case byteSize = "byte_size"
        case capturedAt = "captured_at"
        case deviceID = "device_id"
        case ext
        case livePhotoGroupID = "live_photo_group_id"
    }
}

struct CreateUploadResponse: Decodable {
    let uploadID: String
    let assetID: String
    let storageKey: String
    let putURL: String
    let thumbKey: String?
    let thumbPutURL: String?

    enum CodingKeys: String, CodingKey {
        case uploadID = "upload_id"
        case assetID = "asset_id"
        case storageKey = "storage_key"
        case putURL = "put_url"
        case thumbKey = "thumb_key"
        case thumbPutURL = "thumb_put_url"
    }
}

struct DeviceResponse: Decodable {
    let id: String
    let name: String
}

// MARK: - Library (browse) DTOs

struct RemoteAsset: Decodable, Identifiable {
    let id: String
    let mediaType: String
    let capturedAt: Date?
    var favorite: Bool
    let thumbURL: String?
    let downloadURL: String?

    enum CodingKeys: String, CodingKey {
        case id
        case mediaType = "media_type"
        case capturedAt = "captured_at"
        case favorite
        case thumbURL = "thumb_url"
        case downloadURL = "download_url"
    }
}

struct AssetsResponse: Decodable {
    let assets: [RemoteAsset]
}

struct RemoteAlbum: Decodable, Identifiable {
    let id: String
    let name: String
    let assetCount: Int
    let coverURL: String?

    enum CodingKeys: String, CodingKey {
        case id, name
        case assetCount = "asset_count"
        case coverURL = "cover_url"
    }
}

struct AlbumsResponse: Decodable {
    let albums: [RemoteAlbum]
}

struct RemoteStats: Decodable {
    let photoCount: Int
    let videoCount: Int
    let totalBytes: Int64

    enum CodingKeys: String, CodingKey {
        case photoCount = "photo_count"
        case videoCount = "video_count"
        case totalBytes = "total_bytes"
    }
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
    let id: String              // PHAsset.localIdentifier
    var mediaType: String       // "photo" | "video" | "live"
    var state: BackupState
    var retryCount: Int = 0
    // Upload ids still in flight (>1 for Live Photos: still + paired video).
    var outstanding: [String] = []
    // Reason for the last failure, shown in the UI.
    var errorMessage: String? = nil
    // Upload progress 0...1 (aggregate across parts), for the UI. Not persisted.
    var progress: Double = 0

    // --- Free Up Space (Phase 2a) ---
    var byteSize: Int64 = 0         // total on-device original size (sum of parts)
    var shas: [String] = []          // sha256 of each uploaded part (for verify reconciliation)
    var verified: Bool = false       // server confirmed status=complete for all parts
    var deletedFromDevice: Bool = false

    /// SAFE_TO_DELETE: fully verified on the server and still on this device.
    var safeToDelete: Bool { verified && !deletedFromDevice }
}

let maxRetries = 3
