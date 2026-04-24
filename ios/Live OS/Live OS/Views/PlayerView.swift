import SwiftUI
import AVKit

struct PlayerView: View {
    let file: VideoFileInfo
    let client: APIClient

    @State private var player: AVPlayer?
    @State private var isLoading = true
    @State private var errorMessage: String?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            Group {
                if let player {
                    VideoPlayer(player: player)
                        .ignoresSafeArea()
                } else if let err = errorMessage {
                    ContentUnavailableView("无法播放", systemImage: "exclamationmark.triangle", description: Text(err))
                } else {
                    ProgressView("准备播放…")
                }
            }
            .navigationTitle(file.name)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("关闭") { dismiss() }
                }
            }
            .task { await preparePlayer() }
            .onDisappear { player?.pause() }
        }
    }

    private func preparePlayer() async {
        // 优先：HLS（FLV/TS/MKV 需转封装）
        if let hlsString = file.hlsURL, let hlsURL = client.makeHLSURL(hlsRelative: hlsString) {
            player = makePlayer(url: hlsURL)
            return
        }
        // 次选：直接文件 URL（MP4/MOV 等原生支持格式）
        if let fileString = file.fileURL, let fileURL = client.makeFileURL(fileRelative: fileString) {
            player = makePlayer(url: fileURL)
            return
        }
        errorMessage = "无法获取播放地址"
    }

    private func makePlayer(url: URL) -> AVPlayer {
        let item = AVPlayerItem(url: url)
        let p = AVPlayer(playerItem: item)
        p.play()
        return p
    }
}
