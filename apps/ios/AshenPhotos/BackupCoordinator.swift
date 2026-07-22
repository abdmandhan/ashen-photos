import Foundation
import Photos

@MainActor
final class BackupCoordinator: NSObject, ObservableObject {
    @Published private(set) var items: [String: BackupItem] = [:]
    @Published private(set) var running = false
    @Published var statusLine = "Idle"
    @Published var authorized = false

    private let auth: AuthStore
    private let settings: SettingsStore
    private lazy var api = auth.client()

    // uploadID -> localIdentifier, so upload completion maps back to an item.
    private var uploadToLocal: [String: String] = [:]

    init(auth: AuthStore, settings: SettingsStore) {
        self.auth = auth
        self.settings = settings
        super.init()
        load()
        UploadManager.shared.onFinish = { [weak self] uploadID, ok in
            Task { @MainActor in self?.handleFinish(uploadID: uploadID, ok: ok) }
        }
        UploadManager.shared.start()
    }

    // MARK: Derived counts

    var total: Int { items.count }
    var done: Int { items.values.filter { $0.state == .done || $0.state == .skipped }.count }
    var failed: Int { items.values.filter { $0.state == .failed }.count }
    var remaining: Int { items.values.filter { $0.state == .pending || $0.state == .uploading }.count }

    // MARK: Run

    func run() async {
        guard !running else { return }
        running = true
        defer { running = false; save() }

        statusLine = "Requesting Photos access…"
        let status = await PhotoScanner.requestAuthorization()
        authorized = (status == .authorized || status == .limited)
        guard authorized else { statusLine = "Photos access denied"; return }

        // Watch for new photos so backups pick them up automatically.
        PHPhotoLibrary.shared().register(self)

        statusLine = "Scanning library…"
        let assets = PhotoScanner.fetchAssets()
        mergeAssets(assets)
        retryFailed()

        for asset in assets {
            if !settings.canUpload {
                statusLine = "Paused (waiting for Wi-Fi/charging)"
                break
            }
            guard let item = items[asset.localIdentifier], item.state == .pending else { continue }
            await process(asset)
        }

        statusLine = remaining > 0 ? "Uploading in background…" : "Backup complete"
    }

    /// Exports one asset (1 part, or 2 for a Live Photo), dedups, and hands each
    /// part to the background uploader. An asset is done only when all parts finish.
    private func process(_ asset: PHAsset) async {
        let id = asset.localIdentifier
        do {
            statusLine = "Processing \(done + 1)/\(total)…"
            let parts = try await PhotoScanner.export(asset)

            var outstanding: [String] = []
            for part in parts {
                let results = try await api.check([CheckItem(sha256: part.sha256, byteSize: part.byteSize)])
                if results.first?.exists == true {
                    try? FileManager.default.removeItem(at: part.fileURL)
                    continue
                }
                let req = CreateUploadRequest(
                    sha256: part.sha256, mediaType: part.mediaType, byteSize: part.byteSize,
                    capturedAt: part.capturedAt, deviceID: auth.deviceID,
                    ext: part.ext, livePhotoGroupID: part.livePhotoGroupID
                )
                let up = try await api.createUpload(req)
                outstanding.append(up.uploadID)
                uploadToLocal[up.uploadID] = id
                UploadManager.shared.upload(fileURL: part.fileURL, to: up.putURL, uploadID: up.uploadID)
            }

            var updated = items[id]!
            updated.outstanding = outstanding
            updated.state = outstanding.isEmpty ? .skipped : .uploading
            items[id] = updated
        } catch {
            markFailed(id)
        }
        save()
    }

    // MARK: Upload completion

    private func handleFinish(uploadID: String, ok: Bool) {
        guard let local = uploadToLocal[uploadID], var item = items[local] else { return }
        uploadToLocal[uploadID] = nil
        item.outstanding.removeAll { $0 == uploadID }

        if !ok {
            markFailed(local)
        } else if item.outstanding.isEmpty {
            item.state = .done
            items[local] = item
        } else {
            items[local] = item // still waiting on other parts
        }
        save()
        if remaining == 0 { statusLine = "Backup complete" }
    }

    private func markFailed(_ id: String) {
        guard var item = items[id] else { return }
        item.state = .failed
        item.outstanding = []
        items[id] = item
    }

    /// Resets failed items (under the retry cap) back to pending so the next
    /// scan re-uploads them. Handles transient network loss.
    private func retryFailed() {
        for (id, var item) in items where item.state == .failed && item.retryCount < maxRetries {
            item.retryCount += 1
            item.state = .pending
            item.outstanding = []
            items[id] = item
        }
    }

    // MARK: Local queue

    private func mergeAssets(_ assets: [PHAsset]) {
        for asset in assets where items[asset.localIdentifier] == nil {
            let type: String
            if asset.mediaType == .image, asset.mediaSubtypes.contains(.photoLive) {
                type = "live"
            } else {
                type = asset.mediaType == .video ? "video" : "photo"
            }
            items[asset.localIdentifier] = BackupItem(id: asset.localIdentifier, mediaType: type, state: .pending)
        }
    }

    private var storeURL: URL {
        let dir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        return dir.appendingPathComponent("backup_queue.json")
    }

    private func save() {
        if let data = try? JSONEncoder().encode(Array(items.values)) {
            try? data.write(to: storeURL)
        }
    }

    private func load() {
        guard let data = try? Data(contentsOf: storeURL),
              let arr = try? JSONDecoder().decode([BackupItem].self, from: data) else { return }
        for var it in arr {
            if it.state == .uploading { it.state = .pending; it.outstanding = [] } // resume interrupted
            items[it.id] = it
        }
    }
}

// MARK: - Auto-catch new photos

extension BackupCoordinator: PHPhotoLibraryChangeObserver {
    nonisolated func photoLibraryDidChange(_ changeInstance: PHChange) {
        Task { @MainActor in
            guard !self.running else { return }
            await self.run()
        }
    }
}
