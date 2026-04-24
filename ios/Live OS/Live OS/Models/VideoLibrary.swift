import Foundation

struct VideoRoomInfo: Identifiable, Decodable {
    let hostName: String
    let platform: String
    let folderPath: String
    let videoCount: Int
    let totalSize: Int64
    let latestVideoAt: Int64
    let latestVideo: String?

    var id: String { folderPath }

    var totalSizeFormatted: String {
        ByteCountFormatter.string(fromByteCount: totalSize, countStyle: .file)
    }

    var latestDate: Date {
        Date(timeIntervalSince1970: TimeInterval(latestVideoAt))
    }

    enum CodingKeys: String, CodingKey {
        case hostName = "host_name"
        case platform
        case folderPath = "folder_path"
        case videoCount = "video_count"
        case totalSize = "total_size"
        case latestVideoAt = "latest_video_at"
        case latestVideo = "latest_video"
    }
}

struct VideoFileInfo: Identifiable, Decodable {
    let name: String
    let relPath: String
    let size: Int64
    let modTime: Int64
    let fileURL: String?
    let thumbnailURL: String?
    let hlsURL: String?

    var id: String { relPath }

    var sizeFormatted: String {
        ByteCountFormatter.string(fromByteCount: size, countStyle: .file)
    }

    var modDate: Date {
        Date(timeIntervalSince1970: TimeInterval(modTime))
    }

    var isNativePlayable: Bool {
        let ext = (name as NSString).pathExtension.lowercased()
        return ["mp4", "m4v", "mov"].contains(ext)
    }

    enum CodingKeys: String, CodingKey {
        case name, size
        case relPath = "rel_path"
        case modTime = "mod_time"
        case fileURL = "file_url"
        case thumbnailURL = "thumbnail_url"
        case hlsURL = "hls_url"
    }
}
