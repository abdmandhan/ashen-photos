import SwiftUI
import Photos

struct FreeSpaceView: View {
    @ObservedObject var coordinator: BackupCoordinator
    @State private var showReview = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 20) {
                    recoverableCard

                    HStack(spacing: 12) {
                        statCard("\(coordinator.safePhotoCount)", "Photos")
                        statCard("\(coordinator.safeVideoCount)", "Videos")
                    }

                    if let last = coordinator.lastBackupAt {
                        Text("Last backup \(last.formatted(.relative(presentation: .named)))")
                            .font(.footnote).foregroundStyle(.secondary)
                    }

                    if let s = coordinator.backendStats {
                        VStack(spacing: 6) {
                            Text("Backed up to Ashen").font(.footnote).foregroundStyle(.secondary)
                            HStack(spacing: 16) {
                                Label("\(s.photoCount)", systemImage: "photo")
                                Label("\(s.videoCount)", systemImage: "video")
                                Label(formatBytes(s.totalBytes), systemImage: "internaldrive")
                            }
                            .font(.subheadline)
                        }
                        .padding()
                        .frame(maxWidth: .infinity)
                        .background(Color(.secondarySystemBackground))
                        .clipShape(RoundedRectangle(cornerRadius: 12))
                    }

                    Button {
                        showReview = true
                    } label: {
                        Label("Free Up Space", systemImage: "trash")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(coordinator.safeToDeleteItems.isEmpty)

                    Text("Only photos and videos fully backed up to Ashen are shown here. Removing them frees space on this iPhone; they stay in your backup.")
                        .font(.caption).foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)

                    Spacer()
                }
                .padding()
            }
            .navigationTitle("Storage")
            .task { await coordinator.loadBackendStats(); await coordinator.reconcileFreeSpace() }
            .refreshable { await coordinator.loadBackendStats(); await coordinator.reconcileFreeSpace() }
            .sheet(isPresented: $showReview) {
                FreeSpaceReviewView(coordinator: coordinator)
            }
            .overlay {
                if coordinator.reconciling && coordinator.safeToDeleteItems.isEmpty {
                    ProgressView("Checking backups…")
                }
            }
        }
    }

    private var recoverableCard: some View {
        VStack(spacing: 4) {
            Text(formatBytes(coordinator.recoverableBytes))
                .font(.system(size: 40, weight: .bold))
            Text("Recoverable space")
                .font(.subheadline).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 24)
        .background(Color.blue.opacity(0.1))
        .clipShape(RoundedRectangle(cornerRadius: 16))
    }

    private func statCard(_ value: String, _ label: String) -> some View {
        VStack(spacing: 4) {
            Text(value).font(.title2.bold())
            Text(label).font(.footnote).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 16)
        .background(Color(.secondarySystemBackground))
        .clipShape(RoundedRectangle(cornerRadius: 12))
    }
}

struct FreeSpaceReviewView: View {
    @ObservedObject var coordinator: BackupCoordinator
    @Environment(\.dismiss) private var dismiss

    @State private var phAssets: [PHAsset] = []
    @State private var selected = Set<String>()
    @State private var deleting = false

    private let columns = [GridItem(.adaptive(minimum: 108), spacing: 3)]

    private var selectedBytes: Int64 {
        coordinator.safeToDeleteItems
            .filter { selected.contains($0.id) }
            .reduce(0) { $0 + $1.byteSize }
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    LazyVGrid(columns: columns, spacing: 3) {
                        ForEach(phAssets, id: \.localIdentifier) { asset in
                            ReviewTile(asset: asset, selected: selected.contains(asset.localIdentifier))
                                .onTapGesture { toggle(asset.localIdentifier) }
                        }
                    }
                    .padding(3)
                }
                deleteBar
            }
            .navigationTitle("Review")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(selected.count == phAssets.count ? "Deselect all" : "Select all") {
                        if selected.count == phAssets.count { selected.removeAll() }
                        else { selected = Set(phAssets.map(\.localIdentifier)) }
                    }
                }
            }
            .onAppear(perform: loadAssets)
        }
    }

    private var deleteBar: some View {
        VStack(spacing: 8) {
            Text("\(selected.count) selected · \(formatBytes(selectedBytes)) recoverable")
                .font(.footnote).foregroundStyle(.secondary)
            Button {
                Task { await delete() }
            } label: {
                Label(deleting ? "Deleting…" : "Delete from iPhone", systemImage: "trash")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(.red)
            .disabled(selected.isEmpty || deleting)
        }
        .padding()
        .background(.bar)
    }

    private func loadAssets() {
        let ids = coordinator.safeToDeleteItems.map(\.id)
        let fetch = PHAsset.fetchAssets(withLocalIdentifiers: ids, options: nil)
        var out: [PHAsset] = []
        fetch.enumerateObjects { a, _, _ in out.append(a) }
        phAssets = out
        selected = Set(out.map(\.localIdentifier)) // default: all selected
    }

    private func toggle(_ id: String) {
        if selected.contains(id) { selected.remove(id) } else { selected.insert(id) }
    }

    private func delete() async {
        deleting = true
        defer { deleting = false }
        // iOS presents its own confirmation dialog inside deleteFromDevice.
        let ok = await coordinator.deleteFromDevice(ids: Array(selected))
        if ok { dismiss() }
    }
}

/// A thumbnail loaded from the local photo library (asset is still on device).
private struct ReviewTile: View {
    let asset: PHAsset
    let selected: Bool
    @State private var image: UIImage?

    var body: some View {
        Color.clear
            .aspectRatio(1, contentMode: .fit)
            .overlay {
                if let image {
                    Image(uiImage: image).resizable().scaledToFill()
                } else {
                    Rectangle().fill(.gray.opacity(0.15))
                }
            }
            .clipped()
            .overlay(alignment: .topTrailing) {
                Image(systemName: selected ? "checkmark.circle.fill" : "circle")
                    .foregroundStyle(selected ? Color.blue : Color.white)
                    .padding(6)
                    .shadow(radius: 2)
            }
            .overlay(alignment: .bottomLeading) {
                if asset.mediaType == .video {
                    Image(systemName: "play.fill").font(.caption2).foregroundStyle(.white).padding(4)
                }
            }
            .onAppear(perform: load)
    }

    private func load() {
        let opts = PHImageRequestOptions()
        opts.deliveryMode = .opportunistic
        opts.isNetworkAccessAllowed = true
        PHImageManager.default().requestImage(
            for: asset, targetSize: CGSize(width: 240, height: 240),
            contentMode: .aspectFill, options: opts
        ) { img, _ in
            if let img { image = img }
        }
    }
}
