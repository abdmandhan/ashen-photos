import Foundation
import Photos
import PhotosUI
import CryptoKit
import UIKit

struct ExportedAsset {
    let localIdentifier: String
    let fileURL: URL
    let sha256: String
    let byteSize: Int64
    let ext: String?
    let mediaType: String            // "photo" | "video"
    let capturedAt: Date?
    let livePhotoGroupID: String?    // shared across the still + paired video
    var thumbnailJPEG: Data?         // client-rendered thumbnail (HEIC/video decode natively)
}

enum ScanError: Error {
    case noResource
    case exportFailed
    case timedOut
}

/// Ensures a continuation resumes exactly once, from either the callback or a timeout.
private final class ResumeGuard: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false
    func claim() -> Bool {
        lock.lock(); defer { lock.unlock() }
        if done { return false }
        done = true
        return true
    }
}

/// Reads the photo library and exports originals to temp files, hashing as it streams.
enum PhotoScanner {
    static func requestAuthorization() async -> PHAuthorizationStatus {
        await withCheckedContinuation { cont in
            PHPhotoLibrary.requestAuthorization(for: .readWrite) { cont.resume(returning: $0) }
        }
    }

    static var authorizationStatus: PHAuthorizationStatus {
        PHPhotoLibrary.authorizationStatus(for: .readWrite)
    }

    /// Whether the app has only limited photo access (user picked specific photos).
    static var isLimited: Bool { authorizationStatus == .limited }

    /// Presents the system picker to add more photos to the app's limited selection.
    @MainActor
    static func presentLimitedPicker() {
        guard let vc = topViewController() else { return }
        PHPhotoLibrary.shared().presentLimitedLibraryPicker(from: vc)
    }

    @MainActor
    private static func topViewController() -> UIViewController? {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let root = scenes.flatMap { $0.windows }.first { $0.isKeyWindow }?.rootViewController
        var top = root
        while let presented = top?.presentedViewController { top = presented }
        return top
    }

    /// Returns the subset of local identifiers still present in the photo library.
    static func presentIdentifiers(_ ids: [String]) -> Set<String> {
        guard !ids.isEmpty else { return [] }
        let result = PHAsset.fetchAssets(withLocalIdentifiers: ids, options: nil)
        var present = Set<String>()
        result.enumerateObjects { asset, _, _ in present.insert(asset.localIdentifier) }
        return present
    }

    /// All photos + videos. `oldestFirst` backs up older memories first.
    static func fetchAssets(oldestFirst: Bool = false) -> [PHAsset] {
        let opts = PHFetchOptions()
        opts.sortDescriptors = [NSSortDescriptor(key: "creationDate", ascending: oldestFirst)]
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

        // One thumbnail per asset — PhotoKit renders HEIC/video natively.
        let thumb = await thumbnailJPEG(for: asset)

        var parts: [ExportedAsset] = []
        if asset.mediaType == .image, asset.mediaSubtypes.contains(.photoLive) {
            let groupID = UUID().uuidString
            if let still = resources.first(where: { $0.type == .photo })
                ?? resources.first(where: { $0.type == .fullSizePhoto }) {
                parts.append(try await exportResource(still, asset: asset, mediaType: "photo", groupID: groupID))
            }
            if let paired = resources.first(where: { $0.type == .pairedVideo })
                ?? resources.first(where: { $0.type == .fullSizePairedVideo }) {
                parts.append(try await exportResource(paired, asset: asset, mediaType: "video", groupID: groupID))
            }
            if parts.isEmpty { throw ScanError.noResource }
        } else {
            guard let resource = primaryResource(resources, mediaType: asset.mediaType) else {
                throw ScanError.noResource
            }
            let mediaType = asset.mediaType == .video ? "video" : "photo"
            parts.append(try await exportResource(resource, asset: asset, mediaType: mediaType, groupID: nil))
        }

        // Attach the thumbnail to the first part.
        if !parts.isEmpty { parts[0].thumbnailJPEG = thumb }
        return parts
    }

    // iCloud-backed assets can stall indefinitely; time out so the batch keeps moving.
    private static let dataTimeout: TimeInterval = 120
    private static let thumbTimeout: TimeInterval = 30

    /// Hashes an asset's primary resource (no temp file) — for backfill matching.
    static func hashPrimary(_ asset: PHAsset) async -> String? {
        let resources = PHAssetResource.assetResources(for: asset)
        guard let resource = primaryResource(resources, mediaType: asset.mediaType) else { return nil }
        var hasher = SHA256()
        let opts = PHAssetResourceRequestOptions()
        opts.isNetworkAccessAllowed = true
        let ok: Bool = await withCheckedContinuation { cont in
            let guardOnce = ResumeGuard()
            let reqID = PHAssetResourceManager.default().requestData(for: resource, options: opts) { chunk in
                hasher.update(data: chunk)
            } completionHandler: { err in
                if guardOnce.claim() { cont.resume(returning: err == nil) }
            }
            DispatchQueue.global().asyncAfter(deadline: .now() + dataTimeout) {
                PHAssetResourceManager.default().cancelDataRequest(reqID)
                if guardOnce.claim() { cont.resume(returning: false) }
            }
        }
        guard ok else { return nil }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }

    /// Renders a ~1024px JPEG thumbnail via PhotoKit (handles HEIC + video posters).
    static func thumbnailJPEG(for asset: PHAsset) async -> Data? {
        await withCheckedContinuation { cont in
            let opts = PHImageRequestOptions()
            opts.isNetworkAccessAllowed = true
            opts.deliveryMode = .highQualityFormat
            opts.resizeMode = .fast
            let guardOnce = ResumeGuard()
            let reqID = PHImageManager.default().requestImage(
                for: asset, targetSize: CGSize(width: 1024, height: 1024),
                contentMode: .aspectFit, options: opts
            ) { img, info in
                // Ignore the degraded preview; wait for (or time out on) the full image.
                let degraded = (info?[PHImageResultIsDegradedKey] as? Bool) ?? false
                if degraded { return }
                if guardOnce.claim() { cont.resume(returning: img?.jpegData(compressionQuality: 0.7)) }
            }
            DispatchQueue.global().asyncAfter(deadline: .now() + thumbTimeout) {
                PHImageManager.default().cancelImageRequest(reqID)
                if guardOnce.claim() { cont.resume(returning: nil) }
            }
        }
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
            let guardOnce = ResumeGuard()
            let reqID = PHAssetResourceManager.default().requestData(for: resource, options: opts) { chunk in
                hasher.update(data: chunk)
                total += Int64(chunk.count)
                try? handle.write(contentsOf: chunk)
            } completionHandler: { error in
                guard guardOnce.claim() else { return }
                try? handle.close()
                if let error { cont.resume(throwing: error) } else { cont.resume() }
            }
            DispatchQueue.global().asyncAfter(deadline: .now() + dataTimeout) {
                PHAssetResourceManager.default().cancelDataRequest(reqID)
                if guardOnce.claim() {
                    try? handle.close()
                    cont.resume(throwing: ScanError.timedOut)
                }
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
