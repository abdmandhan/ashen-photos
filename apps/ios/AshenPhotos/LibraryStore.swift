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
    var param: String? {
        switch self {
        case .all: return nil
        case .photos: return "media_type=photo"
        case .videos: return "media_type=video"
        case .favorites: return "favorite=true"
        }
    }
}

@MainActor
final class LibraryStore: ObservableObject {
    @Published private(set) var assets: [RemoteAsset] = []
    @Published private(set) var albums: [RemoteAlbum] = []
    @Published var filter: LibraryFilter = .all
    @Published var sortOldest = false
    @Published var fromDate: Date?
    @Published var toDate: Date?
    @Published private(set) var loading = false
    @Published private(set) var loadingMore = false
    @Published private(set) var hasMore = true
    @Published var error: String?
    @Published var backfillStatus: String?
    @Published private(set) var backfilling = false

    private let api: APIClient
    private let pageSize = 100
    private var offset = 0

    init(auth: AuthStore) {
        self.api = auth.client()
        // Debug: start on a given filter for screenshots.
        if let f = ProcessInfo.processInfo.environment["ASHEN_DEBUG_FILTER"],
           let lf = LibraryFilter(rawValue: f) {
            filter = lf
        }
    }

    private static let iso = ISO8601DateFormatter()

    private func query(offset: Int) -> String {
        var items = ["limit=\(pageSize)", "offset=\(offset)"]
        if let p = filter.param { items.append(p) }
        if sortOldest { items.append("sort=oldest") }
        let cal = Calendar.current
        if let f = fromDate {
            items.append("from=" + Self.iso.string(from: cal.startOfDay(for: f)))
        }
        if let t = toDate,
           let end = cal.date(bySettingHour: 23, minute: 59, second: 59, of: t) {
            items.append("to=" + Self.iso.string(from: end))
        }
        return "?" + items.joined(separator: "&")
    }

    /// Reloads the first page after a sort/date change.
    func applyFilters() { Task { await reloadAssets() } }

    func load() async {
        loading = true
        defer { loading = false }
        do {
            async let a = api.listAssets(query: query(offset: 0))
            async let al = api.listAlbums()
            let first = try await a
            assets = first
            offset = first.count
            hasMore = first.count >= pageSize
            albums = try await al
            error = nil
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Loads the next page and appends (infinite scroll).
    func loadMore() async {
        guard hasMore, !loadingMore, !loading else { return }
        loadingMore = true
        defer { loadingMore = false }
        do {
            let next = try await api.listAssets(query: query(offset: offset))
            let existing = Set(assets.map(\.id))
            assets.append(contentsOf: next.filter { !existing.contains($0.id) })
            offset += next.count
            hasMore = next.count >= pageSize
        } catch {
            self.error = error.localizedDescription
        }
    }

    /// Sets the filter synchronously (so the segmented control updates immediately)
    /// then reloads from the first page.
    func setFilter(_ f: LibraryFilter) {
        filter = f
        Task { await reloadAssets() }
    }

    private func reloadAssets() async {
        do {
            let a = try await api.listAssets(query: query(offset: 0))
            assets = a
            offset = a.count
            hasMore = a.count >= pageSize
        } catch { self.error = error.localizedDescription }
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
