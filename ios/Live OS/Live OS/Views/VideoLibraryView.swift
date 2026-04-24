import SwiftUI

struct VideoLibraryView: View {
    @Environment(AppConfig.self) private var appConfig
    @State private var vm: VideoLibraryViewModel?

    let columns = [GridItem(.adaptive(minimum: 160, maximum: 220), spacing: 12)]

    var body: some View {
        NavigationStack {
            Group {
                if let vm {
                    libraryContent(vm)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle("视频库")
            .toolbar {
                ToolbarItem(placement: .secondaryAction) {
                    Button("刷新", systemImage: "arrow.clockwise") {
                        Task { await vm?.load() }
                    }
                }
            }
            .task {
                let model = VideoLibraryViewModel(client: appConfig.client)
                vm = model
                await model.load()
            }
        }
    }

    @ViewBuilder
    private func libraryContent(_ vm: VideoLibraryViewModel) -> some View {
        if vm.isLoading && vm.rooms.isEmpty {
            ProgressView("加载中…")
        } else if let err = vm.errorMessage {
            ContentUnavailableView("无法加载", systemImage: "exclamationmark.triangle", description: Text(err))
        } else if vm.rooms.isEmpty {
            ContentUnavailableView("暂无录播", systemImage: "tray", description: Text("录播完成后会出现在这里"))
        } else {
            ScrollView {
                LazyVGrid(columns: columns, spacing: 12) {
                    ForEach(vm.rooms) { room in
                        NavigationLink(destination: VideoListView(room: room)) {
                            RoomCard(room: room, client: appConfig.client)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding()
            }
            .refreshable { await vm.load() }
        }
    }
}

private struct RoomCard: View {
    let room: VideoRoomInfo
    let client: APIClient

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            AsyncImage(url: thumbnailURL) { phase in
                switch phase {
                case .success(let img):
                    img.resizable().aspectRatio(16/9, contentMode: .fill)
                case .failure:
                    placeholder
                default:
                    Color.secondary.opacity(0.15).aspectRatio(16/9, contentMode: .fill).overlay { ProgressView() }
                }
            }
            .frame(maxWidth: .infinity)
            .clipped()

            VStack(alignment: .leading, spacing: 4) {
                Text(room.hostName).font(.headline).lineLimit(1)
                HStack {
                    Text("\(room.videoCount) 个视频").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Text(room.totalSizeFormatted).font(.caption).foregroundStyle(.secondary)
                }
            }
            .padding(8)
        }
        .background(.secondarySystemGroupedBackground)
        .clipShape(RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.06), radius: 4, y: 2)
    }

    private var thumbnailURL: URL? {
        guard let latest = room.latestVideo else { return nil }
        return client.thumbnailURL(for: "\(room.folderPath)/\(latest)")
    }

    private var placeholder: some View {
        Color.secondary.opacity(0.15)
            .aspectRatio(16/9, contentMode: .fill)
            .overlay { Image(systemName: "video").font(.largeTitle).foregroundStyle(.secondary) }
    }
}
