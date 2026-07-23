import Foundation
import Photos

@MainActor
final class BackupCoordinator: NSObject, ObservableObject {
    @Published private(set) var items: [String: BackupItem] = [:]
    @Published private(set) var running = false
    @Published private(set) var paused = false
    @Published var statusLine = "Idle"
    @Published var authorized = false
    @Published var lastBackupAt: Date? {
        didSet { UserDefaults.standard.set(lastBackupAt, forKey: "last_backup_at") }
    }
    @Published var reconciling = false
    @Published var backendStats: RemoteStats?

    private let auth: AuthStore
    private let settings: SettingsStore
    private lazy var api = auth.client()

    // uploadID -> localIdentifier, so upload completion maps back to an item.
    private var uploadToLocal: [String: String] = [:]
    // Owns the backup run so it survives view/tab changes (not tied to a SwiftUI .task).
    private var backupTask: Task<Void, Never>?

    /// The active coordinator, so background tasks can reuse it instead of
    /// building a second one that double-registers UploadManager callbacks.
    static weak var shared: BackupCoordinator?

    init(auth: AuthStore, settings: SettingsStore) {
        self.auth = auth
        self.settings = settings
        super.init()
        Self.shared = self
        load()
        lastBackupAt = UserDefaults.standard.object(forKey: "last_backup_at") as? Date
        UploadManager.shared.onFinish = { [weak self] uploadID, ok, reason in
            Task { @MainActor in self?.handleFinish(uploadID: uploadID, ok: ok, reason: reason) }
        }
        UploadManager.shared.onProgress = { [weak self] uploadID, fraction in
            Task { @MainActor in self?.handleProgress(uploadID: uploadID, fraction: fraction) }
        }
        UploadManager.shared.start()
    }

    // MARK: Derived counts

    var total: Int { items.count }
    /// Completed = actually uploaded now + already-on-server (dedup skipped).
    var done: Int { items.values.filter { $0.state == .done || $0.state == .skipped }.count }
    /// Files this run actually sent to storage.
    var uploaded: Int { items.values.filter { $0.state == .done }.count }
    /// Already on the server, nothing sent (deduped by checksum).
    var skipped: Int { items.values.filter { $0.state == .skipped }.count }
    var failed: Int { items.values.filter { $0.state == .failed }.count }
    var remaining: Int { items.values.filter { $0.state == .pending || $0.state == .uploading }.count }
    var uploading: Int { items.values.filter { $0.state == .uploading }.count }

    /// Failed items with their reasons, for the UI list.
    var failedItems: [BackupItem] {
        items.values.filter { $0.state == .failed }.sorted { $0.id < $1.id }
    }

    /// Force-retry all failed items now (resets the retry cap), then rescan.
    func retryFailedNow() {
        for (id, var item) in items where item.state == .failed {
            item.state = .pending
            item.retryCount = 0
            item.outstanding = []
            item.errorMessage = nil
            items[id] = item
        }
        startBackup()
    }

    // MARK: Pause / resume

    func pause() {
        paused = true
        statusLine = "Paused"
    }

    func resume() {
        paused = false
        startBackup()
    }

    // MARK: Run

    /// Starts a backup in a coordinator-owned task. Safe to call repeatedly (e.g.
    /// from a tab's onAppear or the photo-library observer) — it no-ops if already
    /// running, and is NOT cancelled when the view disappears.
    func startBackup() {
        guard backupTask == nil, !running, !paused else { return }
        backupTask = Task { @MainActor [weak self] in
            await self?.run()
            self?.backupTask = nil
        }
    }

    /// Entry point for BGTaskScheduler. Reuses the live coordinator if present,
    /// else builds one from the stored session. No-op if logged out.
    static func runBackgroundBackup() async {
        guard Keychain.get("token") != nil else { return }
        let coordinator = shared ?? BackupCoordinator(auth: AuthStore(), settings: SettingsStore())
        await coordinator.run()
    }

    func run() async {
        guard !running else { return }
        guard !paused else { statusLine = "Paused"; return }
        running = true
        defer { running = false; save() }

        statusLine = "Requesting Photos access…"
        let status = await PhotoScanner.requestAuthorization()
        authorized = (status == .authorized || status == .limited)
        guard authorized else { statusLine = "Photos access denied"; return }

        // Watch for new photos so backups pick them up automatically.
        PHPhotoLibrary.shared().register(self)

        statusLine = "Scanning library…"
        let assets = PhotoScanner.fetchAssets(oldestFirst: settings.oldestFirst)
        mergeAssets(assets)
        retryFailed()

        // Process assets in concurrent batches of `backupConcurrency`. Heavy work
        // (export, hash, network) suspends at await points, so a batch interleaves.
        let pending = assets.filter { items[$0.localIdentifier]?.state == .pending }
        let limit = max(1, settings.backupConcurrency)

        for batch in pending.chunked(into: limit) {
            if paused || !settings.canUpload { break }
            await withTaskGroup(of: Void.self) { group in
                for asset in batch where items[asset.localIdentifier]?.state == .pending {
                    group.addTask { @MainActor in await self.process(asset) }
                }
            }
        }

        if paused { statusLine = "Paused"; return }
        if !settings.canUpload { statusLine = "Paused (waiting for Wi-Fi/charging)"; return }
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
            var totalBytes: Int64 = 0
            var shas: [String] = []
            var allExisted = true
            for part in parts {
                totalBytes += part.byteSize
                shas.append(part.sha256)
                let results = try await api.check([CheckItem(sha256: part.sha256, byteSize: part.byteSize)])
                if results.first?.exists == true {
                    // Already on the server (verified/complete) — no upload needed.
                    try? FileManager.default.removeItem(at: part.fileURL)
                    continue
                }
                allExisted = false
                let req = CreateUploadRequest(
                    sha256: part.sha256, mediaType: part.mediaType, byteSize: part.byteSize,
                    capturedAt: part.capturedAt, deviceID: auth.deviceID,
                    ext: part.ext, livePhotoGroupID: part.livePhotoGroupID
                )
                let up = try await api.createUpload(req)

                // Upload the client-rendered thumbnail (HEIC/video) if present.
                var hasThumb = false
                if let thumbData = part.thumbnailJPEG, let thumbURL = up.thumbPutURL, !thumbURL.isEmpty {
                    hasThumb = await api.putThumbnail(to: thumbURL, data: thumbData)
                }

                outstanding.append(up.uploadID)
                uploadToLocal[up.uploadID] = id
                UploadManager.shared.upload(fileURL: part.fileURL, to: up.putURL, uploadID: up.uploadID, hasThumb: hasThumb)
            }

            var updated = items[id]!
            updated.outstanding = outstanding
            updated.state = outstanding.isEmpty ? .skipped : .uploading
            updated.errorMessage = nil
            updated.byteSize = totalBytes
            updated.shas = shas
            // Every part already existed on the server → verified now (dedup = complete).
            if allExisted { updated.verified = true; lastBackupAt = Date() }
            items[id] = updated
        } catch {
            if isCancellation(error) {
                // Interrupted (tab switch, backgrounding) — leave pending to resume,
                // don't treat as a real failure.
                items[id]?.state = .pending
                items[id]?.outstanding = []
            } else {
                markFailed(id, reason: error.localizedDescription)
            }
        }
        save()
    }

    private func isCancellation(_ error: Error) -> Bool {
        if error is CancellationError { return true }
        if let urlErr = error as? URLError, urlErr.code == .cancelled { return true }
        return false
    }

    // MARK: Upload completion

    private func handleProgress(uploadID: String, fraction: Double) {
        guard let local = uploadToLocal[uploadID], var item = items[local] else { return }
        // Only move forward; avoids flicker when parts report out of order.
        if fraction > item.progress {
            item.progress = fraction
            items[local] = item
        }
    }

    /// Items currently exporting/uploading, for the live progress list.
    var inProgressItems: [BackupItem] {
        items.values.filter { $0.state == .uploading }.sorted { $0.id < $1.id }
    }

    private func handleFinish(uploadID: String, ok: Bool, reason: String?) {
        guard let local = uploadToLocal[uploadID], var item = items[local] else { return }
        uploadToLocal[uploadID] = nil
        item.outstanding.removeAll { $0 == uploadID }

        if !ok {
            let r = reason ?? "Upload failed"
            if r.localizedCaseInsensitiveContains("cancel") {
                // Transient interruption — retry on the next run, don't mark failed.
                item.state = .pending
                item.outstanding = []
                items[local] = item
            } else {
                markFailed(local, reason: r)
            }
        } else if item.outstanding.isEmpty {
            item.state = .done
            item.errorMessage = nil
            item.progress = 1
            items[local] = item
            lastBackupAt = Date()
        } else {
            items[local] = item // still waiting on other parts
        }
        save()
        if remaining == 0 { statusLine = "Backup complete" }
    }

    private func markFailed(_ id: String, reason: String) {
        guard var item = items[id] else { return }
        item.state = .failed
        item.outstanding = []
        item.errorMessage = reason
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

    /// Loads backend storage usage (photos/videos/bytes stored on the server).
    func loadBackendStats() async {
        backendStats = try? await api.stats()
    }

    // MARK: - Free Up Space (Phase 2a)

    /// Verified + still-on-device items, eligible for deletion.
    var safeToDeleteItems: [BackupItem] {
        items.values.filter { $0.safeToDelete && $0.byteSize > 0 }
    }
    var recoverableBytes: Int64 { safeToDeleteItems.reduce(0) { $0 + $1.byteSize } }
    var safePhotoCount: Int { safeToDeleteItems.filter { $0.mediaType != "video" }.count }
    var safeVideoCount: Int { safeToDeleteItems.filter { $0.mediaType == "video" }.count }

    /// Confirms with the server which uploaded items are fully verified (`complete`),
    /// and drops items no longer present in the photo library. Cheap: reads cached
    /// state, one batched `/uploads/check`.
    func reconcileFreeSpace() async {
        guard !reconciling else { return }
        reconciling = true
        defer { reconciling = false; save() }

        // 1. Mark items whose local asset is gone as deleted-from-device.
        let ids = Array(items.keys)
        let present = PhotoScanner.presentIdentifiers(ids)
        for id in ids where !present.contains(id) {
            items[id]?.deletedFromDevice = true
        }

        // 2. Reconcile verification for uploaded-but-unverified items.
        let pending = items.values.filter { !$0.verified && !$0.deletedFromDevice && !$0.shas.isEmpty }
        guard !pending.isEmpty else { return }

        // Batched existence check; a sha "exists" only if a complete asset backs it.
        var verifiedShas = Set<String>()
        let allShas = Array(Set(pending.flatMap { $0.shas }))
        for chunk in allShas.chunked(into: Config.checkBatchSize) {
            let items = chunk.map { CheckItem(sha256: $0, byteSize: 0) }
            if let results = try? await api.check(items) {
                for r in results where r.exists { verifiedShas.insert(r.sha256) }
            }
        }
        for item in pending where item.shas.allSatisfy({ verifiedShas.contains($0) }) {
            items[item.id]?.verified = true
        }
    }

    /// Deletes the given assets from the device via PhotoKit. iOS shows its own
    /// system confirmation dialog. Returns true if the user confirmed + it succeeded.
    func deleteFromDevice(ids: [String]) async -> Bool {
        let fetch = PHAsset.fetchAssets(withLocalIdentifiers: ids, options: nil)
        var phAssets: [PHAsset] = []
        fetch.enumerateObjects { a, _, _ in phAssets.append(a) }
        guard !phAssets.isEmpty else { return false }

        let success: Bool = await withCheckedContinuation { cont in
            PHPhotoLibrary.shared().performChanges {
                PHAssetChangeRequest.deleteAssets(phAssets as NSArray)
            } completionHandler: { ok, _ in
                cont.resume(returning: ok)
            }
        }
        if success {
            for id in ids { items[id]?.deletedFromDevice = true }
            save()
        }
        return success
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
        Task { @MainActor in self.startBackup() }
    }
}
