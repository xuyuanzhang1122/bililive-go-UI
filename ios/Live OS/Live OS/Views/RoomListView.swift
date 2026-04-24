import SwiftUI

struct RoomListView: View {
    @Environment(AppConfig.self) private var appConfig
    @State private var vm: RoomListViewModel?
    @State private var showAddSheet = false
    @State private var addInput = ""
    @State private var isAdding = false
    @State private var addError: String?
    @State private var deleteError: String?

    var body: some View {
        NavigationStack {
            Group {
                if let vm {
                    roomList(vm)
                } else {
                    ProgressView()
                }
            }
            .navigationTitle("直播间")
            .toolbar {
                ToolbarItem(placement: .primaryAction) {
                    Button("添加", systemImage: "plus") { showAddSheet = true }
                }
                ToolbarItem(placement: .secondaryAction) {
                    Button("刷新", systemImage: "arrow.clockwise") {
                        Task { await vm?.load() }
                    }
                }
            }
            .sheet(isPresented: $showAddSheet) { addSheet }
            .task {
                let model = RoomListViewModel(client: appConfig.client)
                vm = model
                await model.load()
            }
            .alert("删除失败", isPresented: .constant(deleteError != nil), presenting: deleteError) { _ in
                Button("好") { deleteError = nil }
            } message: { Text($0) }
        }
    }

    @ViewBuilder
    private func roomList(_ vm: RoomListViewModel) -> some View {
        if vm.isLoading && vm.rooms.isEmpty {
            ProgressView("加载中…")
        } else if let err = vm.errorMessage {
            ContentUnavailableView("无法加载", systemImage: "exclamationmark.triangle", description: Text(err))
        } else if vm.rooms.isEmpty {
            ContentUnavailableView("暂无直播间", systemImage: "antenna.radiowaves.left.and.right", description: Text("点右上角 + 添加抖音直播间"))
        } else {
            List {
                ForEach(vm.rooms) { room in
                    RoomRow(room: room, client: appConfig.client) {
                        Task {
                            do { try await vm.deleteRoom(room) }
                            catch { deleteError = error.localizedDescription }
                        }
                    } onToggle: {
                        Task { await vm.toggleListening(room) }
                    }
                }
            }
            .refreshable { await vm.load() }
        }
    }

    private var addSheet: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("抖音直播间链接或分享文案", text: $addInput, axis: .vertical)
                        .lineLimit(3...6)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } footer: {
                    Text("支持 live.douyin.com 链接、抖音短链或完整分享文案，服务器将自动解析。")
                }
                if let err = addError {
                    Section { Text(err).foregroundStyle(.red) }
                }
                Section {
                    Button(action: submitAdd) {
                        HStack {
                            Spacer()
                            if isAdding { ProgressView().tint(.white) }
                            else { Text("添加").bold() }
                            Spacer()
                        }
                    }
                    .disabled(addInput.trimmingCharacters(in: .whitespaces).isEmpty || isAdding)
                }
            }
            .navigationTitle("添加直播间")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("取消") { showAddSheet = false }
                }
            }
        }
    }

    private func submitAdd() {
        guard let vm else { return }
        isAdding = true
        addError = nil
        Task {
            do {
                try await vm.addRoom(rawInput: addInput)
                showAddSheet = false
                addInput = ""
            } catch {
                addError = error.localizedDescription
            }
            isAdding = false
        }
    }
}

private struct RoomRow: View {
    let room: LiveInfo
    let client: APIClient
    let onDelete: () -> Void
    let onToggle: () -> Void

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(room.recording ? .red : room.listening ? .green : .gray.opacity(0.4))
                .frame(width: 10, height: 10)

            VStack(alignment: .leading, spacing: 2) {
                Text(room.hostName.isEmpty ? "未知主播" : room.hostName)
                    .font(.headline)
                Text(room.roomName.isEmpty ? "—" : room.roomName)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            Spacer()

            Button(room.listening ? "停止" : "监听", action: onToggle)
                .buttonStyle(.bordered)
                .controlSize(.small)
                .tint(room.listening ? .orange : .accentColor)
        }
        .swipeActions(edge: .trailing) {
            Button("删除", role: .destructive, action: onDelete)
        }
    }
}
