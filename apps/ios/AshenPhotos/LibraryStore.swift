import Foundation

enum LibraryFilter: String, CaseIterable, Identifiable {
    case all, photos, videos, favorites
    var id: String { rawValue }
    var title: String {
        switch self {
        case .all: return "All"
        case .photos: return "Photos"
        case .videos: return "Videos"
        case .favorites: return "Favorites"
        }
    }
    var query: String {
        switch self {
        case .all: return ""
        case .photos: return "?media_type=photo"
        case .videos: return "?media_type=video"
        case .favorites: return "?favorite=true"
        }
    }
}

@MainActor
final class LibraryStore: ObservableObject {
    @Published private(set) var assets: [RemoteAsset] = []
    @Published private(set) var albums: [RemoteAlbum] = []
    @Published var filter: LibraryFilter = .all
    @Published private(set) var loading = false
    @Published var error: String?
    @Published var backfillStatus: String?
    @Published private(set) var backfilling = false

    private let api: APIClient

    init(auth: AuthStore) {
        self.api = auth.client()
        // Debug: start on a given filter for screenshots.
        if let f = ProcessInfo.processInfo.environment["ASHEN_DEBUG_FILTER"],
           let lf = LibraryFilter(rawValue: f) {
            filter = lf
        }
    }

    func load() async {
        loading = true
        defer { loading = false }
        do {
            async let a = api.listAssets(query: filter.query)
            async let al = api.listAlbums()
            assets = try await a
            albums = try await al
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Sets the filter synchronously (so the segmented control updates immediately)
    /// then reloads. The async variant snapped the picker back before `filter` changed.
    func setFilter(_ f: LibraryFilter) {
        filter = f
        Task { await reloadAssets() }
    }

    private func reloadAssets() async {
        do { assets = try await api.listAssets(query: filter.query) }
        catch { self.error = error.localizedDescription }
    }

    func toggleFavorite(_ asset: RemoteAsset) async {
        let newValue = !asset.favorite
        do {
            try await api.setFavorite(assetID: asset.id, favorite: newValue)
            if filter == .favorites && !newValue {
                assets.removeAll { $0.id == asset.id }
            } else if let i = assets.firstIndex(where: { $0.id == asset.id }) {
                assets[i].favorite = newValue
            }
        } catch {
            self.error = error.localizedDescription
        }
    }

    func createAlbum(name: String) async {
        do { try await api.createAlbum(name: name); albums = try await api.listAlbums() }
        catch { self.error = error.localizedDescription }
    }

    func deleteAlbum(id: String) async {
        do { try await api.deleteAlbum(id: id); albums = try await api.listAlbums() }
        catch { self.error = error.localizedDescription }
    }

    func albumAssets(id: String) async -> [RemoteAsset] {
        do { return try await api.albumAssets(id: id) }
        catch { self.error = error.localizedDescription; return [] }
    }

    /// Adds an asset to an album; refreshes album covers/counts.
    func addToAlbum(albumID: String, assetID: String) async {
        do { try await api.addToAlbum(albumID: albumID, assetID: assetID); albums = try await api.listAlbums() }
        catch { self.error = error.localizedDescription }
    }

    func removeFromAlbum(albumID: String, assetID: String) async {
        do { try await api.removeFromAlbum(albumID: albumID, assetID: assetID); albums = try await api.listAlbums() }
        catch { self.error = error.localizedDescription }
    }

    /// Backfills thumbnails for already-backed-up assets that lack one (HEIC/video
    /// uploaded before client thumbnails). Re-hashes local originals to match the
    /// server's missing list, renders a thumbnail, and uploads it.
    func backfillThumbnails() async {
        guard !backfilling else { return }
        backfilling = true
        defer { backfilling = false }

        backfillStatus = "Checking…"
        let missing: [String]
        do { missing = try await api.missingThumbs() }
        catch { backfillStatus = "Failed: \(error.localizedDescription)"; return }

        var pending = Set(missing)
        if pending.isEmpty { backfillStatus = "Thumbnails up to date"; return }

        let total = pending.count
        var done = 0
        for asset in PhotoScanner.fetchAssets() {
            if pending.isEmpty { break }
            guard let sha = await PhotoScanner.hashPrimary(asset), pending.contains(sha) else { continue }
            pending.remove(sha)
            guard let thumb = await PhotoScanner.thumbnailJPEG(for: asset),
                  let putURL = try? await api.presignThumb(sha256: sha),
                  await api.putThumbnail(to: putURL, data: thumb) else { continue }
            try? await api.commitThumb(sha256: sha)
            done += 1
            backfillStatus = "Backfilled \(done)/\(total)…"
        }
        backfillStatus = "Backfilled \(done) thumbnail\(done == 1 ? "" : "s")"
        await load()
    }
}
