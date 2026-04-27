import SwiftUI

struct HistoryView: View {
    @Environment(AppConfig.self) private var appConfig

    @State private var entries: [HistoryEntry] = []
    @State private var isLoading = true
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Group {
                if isLoading && entries.isEmpty {
                    ProgressView("加载中…")
                } else if let err = errorMessage, entries.isEmpty {
                    ContentUnavailableView("无法加载", systemImage: "exclamationmark.triangle", description: Text(err))
                } else if entries.isEmpty {
                    ContentUnavailableView("暂无观看历史", systemImage: "clock", description: Text("观看过的视频会出现在这里"))
                } else {
                    List {
                        ForEach(entries) { entry in
                            VStack(alignment: .leading, spacing: 6) {
                                Text(entry.displayName)
                                    .font(.subheadline.weight(.medium))
                                    .lineLimit(1)
                                HStack(spacing: 10) {
                                    Text("\(formatTime(entry.positionSeconds)) / \(formatTime(entry.durationSeconds))")
                                        .font(.caption.monospacedDigit())
                                        .foregroundStyle(.blue)
                                    Text(entry.updatedAt)
                                        .font(.caption)
                                        .foregroundStyle(.secondary)
                                }
                            }
                            .padding(.vertical, 4)
                        }
                        .onDelete(perform: deleteEntries)
                    }
                    .refreshable { await load() }
                }
            }
            .navigationTitle("观看历史")
            .task { await load() }
        }
    }

    private func load() async {
        isLoading = true
        errorMessage = nil
        do {
            entries = try await fetchHistory()
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }

    private func makeRequest(path: String, method: String = "GET") -> URLRequest {
        let base = appConfig.activeURL.trimmingCharacters(in: .init(charactersIn: "/"))
        var req = URLRequest(url: URL(string: base + path)!)
        req.httpMethod = method
        req.timeoutInterval = 10
        req.setValue("application/json", forHTTPHeaderField: "Accept")
        if !appConfig.apiKey.isEmpty {
            req.setValue("Bearer \(appConfig.apiKey)", forHTTPHeaderField: "Authorization")
        }
        return req
    }

    private func fetchHistory() async throws -> [HistoryEntry] {
        let (data, response) = try await URLSession.shared.data(for: makeRequest(path: "/api/history"))
        if let httpResp = response as? HTTPURLResponse, httpResp.statusCode == 401 {
            throw NSError(domain: "", code: 401, userInfo: [NSLocalizedDescriptionKey: "API Key 鉴权失败，请检查设置"])
        }
        return try JSONDecoder().decode([HistoryEntry].self, from: data)
    }

    private func deleteEntries(at offsets: IndexSet) {
        for idx in offsets {
            let entry = entries[idx]
            let encoded = entry.videoPath.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? entry.videoPath
            let req = makeRequest(path: "/api/history/\(encoded)", method: "DELETE")
            Task { _ = try? await URLSession.shared.data(for: req) }
        }
        entries.remove(atOffsets: offsets)
    }

    private func formatTime(_ seconds: Double) -> String {
        guard seconds.isFinite && seconds > 0 else { return "00:00" }
        let total = Int(seconds.rounded())
        let h = total / 3600, m = (total % 3600) / 60, s = total % 60
        if h > 0 { return String(format: "%d:%02d:%02d", h, m, s) }
        return String(format: "%02d:%02d", m, s)
    }
}

struct HistoryEntry: Identifiable, Codable {
    let id: Int64
    let videoPath: String
    let videoName: String
    let positionSeconds: Double
    let durationSeconds: Double
    let updatedAt: String

    var displayName: String {
        if !videoName.isEmpty { return videoName }
        return videoPath.split(separator: "/").last.map(String.init) ?? "未知视频"
    }

    enum CodingKeys: String, CodingKey {
        case id
        case videoPath = "video_path"
        case videoName = "video_name"
        case positionSeconds = "position_seconds"
        case durationSeconds = "duration_seconds"
        case updatedAt = "updated_at"
    }
}
