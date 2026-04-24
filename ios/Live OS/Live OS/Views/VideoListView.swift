import SwiftUI

struct VideoListView: View {
    let room: VideoRoomInfo
    @Environment(AppConfig.self) private var appConfig
    @State private var vm: VideoListViewModel?
    @State private var editMode: EditMode = .inactive
    @State private var deleteError: String?
    @State private var selectedFile: VideoFileInfo?

    var body: some View {
        Group {
            if let vm {
                fileList(vm)
            } else {
                ProgressView()
            }
        }
        .navigationTitle(room.hostName)
        .navigationBarTitleDisplayMode(.large)
        .toolbar { toolbar }
        .environment(\.editMode, $editMode)
        .task {
            let model = VideoListViewModel(client: appConfig.client, room: room)
            vm = model
            await model.load()
        }
        .fullScreenCover(item: $selectedFile) { file in
            PlayerView(file: file, client: appConfig.client)
        }
        .alert("删除失败", isPresented: .constant(deleteError != nil), presenting: deleteError) { _ in
            Button("好") { deleteError = nil }
        } message: { Text($0) }
    }

    @ViewBuilder
    private func fileList(_ vm: VideoListViewModel) -> some View {
        @Bindable var vm = vm
        if vm.isLoading && vm.files.isEmpty {
            ProgressView("加载中…")
        } else if let err = vm.errorMessage {
            ContentUnavailableView("无法加载", systemImage: "exclamationmark.triangle", description: Text(err))
        } else if vm.files.isEmpty {
            ContentUnavailableView("暂无视频", systemImage: "tray", description: Text("该主播尚无录播文件"))
        } else {
            List(selection: $vm.selection) {
                ForEach(vm.files) { file in
                    FileRow(file: file, client: appConfig.client)
                        .contentShape(Rectangle())
                        .onTapGesture {
                            if editMode == .inactive { selectedFile = file }
                        }
                        .swipeActions(edge: .trailing) {
                            Button("删除", role: .destructive) {
                                Task {
                                    do { try await vm.deleteFile(file) }
                                    catch { deleteError = error.localizedDescription }
                                }
                            }
                        }
                }
            }
            .refreshable { await vm.load() }
        }
    }

    @ToolbarContentBuilder
    private var toolbar: some ToolbarContent {
        ToolbarItem(placement: .secondaryAction) {
            Button("刷新", systemImage: "arrow.clockwise") {
                Task { await vm?.load() }
            }
        }
        if let vm, !vm.files.isEmpty {
            ToolbarItem(placement: .primaryAction) {
                if editMode == .active {
                    Button("删除(\(vm.selection.count))") {
                        Task {
                            do { try await vm.deleteSelected() }
                            catch { deleteError = error.localizedDescription }
                            editMode = .inactive
                        }
                    }
                    .disabled(vm.selection.isEmpty)
                    .tint(.red)
                } else {
                    EditButton()
                }
            }
        }
    }
}

private struct FileRow: View {
    let file: VideoFileInfo
    let client: APIClient

    var body: some View {
        HStack(spacing: 12) {
            AsyncImage(url: client.thumbnailURL(for: file)) { phase in
                if case .success(let img) = phase {
                    ZStack {
                        img.resizable()
                            .aspectRatio(16/9, contentMode: .fill)
                            .blur(radius: 8)
                            .scaleEffect(1.2)
                        Color.black.opacity(0.3)
                        img.resizable()
                            .aspectRatio(contentMode: .fit)
                    }
                    .frame(width: 80, height: 45).clipped()
                } else {
                    RoundedRectangle(cornerRadius: 4).fill(.secondary.opacity(0.15)).frame(width: 80, height: 45)
                        .overlay { Image(systemName: "video").foregroundStyle(.secondary) }
                }
            }
            .clipShape(RoundedRectangle(cornerRadius: 4))

            VStack(alignment: .leading, spacing: 4) {
                Text(file.name).font(.subheadline).lineLimit(2)
                HStack {
                    Text(file.sizeFormatted).font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Text(file.modDate, style: .date).font(.caption).foregroundStyle(.secondary)
                }
            }
        }
        .padding(.vertical, 4)
    }
}
