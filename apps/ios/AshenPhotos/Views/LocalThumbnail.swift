import SwiftUI
import Photos

/// Loads a thumbnail for an on-device asset by its local identifier.
struct LocalThumbnail: View {
    let localIdentifier: String
    var side: CGFloat = 44
    @State private var image: UIImage?

    var body: some View {
        Group {
            if let image {
                Image(uiImage: image).resizable().scaledToFill()
            } else {
                Rectangle().fill(.gray.opacity(0.2))
            }
        }
        .frame(width: side, height: side)
        .clipShape(RoundedRectangle(cornerRadius: 6))
        .onAppear(perform: load)
    }

    private func load() {
        guard image == nil,
              let asset = PHAsset.fetchAssets(withLocalIdentifiers: [localIdentifier], options: nil).firstObject
        else { return }
        let opts = PHImageRequestOptions()
        opts.deliveryMode = .opportunistic
        opts.isNetworkAccessAllowed = true
        PHImageManager.default().requestImage(
            for: asset, targetSize: CGSize(width: side * 2, height: side * 2),
            contentMode: .aspectFill, options: opts
        ) { img, _ in if let img { image = img } }
    }
}
