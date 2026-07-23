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

    private let api: APIClient

    init(auth: AuthStore) { self.api = auth.client() }

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

    func setFilter(_ f: LibraryFilter) async {
        filter = f
        do { assets = try await api.listAssets(query: f.query) }
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
}
