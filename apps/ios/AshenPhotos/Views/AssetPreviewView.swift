import SwiftUI
import AVKit

struct AssetPreviewView: View {
    let assets: [RemoteAsset]
    @State var current: String            // currently-shown asset id
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ZStack(alignment: .topLeading) {
            Color.black.ignoresSafeArea()

            TabView(selection: $current) {
                ForEach(assets) { asset in
                    AssetPage(asset: asset, isActive: asset.id == current)
                        .tag(asset.id)
                }
            }
            .tabViewStyle(.page(indexDisplayMode: .never))
            .ignoresSafeArea()

            Button { dismiss() } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.title)
                    .foregroundStyle(.white, .black.opacity(0.4))
            }
            .padding()
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
