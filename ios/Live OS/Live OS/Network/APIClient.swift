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
        req.timeoutInterval = 30
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
        if statusCode >= 400 {
            if let wrappedError = try? decoder.decode(APIResponse<EmptyData>.self, from: data) {
                throw APIError.serverError(wrappedError.errNo == 0 ? statusCode : wrappedError.errNo, wrappedError.errMsg)
            }
            let responseText = String(data: data, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let message = responseText.flatMap { $0.isEmpty ? nil : $0 } ?? "请求失败"
            throw APIError.serverError(statusCode, message)
        }
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

    func addLive(url: String, listen: Bool = true) async throws {
        let body = try JSONEncoder().encode([AddLiveRequest(url: url, listen: listen)])
        let req = try makeRequest("/api/lives", method: "POST", body: body)
        let (_, response) = try await session.data(for: req)
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        if status == 401 || status == 403 { throw APIError.unauthorized }
        if status >= 400 { throw APIError.serverError(status, "添加直播间失败") }
    }

    func deleteLive(id: String, deleteFiles: Bool = false) async throws {
        let body = try JSONEncoder().encode(DeleteLiveRequest(deleteFiles: deleteFiles))
        _ = try await fetch(APIResponse<EmptyData>.self, path: "/api/lives/\(id)", method: "DELETE", body: body)
    }

    func controlLive(id: String, action: String) async throws -> LiveInfo {
        try await fetch(LiveInfo.self, path: "/api/lives/\(id)/\(action)")
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
        let encoded = encodedRelPath(folderPath)
        return try await fetch([VideoFileInfo].self, path: "/api/video-files/\(encoded)")
    }

    // MARK: - File management

    func deleteFile(relPath: String) async throws {
        let encoded = encodedRelPath(relPath)
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
        let encoded = encodedRelPath(relPath)
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

    private static let urlPathSafeChars: CharacterSet = {
        var c = CharacterSet.urlPathAllowed
        c.remove(charactersIn: "[]{}|\\^`\"<>#% ")
        return c
    }()

    private func encodedRelPath(_ relPath: String) -> String {
        relPath.split(separator: "/").map {
            $0.addingPercentEncoding(withAllowedCharacters: Self.urlPathSafeChars) ?? String($0)
        }.joined(separator: "/")
    }

    /// 计算播放 URL：优先使用后端返回的签名 URL；否则按后缀回退到 /api/stream/hls 或 /files
    func playbackURL(for file: VideoFileInfo) -> URL? {
        if let hls = file.hlsURL, let u = makeHLSURL(hlsRelative: hls) { return u }
        if let f = file.fileURL, let u = makeFileURL(fileRelative: f) { return u }
        let encoded = encodedRelPath(file.relPath)
        let path = file.isNativePlayable ? "/files/\(encoded)" : "/api/stream/hls/\(encoded)"
        return absoluteURL(path)
    }

    /// 计算缩略图 URL：优先使用后端返回的签名 URL；否则回退到 /api/thumbnail
    func thumbnailURL(for file: VideoFileInfo) -> URL? {
        if let t = file.thumbnailURL, let u = absoluteURL(t) { return u }
        return thumbnailURL(for: file.relPath)
    }
}
