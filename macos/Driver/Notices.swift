import AppKit
import UserNotifications
import WebKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        Notices.shared.start()
    }
}

@MainActor
final class Notices: NSObject, UNUserNotificationCenterDelegate {
    static let shared = Notices()

    private struct Notice: Decodable {
        let slug: String
        let thread: Int
        let title: String
        let body: String
    }

    private struct Batch: Decodable {
        let notices: [Notice]
    }

    private weak var webView: WKWebView?
    private var pageReady = false
    private var pending: (slug: String, thread: Int)?

    func start() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert]) { granted, _ in
            guard granted else { return }
            Task { @MainActor in Notices.shared.listen() }
        }
    }

    func pageLoading() {
        pageReady = false
    }

    func pageLoaded(_ view: WKWebView) {
        webView = view
        pageReady = true
        flush()
    }

    private func listen() {
        let base = ServerConfig.baseURL(stored: UserDefaults.standard.string(forKey: "serverURL"))
        var request = URLRequest(url: base.appending(path: "api/notices"))
        request.timeoutInterval = 60
        Task {
            while true {
                do {
                    let (data, response) = try await URLSession.shared.data(for: request)
                    guard (response as? HTTPURLResponse)?.statusCode == 200 else {
                        try? await Task.sleep(for: .seconds(5))
                        continue
                    }
                    for notice in try JSONDecoder().decode(Batch.self, from: data).notices {
                        post(notice)
                    }
                } catch {
                    try? await Task.sleep(for: .seconds(2))
                }
            }
        }
    }

    private func post(_ notice: Notice) {
        let content = UNMutableNotificationContent()
        content.title = notice.title
        content.body = notice.body
        content.userInfo = ["slug": notice.slug, "thread": notice.thread]
        UNUserNotificationCenter.current().add(
            UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        )
    }

    private func open(slug: String, thread: Int) {
        NSApp.activate(ignoringOtherApps: true)
        for window in NSApp.windows where window.canBecomeMain {
            if window.isMiniaturized { window.deminiaturize(nil) }
            window.makeKeyAndOrderFront(nil)
        }
        pending = (slug, thread)
        flush()
    }

    private func flush() {
        guard pageReady, let webView, let ask = pending,
              let json = try? JSONSerialization.data(withJSONObject: ["slug": ask.slug, "thread": ask.thread]),
              let literal = String(data: json, encoding: .utf8)
        else { return }
        pending = nil
        webView.evaluateJavaScript(
            "window.driverPendingOpen = \(literal); window.dispatchEvent(new CustomEvent('driver:open'));"
        )
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .list])
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let info = response.notification.request.content.userInfo
        if let slug = info["slug"] as? String {
            let thread = info["thread"] as? Int ?? 0
            Task { @MainActor in Notices.shared.open(slug: slug, thread: thread) }
        }
        completionHandler()
    }
}
