import SwiftUI

@Observable
final class AppConfig {
    var serverURL: String {
        didSet { UserDefaults.standard.set(serverURL, forKey: "serverURL") }
    }
    var apiKey: String {
        didSet { UserDefaults.standard.set(apiKey, forKey: "apiKey") }
    }

    private(set) lazy var client: APIClient = makeClient()

    init() {
        serverURL = UserDefaults.standard.string(forKey: "serverURL") ?? ""
        apiKey    = UserDefaults.standard.string(forKey: "apiKey") ?? ""
    }

    func applySettings(serverURL: String, apiKey: String) {
        self.serverURL = serverURL
        self.apiKey    = apiKey
        client = makeClient()
    }

    private func makeClient() -> APIClient {
        APIClient(baseURL: serverURL, apiKey: apiKey)
    }
}
