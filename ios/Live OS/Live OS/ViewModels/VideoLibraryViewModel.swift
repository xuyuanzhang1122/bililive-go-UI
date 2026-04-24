import SwiftUI

@Observable
final class VideoLibraryViewModel {
    var rooms: [VideoRoomInfo] = []
    var isLoading = false
    var errorMessage: String?

    private let client: APIClient

    init(client: APIClient) {
        self.client = client
    }

    @MainActor
    func load() async {
        isLoading = true
        errorMessage = nil
        do {
            rooms = try await client.getVideoLibrary()
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }
}

@Observable
final class VideoListViewModel {
    var files: [VideoFileInfo] = []
    var isLoading = false
    var errorMessage: String?
    var selection: Set<String> = []

    private let client: APIClient
    let room: VideoRoomInfo

    init(client: APIClient, room: VideoRoomInfo) {
        self.client = client
        self.room   = room
    }

    @MainActor
    func load() async {
        isLoading = true
        errorMessage = nil
        do {
            files = try await client.getVideoFiles(folderPath: room.folderPath)
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }

    @MainActor
    func deleteFile(_ file: VideoFileInfo) async throws {
        try await client.deleteFile(relPath: file.relPath)
        files.removeAll { $0.id == file.id }
    }

    @MainActor
    func deleteSelected() async throws {
        let paths = Array(selection)
        try await client.deleteFiles(relPaths: paths)
        files.removeAll { selection.contains($0.id) }
        selection.removeAll()
    }
}
