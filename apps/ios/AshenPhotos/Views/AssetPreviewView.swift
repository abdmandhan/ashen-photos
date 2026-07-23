import SwiftUI
import AVKit

struct AssetPreviewView: View {
    let asset: RemoteAsset
    @Environment(\.dismiss) private var dismiss
    @State private var player: AVPlayer?

    private var url: URL? { asset.downloadURL.flatMap(URL.init) }
    private var isVideo: Bool { asset.mediaType == "video" }

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            if isVideo {
                VideoPlayer(player: player)
                    .ignoresSafeArea()
                    .onAppear {
                        if let u = url {
                            let p = AVPlayer(url: u)
                            player = p
                            p.play()
                        }
                    }
                    .onDisappear { player?.pause() }
            } else {
                AsyncImage(url: url) { phase in
                    switch phase {
                    case .success(let img):
                        img.resizable().scaledToFit()
                    case .failure:
                        Image(systemName: "exclamationmark.triangle").foregroundStyle(.white)
                    default:
                        ProgressView().tint(.white)
                    }
                }
            }

            VStack {
                HStack {
                    Button { dismiss() } label: {
                        Image(systemName: "xmark.circle.fill")
                            .font(.title)
                            .foregroundStyle(.white, .black.opacity(0.4))
                    }
                    Spacer()
                }
                Spacer()
            }
            .padding()
        }
    }
}
