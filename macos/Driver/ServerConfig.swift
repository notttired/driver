import Foundation

/// Where the app points its HTTP calls.
///
/// The mac app runs on the SAME machine as `driver serve`, so its fallback is
/// loopback. A stored value always wins; the fallback is only for "nothing
/// usable is stored". Nothing in this app writes the key (`defaults write
/// dev.notttired.DriverMac serverURL …` is the override path — the UserDefaults
/// domain is the bundle id), and the failure notice offers the way back to the
/// default.
enum ServerConfig {
    /// `http://127.0.0.1:7781` — the address `driver serve` binds by default
    /// (docs/DESIGN.md "Names, paths, knobs"; `-p` / `DRIVER_PORT` move it).
    static let fallbackURL = URL(string: "http://127.0.0.1:7781")!

    /// Resolves the stored server string to a usable base URL.
    ///
    /// "Usable" is stricter than `URL(string:)` succeeding: that initializer
    /// accepts a bare `127.0.0.1:7781` by reading `127.0.0.1` as the SCHEME,
    /// producing a hostless URL that every request built on it fails against.
    /// So a stored value only wins when it carries both a scheme and a host.
    static func baseURL(stored: String?) -> URL {
        guard let raw = stored?.trimmingCharacters(in: .whitespacesAndNewlines),
              !raw.isEmpty,
              let url = URL(string: raw),
              let scheme = url.scheme, !scheme.isEmpty,
              let host = url.host(), !host.isEmpty
        else { return fallbackURL }
        return url
    }
}
