import SwiftUI

struct LibraryView: View {
    @ObservedObject var store: LibraryStore
    @State private var showNewAlbum = false
    @State private var newAlbumName = ""

    private let columns = [GridItem(.adaptive(minimum: 108), spacing: 3)]

    var body: some View {
        NavigationStack {
            ScrollView {
                if !store.albums.isEmpty {
                    albumsRow
                }

                Picker("Filter", selection: Binding(
                    get: { store.filter },
                    set: { f in Task { await store.setFilter(f) } }
                )) {
                    ForEach(LibraryFilter.allCases) { Text($0.title).tag($0) }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal)
                .padding(.bottom, 8)

                if store.assets.isEmpty {
                    Text(store.loading ? "Loading…" : "Nothing here.")
                        .foregroundStyle(.secondary)
                        .padding(.top, 40)
                } else {
                    LazyVGrid(columns: columns, spacing: 3) {
                        ForEach(store.assets) { asset in
                            tile(asset)
                        }
                    }
                    .padding(.horizontal, 3)
                }
            }
            .navigationTitle("Library")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { showNewAlbum = true } label: { Image(systemName: "rectangle.stack.badge.plus") }
                }
            }
            .refreshable { await store.load() }
            .task { await store.load() }
            .alert("New album", isPresented: $showNewAlbum) {
                TextField("Name", text: $newAlbumName)
                Button("Create") {
                    let n = newAlbumName; newAlbumName = ""
                    Task { await store.createAlbum(name: n) }
                }
                Button("Cancel", role: .cancel) { newAlbumName = "" }
            }
        }
    }

    private var albumsRow: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 12) {
                ForEach(store.albums) { al in
                    VStack(alignment: .leading, spacing: 4) {
                        AsyncImage(url: al.coverURL.flatMap(URL.init)) { img in
                            img.resizable().scaledToFill()
                        } placeholder: {
                            Rectangle().fill(.gray.opacity(0.2))
                        }
                        .frame(width: 110, height: 110)
                        .clipShape(RoundedRectangle(cornerRadius: 10))
                        Text(al.name).font(.subheadline).lineLimit(1)
                        Text("\(al.assetCount)").font(.caption).foregroundStyle(.secondary)
                    }
                    .frame(width: 110)
                }
            }
            .padding(.horizontal)
        }
        .padding(.vertical, 8)
    }

    private func tile(_ asset: RemoteAsset) -> some View {
        ZStack(alignment: .topTrailing) {
            AsyncImage(url: asset.thumbURL.flatMap(URL.init)) { img in
                img.resizable().scaledToFill()
            } placeholder: {
                Rectangle().fill(.gray.opacity(0.15))
            }
            .frame(minWidth: 0, maxWidth: .infinity)
            .aspectRatio(1, contentMode: .fill)
            .clipped()

            Button {
                Task { await store.toggleFavorite(asset) }
            } label: {
                Image(systemName: asset.favorite ? "heart.fill" : "heart")
                    .font(.footnote)
                    .foregroundStyle(asset.favorite ? .pink : .white)
                    .padding(6)
                    .background(.black.opacity(0.35), in: Circle())
            }
            .padding(4)

            if asset.mediaType == "video" {
                Image(systemName: "play.fill")
                    .font(.caption2).foregroundStyle(.white)
                    .padding(4).background(.black.opacity(0.4), in: Circle())
                    .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .bottomTrailing)
                    .padding(4)
            }
        }
    }
}
