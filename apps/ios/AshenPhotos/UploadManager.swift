import Foundation

/// Owns the background URLSession that PUTs originals to presigned storage URLs.
/// Task identity (upload id + temp file path) is carried in `taskDescription`
/// so it survives app relaunch.
final class UploadManager: NSObject {
    static let shared = UploadManager()

    /// Called on the main actor when an upload finishes (success or failure).
    var onFinish: ((_ uploadID: String, _ success: Bool) -> Void)?

    /// Set by the AppDelegate for background-session completion.
    var backgroundCompletion: (() -> Void)?

    private lazy var session: URLSession = {
        let cfg = URLSessionConfiguration.background(withIdentifier: "test.ashen.photos.upload")
        cfg.isDiscretionary = false
        cfg.sessionSendsLaunchEvents = true
        return URLSession(configuration: cfg, delegate: self, delegateQueue: nil)
    }()

    func start() { _ = session }

    /// Uploads `fileURL` to `putURL` (a presigned PUT), tagging the task with the upload id.
    func upload(fileURL: URL, to putURL: String, uploadID: String) {
        guard let url = URL(string: putURL) else {
            onFinish?(uploadID, false)
            return
        }
        var req = URLRequest(url: url)
        req.httpMethod = "PUT"
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")

        let task = session.uploadTask(with: req, fromFile: fileURL)
        task.taskDescription = "\(uploadID)|\(fileURL.path)"
        task.resume()
    }
}

extension UploadManager: URLSessionDataDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        let parts = (task.taskDescription ?? "").split(separator: "|", maxSplits: 1).map(String.init)
        let uploadID = parts.first ?? ""
        if parts.count == 2 { try? FileManager.default.removeItem(atPath: parts[1]) }

        let http = task.response as? HTTPURLResponse
        let ok = error == nil && (200..<300).contains(http?.statusCode ?? 0)

        Task { @MainActor in
            if ok { await Self.callComplete(uploadID: uploadID) }
            self.onFinish?(uploadID, ok)
        }
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        DispatchQueue.main.async { self.backgroundCompletion?(); self.backgroundCompletion = nil }
    }

    /// Signals the API the object landed; safe to call after a background relaunch (token from Keychain).
    private static func callComplete(uploadID: String) async {
        let api = APIClient(tokenProvider: { Keychain.get("token") })
        try? await api.completeUpload(id: uploadID)
    }
}
