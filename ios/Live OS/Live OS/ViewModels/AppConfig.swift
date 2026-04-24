import Foundation
import Network

@Observable
final class AppConfig {
    var serverURL: String {
        didSet {
            UserDefaults.standard.set(serverURL, forKey: "serverURL")
            _cachedClient = nil
        }
    }

    var lanURL: String {
        didSet {
            UserDefaults.standard.set(lanURL, forKey: "lanURL")
            _cachedClient = nil
        }
    }

    var publicURL: String {
        didSet {
            UserDefaults.standard.set(publicURL, forKey: "publicURL")
            _cachedClient = nil
        }
    }

    var autoSwitchNetwork: Bool {
        didSet {
            UserDefaults.standard.set(autoSwitchNetwork, forKey: "autoSwitchNetwork")
            if autoSwitchNetwork {
                startNetworkMonitor()
            } else {
                stopNetworkMonitor()
            }
        }
    }

    var apiKey: String {
        didSet {
            UserDefaults.standard.set(apiKey, forKey: "apiKey")
            _cachedClient = nil
        }
    }

    /// The currently active URL (auto-switched or manual)
    var activeURL: String {
        if autoSwitchNetwork {
            if !lanURL.isEmpty && !publicURL.isEmpty {
                return isOnLAN ? lanURL : publicURL
            }
            // Partial config: use whichever is filled
            if !lanURL.isEmpty { return lanURL }
            if !publicURL.isEmpty { return publicURL }
        }
        return serverURL
    }

    /// Whether we're currently detected as being on the LAN
    private(set) var isOnLAN: Bool = false

    @ObservationIgnored
    private var _cachedClient: APIClient?

    @ObservationIgnored
    private var monitor: NWPathMonitor?

    @ObservationIgnored
    private var monitorQueue = DispatchQueue(label: "NetworkMonitor")

    @ObservationIgnored
    private var lanCheckTask: Task<Void, Never>?

    var client: APIClient {
        let url = activeURL
        if let existing = _cachedClient, existing.baseURL == url {
            return existing
        }
        let fresh = APIClient(baseURL: url, apiKey: apiKey)
        _cachedClient = fresh
        return fresh
    }

    init() {
        serverURL = UserDefaults.standard.string(forKey: "serverURL") ?? ""
        lanURL = UserDefaults.standard.string(forKey: "lanURL") ?? ""
        publicURL = UserDefaults.standard.string(forKey: "publicURL") ?? ""
        autoSwitchNetwork = UserDefaults.standard.bool(forKey: "autoSwitchNetwork")
        apiKey = UserDefaults.standard.string(forKey: "apiKey") ?? ""

        if autoSwitchNetwork && !lanURL.isEmpty && !publicURL.isEmpty {
            startNetworkMonitor()
        }
    }

    func applySettings(serverURL: String, apiKey: String) {
        self.serverURL = serverURL
        self.apiKey = apiKey
    }

    func applyNetworkSettings(lanURL: String, publicURL: String, autoSwitch: Bool) {
        self.lanURL = lanURL
        self.publicURL = publicURL
        self.autoSwitchNetwork = autoSwitch
    }

    // MARK: - Network Monitoring

    private func startNetworkMonitor() {
        stopNetworkMonitor()

        monitor = NWPathMonitor()
        monitor?.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            // When network changes, re-check LAN connectivity
            self.checkLANConnectivity()
        }
        monitor?.start(queue: monitorQueue)

        // Initial check
        checkLANConnectivity()
    }

    private func stopNetworkMonitor() {
        monitor?.cancel()
        monitor = nil
        lanCheckTask?.cancel()
        lanCheckTask = nil
    }

    private func checkLANConnectivity() {
        lanCheckTask?.cancel()
        lanCheckTask = Task { [weak self] in
            guard let self, !self.lanURL.isEmpty else { return }

            let lanReachable = await self.pingServer(self.lanURL)
            if !Task.isCancelled {
                await MainActor.run {
                    self.isOnLAN = lanReachable
                }
            }
        }
    }

    /// Try a quick HTTP HEAD to see if the server at the given URL is reachable
    private func pingServer(_ urlString: String) async -> Bool {
        let trimmed = urlString.trimmingCharacters(in: .init(charactersIn: "/"))
        guard let url = URL(string: trimmed + "/api/info") else { return false }

        var req = URLRequest(url: url)
        req.httpMethod = "HEAD"
        req.timeoutInterval = 2

        if !apiKey.isEmpty {
            req.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        }

        do {
            let (_, response) = try await URLSession.shared.data(for: req)
            let status = (response as? HTTPURLResponse)?.statusCode ?? 0
            return status == 200
        } catch {
            return false
        }
    }

    /// Manually trigger a LAN check (e.g., user pulls to refresh)
    func refreshNetworkStatus() {
        if autoSwitchNetwork && !lanURL.isEmpty && !publicURL.isEmpty {
            checkLANConnectivity()
        }
    }
}
