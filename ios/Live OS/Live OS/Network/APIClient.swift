import Foundation

final class APIClient {
    private let session: URLSession
    private let decoder: JSONDecoder

    var baseURL: String
    var apiKey: String

    init(baseURL: String, apiKey: String) {
        self.baseURL = baseURL
        self.apiKey = apiKey
        self.session = URLSession.shared
        self.decoder = JSONDecoder()
    }

    // MARK: - Request helpers

    private func makeRequest(_ path: String, method: String = "GET", body: Data? = nil) throws -> URLRequest {
        let urlString = baseURL.trimmingCharacters(in: .init(charactersIn: "/")) + path
        guard let url = URL(string: urlString) else { throw APIError.invalidURL }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.timeoutInterval = 15
        if !apiKey.isEmpty {
            req.setValue("Bearer \(apiKey)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            req.httpBody = body
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        }
        return req
    }

    private func fetch<T: Decodable>(_ type: T.Type, path: String, method: String = "GET", body: Data? = nil) async throws -> T {
        let req = try makeRequest(path, method: method, body: body)
        let (data, response) = try await session.data(for: req)
        let statusCode = (response as? HTTPURLResponse)?.statusCode ?? 0
        if statusCode == 401 || statusCode == 403 { throw APIError.unauthorized }
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw APIError.decodingError(error)
        }
    }

    private func fetchWrapped<T: Decodable>(_ type: T.Type, path: String, method: String = "GET", body: Data? = nil) async throws -> T {
        let wrapped = try await fetch(APIResponse<T>.self, path: path, method: method, body: body)
        if wrapped.errNo != 0 {
            throw APIError.serverError(wrapped.errNo, wrapped.errMsg)
        }
        guard let data = wrapped.data else {
            throw APIError.serverError(-1, "响应 data 为空")
        }
        return data
    }

    // MARK: - Live rooms

    func getLives() async throws -> [LiveInfo] {
        try await fetch([LiveInfo].self, path: "/api/lives")
    }

    func addLive(url: String, listen: Bool = true) async throws -> [LiveInfo] {
        let body = try JSONEncoder().encode([AddLiveRequest(url: url, listen: listen)])
        return try await fetch([LiveInfo].self, path: "/api/lives", method: "POST", body: body)
    }

    func deleteLive(id: String, deleteFiles: Bool = false) async throws {
        let body = try JSONEncoder().encode(DeleteLiveRequest(deleteFiles: deleteFiles))
        _ = try await fetch(APIResponse<EmptyData>.self, path: "/api/lives/\(id)", method: "DELETE", body: body)
    }

    func controlLive(id: String, action: String) async throws {
        _ = try await fetch(APIResponse<EmptyData>.self, path: "/api/lives/\(id)/\(action)")
    }

    // MARK: - URL resolver

    func resolveURL(_ rawURL: String) async throws -> String {
        let encoded = rawURL.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? rawURL
        let result = try await fetchWrapped(ResolveURLResult.self, path: "/api/resolve-url?url=\(encoded)")
        return result.url
    }

    // MARK: - Video library

    func getVideoLibrary() async throws -> [VideoRoomInfo] {
        try await fetch([VideoRoomInfo].self, path: "/api/video-library")
    }

    func getVideoFiles(folderPath: String) async throws -> [VideoFileInfo] {
        let encoded = folderPath.split(separator: "/").map {
            $0.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? String($0)
        }.joined(separator: "/")
        return try await fetch([VideoFileInfo].self, path: "/api/video-files/\(encoded)")
    }

    // MARK: - File management

    func deleteFile(relPath: String) async throws {
        let encoded = relPath.split(separator: "/").map {
            $0.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? String($0)
        }.joined(separator: "/")
        _ = try await fetch(APIResponse<EmptyData>.self, path: "/api/file/\(encoded)", method: "DELETE")
    }

    func deleteFiles(relPaths: [String]) async throws {
        let body = try JSONEncoder().encode(["paths": relPaths])
        _ = try await fetch(APIResponse<EmptyData>.self, path: "/api/batch/file/delete", method: "POST", body: body)
    }

    // MARK: - Signed URLs

    func absoluteURL(_ relativeURLString: String) -> URL? {
        let base = baseURL.trimmingCharacters(in: .init(charactersIn: "/"))
        let rel = relativeURLString.hasPrefix("/") ? relativeURLString : "/\(relativeURLString)"
        guard var components = URLComponents(string: base + rel) else { return nil }
        if !apiKey.isEmpty {
            var items = components.queryItems ?? []
            items.append(URLQueryItem(name: "_key", value: apiKey))
            components.queryItems = items
        }
        return components.url
    }

    func thumbnailURL(for relPath: String) -> URL? {
        let encoded = relPath.split(separator: "/").map {
            $0.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? String($0)
        }.joined(separator: "/")
        let path = "/api/thumbnail/\(encoded)"
        let base = baseURL.trimmingCharacters(in: .init(charactersIn: "/"))
        return URL(string: base + path)
    }

    func makeHLSURL(hlsRelative: String) -> URL? {
        absoluteURL(hlsRelative)
    }

    func makeFileURL(fileRelative: String) -> URL? {
        absoluteURL(fileRelative)
    }
}
