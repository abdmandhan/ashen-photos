import Foundation
import Photos
import CryptoKit

struct ExportedAsset {
    let localIdentifier: String
    let fileURL: URL
    let sha256: String
    let byteSize: Int64
    let ext: String?
    let mediaType: String            // "photo" | "video"
    let capturedAt: Date?
    let livePhotoGroupID: String?    // shared across the still + paired video
}

enum ScanError: Error {
    case noResource
    case exportFailed
}

/// Reads the photo library and exports originals to temp files, hashing as it streams.
enum PhotoScanner {
    static func requestAuthorization() async -> PHAuthorizationStatus {
        await withCheckedContinuation { cont in
            PHPhotoLibrary.requestAuthorization(for: .readWrite) { cont.resume(returning: $0) }
        }
    }

    /// All photos + videos, newest first.
    static func fetchAssets() -> [PHAsset] {
        let opts = PHFetchOptions()
        opts.sortDescriptors = [NSSortDescriptor(key: "creationDate", ascending: false)]
        opts.predicate = NSPredicate(format: "mediaType == %d OR mediaType == %d",
                                     PHAssetMediaType.image.rawValue,
                                     PHAssetMediaType.video.rawValue)
        let result = PHAsset.fetchAssets(with: opts)
        var assets: [PHAsset] = []
        result.enumerateObjects { asset, _, _ in assets.append(asset) }
        return assets
    }

    /// Exports an asset to one or more temp files. A Live Photo yields two parts
    /// (still + paired video) sharing a `livePhotoGroupID`; everything else yields one.
    static func export(_ asset: PHAsset) async throws -> [ExportedAsset] {
        let resources = PHAssetResource.assetResources(for: asset)

        if asset.mediaType == .image, asset.mediaSubtypes.contains(.photoLive) {
            let groupID = UUID().uuidString
            var parts: [ExportedAsset] = []
            if let still = resources.first(where: { $0.type == .photo })
                ?? resources.first(where: { $0.type == .fullSizePhoto }) {
                parts.append(try await exportResource(still, asset: asset, mediaType: "photo", groupID: groupID))
            }
            if let paired = resources.first(where: { $0.type == .pairedVideo })
                ?? resources.first(where: { $0.type == .fullSizePairedVideo }) {
                parts.append(try await exportResource(paired, asset: asset, mediaType: "video", groupID: groupID))
            }
            if parts.isEmpty { throw ScanError.noResource }
            return parts
        }

        guard let resource = primaryResource(resources, mediaType: asset.mediaType) else {
            throw ScanError.noResource
        }
        let mediaType = asset.mediaType == .video ? "video" : "photo"
        return [try await exportResource(resource, asset: asset, mediaType: mediaType, groupID: nil)]
    }

    /// Streams one resource to a temp file, hashing as it goes.
    private static func exportResource(_ resource: PHAssetResource, asset: PHAsset,
                                       mediaType: String, groupID: String?) async throws -> ExportedAsset {
        let tmp = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString)
        FileManager.default.createFile(atPath: tmp.path, contents: nil)
        let handle = try FileHandle(forWritingTo: tmp)

        var hasher = SHA256()
        var total: Int64 = 0

        let opts = PHAssetResourceRequestOptions()
        opts.isNetworkAccessAllowed = true

        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            PHAssetResourceManager.default().requestData(for: resource, options: opts) { chunk in
                hasher.update(data: chunk)
                total += Int64(chunk.count)
                try? handle.write(contentsOf: chunk)
            } completionHandler: { error in
                try? handle.close()
                if let error { cont.resume(throwing: error) }
                else { cont.resume() }
            }
        }

        let digest = hasher.finalize().map { String(format: "%02x", $0) }.joined()
        let ext = (resource.originalFilename as NSString).pathExtension.lowercased()

        return ExportedAsset(
            localIdentifier: asset.localIdentifier,
            fileURL: tmp,
            sha256: digest,
            byteSize: total,
            ext: ext.isEmpty ? nil : ext,
            mediaType: mediaType,
            capturedAt: asset.creationDate,
            livePhotoGroupID: groupID
        )
    }

    private static func primaryResource(_ resources: [PHAssetResource], mediaType: PHAssetMediaType) -> PHAssetResource? {
        let wanted: PHAssetResourceType = mediaType == .video ? .video : .photo
        return resources.first { $0.type == wanted }
            ?? resources.first { $0.type == .fullSizePhoto }
            ?? resources.first
    }
}
