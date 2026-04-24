import Foundation

struct APIResponse<T: Decodable>: Decodable {
    let errNo: Int
    let errMsg: String
    let data: T?

    enum CodingKeys: String, CodingKey {
        case errNo = "err_no"
        case errMsg = "err_msg"
        case data
    }
}

struct EmptyData: Decodable {}

struct SignedURLData: Decodable {
    let url: String
    let expires: Int64
    let expiresIn: Int

    enum CodingKeys: String, CodingKey {
        case url, expires
        case expiresIn = "expires_in"
    }
}

struct ResolveURLResult: Decodable {
    let url: String
}

struct ServerInfo: Decodable {
    let appName: String
    let appVersion: String
    let platform: String

    enum CodingKeys: String, CodingKey {
        case appName = "app_name"
        case appVersion = "app_version"
        case platform
    }
}

enum APIError: LocalizedError {
    case invalidURL
    case unauthorized
    case serverError(Int, String)
    case networkError(Error)
    case decodingError(Error)

    var errorDescription: String? {
        switch self {
        case .invalidURL:               return "无效的服务器地址"
        case .unauthorized:             return "API Key 错误，请在设置中检查"
        case .serverError(let c, let m): return "服务器错误 \(c): \(m)"
        case .networkError(let e):      return "网络错误: \(e.localizedDescription)"
        case .decodingError(let e):     return "数据解析失败: \(e.localizedDescription)"
        }
    }
}
