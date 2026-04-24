import Foundation

struct LiveInfo: Identifiable, Codable, Equatable {
    let liveId: String
    let hostName: String
    let roomName: String
    let status: Bool
    let listening: Bool
    let recording: Bool

    var id: String { liveId }

    enum CodingKeys: String, CodingKey {
        case liveId = "id"
        case hostName = "host_name"
        case roomName = "room_name"
        case status, listening, recording
    }
}

struct AddLiveRequest: Encodable {
    let url: String
    let listen: Bool

    init(url: String, listen: Bool = true) {
        self.url = url
        self.listen = listen
    }
}

struct DeleteLiveRequest: Encodable {
    let deleteFiles: Bool

    enum CodingKeys: String, CodingKey {
        case deleteFiles = "delete_files"
    }
}
