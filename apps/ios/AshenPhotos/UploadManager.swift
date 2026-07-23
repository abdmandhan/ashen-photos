import Foundation

/// Owns the background URLSession that PUTs originals to presigned storage URLs.
/// Task identity (upload id + temp file path) is carried in `taskDescription`
/// so it survives app relaunch.
final class UploadManager: NSObject {
    static let shared = UploadManager()

    /// Called on the main actor when an upload finishes. `reason` is nil on success.
    var onFinish: ((_ uploadID: String, _ success: Bool, _ reason: String?) -> Void)?

    /// Set by the AppDelegate for background-session completion.
    var backgroundCompletion: (() -> Void)?

    private lazy var session: URLSession = {
        let cfg = URLSessionConfiguration.background(withIdentifier: "test.ashen.photos.upload")
        cfg.isDiscretionary = false
        cfg.sessionSendsLaunchEvents = true
        return URLSession(configuration: cfg, delegate: self, delegateQueue: nil)
    }()

    func start() { _ = session }

    /// Uploads `fileURL` to `putURL` (a presigned PUT), tagging the task with the
    /// upload id and whether a thumbnail was already uploaded for it.
    func upload(fileURL: URL, to putURL: String, uploadID: String, hasThumb: Bool = false) {
        guard let url = URL(string: putURL) else {
            onFinish?(uploadID, false, "Invalid upload URL")
            return
        }
        var req = URLRequest(url: url)
        req.httpMethod = "PUT"
        req.setValue("application/octet-stream", forHTTPHeaderField: "Content-Type")

        let task = session.uploadTask(with: req, fromFile: fileURL)
        task.taskDescription = "\(uploadID)|\(fileURL.path)|\(hasThumb ? "1" : "0")"
        task.resume()
    }
}

extension UploadManager: URLSessionDataDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        let parts = (task.taskDescription ?? "").split(separator: "|", maxSplits: 2).map(String.init)
        let uploadID = parts.first ?? ""
        if parts.count >= 2 { try? FileManager.default.removeItem(atPath: parts[1]) }
        let hasThumb = parts.count >= 3 && parts[2] == "1"

        let http = task.response as? HTTPURLResponse
        let code = http?.statusCode ?? 0
        let ok = error == nil && (200..<300).contains(code)

        var reason: String?
        if !ok {
            if let error { reason = "Upload failed: \(error.localizedDescription)" }
            else { reason = "Storage rejected upload (HTTP \(code))" }
        }

        Task { @MainActor in
            var finalOK = ok
            var finalReason = reason
            if ok {
                // PUT landed; confirm with the API. A failed confirm means the object
                // is orphaned + unverified — surface it so it retries, don't mark done.
                if let confirmErr = await Self.callComplete(uploadID: uploadID, thumb: hasThumb) {
                    finalOK = false
                    finalReason = "Uploaded but confirm failed: \(confirmErr)"
                }
            }
            self.onFinish?(uploadID, finalOK, finalReason)
        }
    }

    func urlSessionDidFinishEvents(forBackgroundURLSession session: URLSession) {
        DispatchQueue.main.async { self.backgroundCompletion?(); self.backgroundCompletion = nil }
    }

    /// Signals the API the object landed; safe to call after a background relaunch
    /// (token from Keychain). Returns nil on success, or a reason string on failure.
    private static func callComplete(uploadID: String, thumb: Bool) async -> String? {
        let api = APIClient(
            tokenProvider: { Keychain.get("token") },
            deviceIDProvider: { UserDefaults.standard.string(forKey: "device_id") }
        )
        do {
            try await api.completeUpload(id: uploadID, thumb: thumb)
            return nil
        } catch {
            return error.localizedDescription
        }
    }
}
