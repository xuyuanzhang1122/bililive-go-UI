import Foundation

final class CacheManager {
    static let shared = CacheManager()
    
    private let fileManager = FileManager.default
    private lazy var cacheDirectory: URL = {
        let urls = fileManager.urls(for: .cachesDirectory, in: .userDomainMask)
        return urls[0].appendingPathComponent("APICache")
    }()
    
    private init() {
        if !fileManager.fileExists(atPath: cacheDirectory.path) {
            try? fileManager.createDirectory(at: cacheDirectory, withIntermediateDirectories: true, attributes: nil)
        }
    }
    
    func save<T: Encodable>(_ object: T, forKey key: String) {
        let url = cacheDirectory.appendingPathComponent(key)
        do {
            let data = try JSONEncoder().encode(object)
            try data.write(to: url)
        } catch {
            print("Failed to save cache for key: \(key) - \(error)")
        }
    }
    
    func load<T: Decodable>(forKey key: String, as type: T.Type) -> T? {
        let url = cacheDirectory.appendingPathComponent(key)
        guard let data = try? Data(contentsOf: url) else { return nil }
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            print("Failed to load cache for key: \(key) - \(error)")
            return nil
        }
    }
}
