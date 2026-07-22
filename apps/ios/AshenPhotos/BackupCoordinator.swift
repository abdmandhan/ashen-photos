import Foundation
import Photos

@MainActor
final class BackupCoordinator: ObservableObject {
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

        statusLine = "Scanning library…"
        let assets = PhotoScanner.fetchAssets()
        mergeAssets(assets)

        for asset in assets {
            if !settings.canUpload {
                statusLine = "Paused (waiting for Wi-Fi/charging)"
                break
            }
            guard var item = items[asset.localIdentifier], item.state == .pending else { continue }

            do {
                statusLine = "Processing \(done + 1)/\(total)…"
                let exported = try await PhotoScanner.export(asset)
                item.sha256 = exported.sha256
                item.mediaType = exported.mediaType
                items[asset.localIdentifier] = item

                // Dedup check.
                let results = try await api.check([CheckItem(sha256: exported.sha256, byteSize: exported.byteSize)])
                if results.first?.exists == true {
                    item.state = .skipped
                    items[asset.localIdentifier] = item
                    try? FileManager.default.removeItem(at: exported.fileURL)
                    continue
                }

                // Create upload + hand off to background session.
                let req = CreateUploadRequest(
                    sha256: exported.sha256,
                    mediaType: exported.mediaType,
                    byteSize: exported.byteSize,
                    capturedAt: exported.capturedAt,
                    deviceID: auth.deviceID,
                    ext: exported.ext
                )
                let up = try await api.createUpload(req)
                item.uploadID = up.uploadID
                item.state = .uploading
                items[asset.localIdentifier] = item
                uploadToLocal[up.uploadID] = asset.localIdentifier

                UploadManager.shared.upload(fileURL: exported.fileURL, to: up.putURL, uploadID: up.uploadID)
            } catch {
                item.state = .failed
                items[asset.localIdentifier] = item
            }
            save()
        }

        statusLine = remaining > 0 ? "Uploading in background…" : "Backup complete"
    }

    // MARK: Upload completion

    private func handleFinish(uploadID: String, ok: Bool) {
        guard let local = uploadToLocal[uploadID], var item = items[local] else { return }
        item.state = ok ? .done : .failed
        items[local] = item
        uploadToLocal[uploadID] = nil
        save()
        if remaining == 0 { statusLine = "Backup complete" }
    }

    // MARK: Local queue persistence

    private func mergeAssets(_ assets: [PHAsset]) {
        for asset in assets where items[asset.localIdentifier] == nil {
            let type = asset.mediaType == .video ? "video" : "photo"
            items[asset.localIdentifier] = BackupItem(
                id: asset.localIdentifier, sha256: nil, mediaType: type,
                uploadID: nil, state: .pending
            )
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
            if it.state == .uploading { it.state = .pending } // resume interrupted
            items[it.id] = it
        }
    }
}
