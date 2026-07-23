import SwiftUI

struct AlbumDetailView: View {
    let album: RemoteAlbum
    @ObservedObject var store: LibraryStore
    @Environment(\.dismiss) private var dismiss

    @State private var assets: [RemoteAsset] = []
    @State private var loading = true
    @State private var preview: RemoteAsset?
    @State private var confirmDelete = false

    private let columns = [GridItem(.adaptive(minimum: 108), spacing: 3)]

    var body: some View {
        ScrollView {
            if loading {
                ProgressView().padding(.top, 40)
            } else if assets.isEmpty {
                Text("No items yet. Add photos from the Library.")
                    .foregroundStyle(.secondary).padding(.top, 40)
            } else {
                LazyVGrid(columns: columns, spacing: 3) {
                    ForEach(assets) { asset in
                        tile(asset)
                    }
                }
                .padding(.horizontal, 3)
            }
        }
        .navigationTitle(album.name)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button(role: .destructive) { confirmDelete = true } label: {
                    Image(systemName: "trash")
                }
            }
        }
        .task { await reload() }
        .fullScreenCover(item: $preview) { asset in
            AssetPreviewView(assets: assets, current: asset.id)
        }
        .confirmationDialog("Delete album “\(album.name)”?", isPresented: $confirmDelete, titleVisibility: .visible) {
            Button("Delete album", role: .destructive) {
                Task { await store.deleteAlbum(id: album.id); dismiss() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("The album is removed. Your photos stay in your library.")
        }
    }

    private func reload() async {
        loading = true
        assets = await store.albumAssets(id: album.id)
        loading = false
    }

    private func tile(_ asset: RemoteAsset) -> some View {
        Color.clear
            .aspectRatio(1, contentMode: .fit)
            .overlay {
                AsyncImage(url: asset.thumbURL.flatMap(URL.init)) { img in
                    img.resizable().scaledToFill()
                } placeholder: {
                    Rectangle().fill(.gray.opacity(0.15))
                }
            }
            .clipped()
            .contentShape(Rectangle())
            .onTapGesture { preview = asset }
            .contextMenu {
                Button(role: .destructive) {
                    Task { await store.removeFromAlbum(albumID: album.id, assetID: asset.id); await reload() }
                } label: {
                    Label("Remove from album", systemImage: "minus.circle")
                }
            }
    }
}
