import SwiftUI
import AVKit
import Photos

struct AssetPreviewView: View {
    let assets: [RemoteAsset]
    @State var current: String            // currently-shown asset id
    @Environment(\.dismiss) private var dismiss
    @State private var saving = false
    @State private var saveMessage: String?

    private var currentAsset: RemoteAsset? { assets.first { $0.id == current } }

    var body: some View {
        ZStack(alignment: .top) {
            Color.black.ignoresSafeArea()

            TabView(selection: $current) {
                ForEach(assets) { asset in
                    AssetPage(asset: asset, isActive: asset.id == current)
                        .tag(asset.id)
                }
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .ignoresSafeArea()

            HStack {
                Button { dismiss() } label: {
                    Image(systemName: "xmark.circle.fill")
                        .font(.title).foregroundStyle(.white, .black.opacity(0.4))
                }
                Spacer()
                Button { Task { await download() } } label: {
                    Image(systemName: saving ? "arrow.down.circle" : "square.and.arrow.down.fill")
                        .font(.title).foregroundStyle(.white, .black.opacity(0.4))
                }
                .disabled(saving)
            }
            .padding()

            if let msg = saveMessage {
                Text(msg)
                    .font(.footnote).padding(8).background(.ultraThinMaterial, in: Capsule())
                    .padding(.top, 60)
            }
        }
    }

    /// Downloads the backed-up original and saves it to the device photo library.
    private func download() async {
        guard let asset = currentAsset, let urlStr = asset.downloadURL, let url = URL(string: urlStr) else { return }
        saving = true
        defer { saving = false }
        do {
            let (data, _) = try await URLSession.shared.data(from: url)
            let ext = asset.mediaType == "video" ? "mov" : "jpg"
            let tmp = FileManager.default.temporaryDirectory
                .appendingPathComponent(UUID().uuidString).appendingPathExtension(ext)
            try data.write(to: tmp)
            try await PHPhotoLibrary.shared().performChanges {
                let req = PHAssetCreationRequest.forAsset()
                req.addResource(with: asset.mediaType == "video" ? .video : .photo, fileURL: tmp, options: nil)
            }
            try? FileManager.default.removeItem(at: tmp)
            showMessage("Saved to Photos")
        } catch {
            showMessage("Save failed")
        }
    }

    private func showMessage(_ text: String) {
        saveMessage = text
        Task {
            try? await Task.sleep(nanoseconds: 2_000_000_000)
            if saveMessage == text { saveMessage = nil }
        }
    }
}

/// One page: a fit-to-screen photo, or an autoplaying video when it's the active page.
private struct AssetPage: View {
    let asset: RemoteAsset
    let isActive: Bool
    @State private var player: AVPlayer?

    private var url: URL? { asset.downloadURL.flatMap(URL.init) }

    var body: some View {
        Group {
            if asset.mediaType == "video" {
                VideoPlayer(player: player)
                    .onAppear { setupAndMaybePlay() }
                    .onChange(of: isActive) { _, active in
                        active ? player?.play() : player?.pause()
                    }
                    .onDisappear { player?.pause() }
            } else {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .success(let img): img.resizable().scaledToFit()
                    case .failure: Image(systemName: "exclamationmark.triangle").foregroundStyle(.white)
                    default: ProgressView().tint(.white)
                    }
                }
            }
        }
    }

    private func setupAndMaybePlay() {
        guard player == nil, let u = url else { return }
        let p = AVPlayer(url: u)
        player = p
        if isActive { p.play() }
    }
}
