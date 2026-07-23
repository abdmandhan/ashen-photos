import SwiftUI

struct LibraryView: View {
    @ObservedObject var store: LibraryStore
    @State private var showNewAlbum = false
    @State private var newAlbumName = ""
    @State private var preview: RemoteAsset?

    private let columns = [GridItem(.adaptive(minimum: 108), spacing: 3)]

    var body: some View {
        NavigationStack {
            ScrollView {
                if !store.albums.isEmpty {
                    albumsRow
                }

                Picker("Filter", selection: Binding(
                    get: { store.filter },
                    set: { store.setFilter($0) }
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
                                .onAppear {
                                    if asset.id == store.assets.last?.id {
                                        Task { await store.loadMore() }
                                    }
                                }
                        }
                    }
                    .padding(.horizontal, 3)
                    if store.loadingMore {
                        ProgressView().padding()
                    }
                }
            }
            .navigationTitle("Library")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Button { showNewAlbum = true } label: { Label("New album", systemImage: "rectangle.stack.badge.plus") }
                        Button {
                            Task { await store.backfillThumbnails() }
                        } label: { Label("Backfill thumbnails", systemImage: "photo.badge.arrow.down") }
                            .disabled(store.backfilling)
                    } label: {
                        Image(systemName: "ellipsis.circle")
                    }
                }
            }
            .safeAreaInset(edge: .top) {
                if let status = store.backfillStatus, store.backfilling {
                    HStack(spacing: 8) {
                        ProgressView().controlSize(.small)
                        Text(status).font(.footnote)
                    }
                    .padding(8)
                    .frame(maxWidth: .infinity)
                    .background(.thinMaterial)
                }
            }
            .refreshable { await store.load() }
            .task {
                await store.load()
                // Debug: auto-open a preview for screenshots.
                if ProcessInfo.processInfo.environment["ASHEN_DEBUG_PREVIEW"] == "1" {
                    preview = store.assets.first
                }
            }
            .alert("New album", isPresented: $showNewAlbum) {
                TextField("Name", text: $newAlbumName)
                Button("Create") {
                    let n = newAlbumName; newAlbumName = ""
                    Task { await store.createAlbum(name: n) }
                }
                Button("Cancel", role: .cancel) { newAlbumName = "" }
            }
            .fullScreenCover(item: $preview) { asset in
                AssetPreviewView(assets: store.assets, current: asset.id)
            }
        }
    }

    private var albumsRow: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 12) {
                ForEach(store.albums) { al in
                    NavigationLink {
                        AlbumDetailView(album: al, store: store)
                    } label: {
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
                    .buttonStyle(.plain)
                }
            }
            .padding(.horizontal)
        }
        .padding(.vertical, 8)
    }

    private func tile(_ asset: RemoteAsset) -> some View {
        // Square cell sized by the grid column; the image fills + clips inside it.
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
                if store.albums.isEmpty {
                    Text("No albums yet")
                } else {
                    Menu("Add to album") {
                        ForEach(store.albums) { al in
                            Button(al.name) {
                                Task { await store.addToAlbum(albumID: al.id, assetID: asset.id) }
                            }
                        }
                    }
                }
            }
            .overlay(alignment: .topTrailing) {
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
            }
            .overlay(alignment: .bottomTrailing) {
                if asset.mediaType == "video" {
                    Image(systemName: "play.fill")
                        .font(.caption2).foregroundStyle(.white)
                        .padding(4).background(.black.opacity(0.4), in: Circle())
                        .padding(4)
                }
            }
    }
}
