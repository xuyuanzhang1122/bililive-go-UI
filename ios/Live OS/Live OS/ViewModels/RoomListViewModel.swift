import SwiftUI

@Observable
final class RoomListViewModel {
    var rooms: [LiveInfo] = []
    var isLoading = false
    var errorMessage: String?

    private let client: APIClient

    init(client: APIClient) {
        self.client = client
    }

    @MainActor
    func load() async {
        let cacheKey = "RoomListLiveInfo"
        // 1. 先读取本地缓存，快速显示
        if let cached: [LiveInfo] = CacheManager.shared.load(forKey: cacheKey, as: [LiveInfo].self), rooms.isEmpty {
            self.rooms = cached
        }
        
        // 2. 然后再去取最新数据
        isLoading = rooms.isEmpty
        errorMessage = nil
        do {
            let newRooms = try await client.getLives()
            self.rooms = newRooms
            CacheManager.shared.save(newRooms, forKey: cacheKey)
        } catch {
            if rooms.isEmpty {
                errorMessage = error.localizedDescription
            }
        }
        isLoading = false
    }

    @MainActor
    func addRoom(rawInput: String) async throws {
        var url = rawInput.trimmingCharacters(in: .whitespacesAndNewlines)
        // 如果不是 live.douyin.com 链接，先尝试解析
        if !url.lowercased().contains("live.douyin.com") {
            url = try await client.resolveURL(url)
        }
        _ = try await client.addLive(url: url)
        await load()
    }

    @MainActor
    func deleteRoom(_ room: LiveInfo, deleteFiles: Bool = false) async throws {
        try await client.deleteLive(id: room.liveId, deleteFiles: deleteFiles)
        rooms.removeAll { $0.id == room.id }
    }

    @MainActor
    func toggleListening(_ room: LiveInfo) async throws {
        let action = room.listening ? "stop" : "start"
        let updatedRoom = try await client.controlLive(id: room.liveId, action: action)
        if let index = rooms.firstIndex(where: { $0.id == room.id }) {
            rooms[index] = updatedRoom
        } else {
            await load()
        }
    }
}
