import SwiftUI

// MARK: - Main Settings

struct SettingsView: View {
    @Environment(AppConfig.self) private var appConfig
    // isInitialSetup kept for API compatibility but no longer changes behavior
    let isInitialSetup: Bool

    @State private var showSaved = false
    @State private var serverVersion: String?
    @State private var isCheckingConnection = false
    @State private var connectionOK: Bool?

    var body: some View {
        NavigationStack {
            List {
                // MARK: 服务器
                serverSection

                // MARK: 认证
                authSection

                // MARK: 关于
                aboutSection
            }
            .navigationTitle("设置")
            .navigationBarTitleDisplayMode(.large)
            .overlay(alignment: .bottom) {
                if showSaved {
                    savedBadge
                }
            }
            .animation(.easeInOut(duration: 0.2), value: showSaved)
        }
    }

    // MARK: - Sections

    private var serverSection: some View {
        Section {
            NavigationLink(destination: NetworkConfigView()) {
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("网络配置")
                        Text(networkSummary)
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Image(systemName: "network")
                        .foregroundStyle(.blue)
                }
            }

            // Connection status row
            HStack {
                Label {
                    Text("连接状态")
                } icon: {
                    Image(systemName: connectionStatusIcon)
                        .foregroundStyle(connectionStatusColor)
                }
                Spacer()
                if isCheckingConnection {
                    ProgressView().controlSize(.small)
                } else {
                    Button("测试") {
                        testConnection()
                    }
                    .buttonStyle(.bordered)
                    .controlSize(.small)
                    if let ok = connectionOK {
                        Image(systemName: ok ? "checkmark.circle.fill" : "xmark.circle.fill")
                            .foregroundStyle(ok ? .green : .red)
                    }
                }
            }

            if let ver = serverVersion {
                Label {
                    Text(ver).foregroundStyle(.secondary)
                } icon: {
                    Image(systemName: "server.rack")
                        .foregroundStyle(.secondary)
                }
                .font(.caption)
            }
        } header: {
            Text("服务器")
        }
    }

    private var authSection: some View {
        Section {
            NavigationLink(destination: APIKeyView()) {
                Label {
                    VStack(alignment: .leading, spacing: 2) {
                        Text("API Key")
                        Text(appConfig.apiKey.isEmpty ? "未设置" : "已配置 (\(maskedKey))")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                } icon: {
                    Image(systemName: "key.fill")
                        .foregroundStyle(.orange)
                }
            }
        } header: {
            Text("认证")
        } footer: {
            Text("服务器启用 security.enable_api_key 后需要填写。")
        }
    }

    private var aboutSection: some View {
        Section {
            Label {
                HStack {
                    Text("版本")
                    Spacer()
                    Text(appVersion).foregroundStyle(.secondary)
                }
            } icon: {
                Image(systemName: "info.circle")
                    .foregroundStyle(.secondary)
            }

            Label {
                HStack {
                    Text("项目主页")
                    Spacer()
                    Image(systemName: "arrow.up.right.square")
                        .foregroundStyle(.secondary)
                        .font(.caption)
                }
            } icon: {
                Image(systemName: "globe")
                    .foregroundStyle(.secondary)
            }
            .onTapGesture {
                UIApplication.shared.open(URL(string: "https://github.com/bililive-go/bililive-go")!)
            }
        } header: {
            Text("关于")
        }
    }

    // MARK: - Helpers

    private var networkSummary: String {
        if appConfig.autoSwitchNetwork {
            let lan = appConfig.lanURL.isEmpty ? "未设置" : appConfig.lanURL
            return "智能切换 · \(appConfig.isOnLAN ? "局域网" : "公网")"
        }
        return appConfig.serverURL.isEmpty ? "未配置" : appConfig.serverURL
    }

    private var connectionStatusIcon: String {
        guard connectionOK != nil else { return "circle.dotted" }
        return connectionOK! ? "wifi" : "wifi.slash"
    }

    private var connectionStatusColor: Color {
        guard let ok = connectionOK else { return .secondary }
        return ok ? .green : .red
    }

    private var maskedKey: String {
        let k = appConfig.apiKey
        guard k.count > 4 else { return String(repeating: "•", count: k.count) }
        return String(repeating: "•", count: k.count - 4) + k.suffix(4)
    }

    private var appVersion: String {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1.0"
    }

    private func testConnection() {
        guard !appConfig.activeURL.isEmpty else {
            connectionOK = false
            return
        }
        isCheckingConnection = true
        connectionOK = nil
        serverVersion = nil
        Task {
            do {
                let info = try await appConfig.client.getServerInfo()
                await MainActor.run {
                    connectionOK = true
                    serverVersion = "\(info.appName) \(info.appVersion)"
                    isCheckingConnection = false
                }
            } catch {
                await MainActor.run {
                    connectionOK = false
                    isCheckingConnection = false
                }
            }
        }
    }

    private var savedBadge: some View {
        Text("已保存")
            .font(.subheadline)
            .padding(.horizontal, 16).padding(.vertical, 8)
            .background(.thinMaterial, in: Capsule())
            .padding(.bottom, 16)
            .transition(.move(edge: .bottom).combined(with: .opacity))
    }
}

// MARK: - Network Config Sub-page

struct NetworkConfigView: View {
    @Environment(AppConfig.self) private var appConfig
    @State private var mode: NetworkMode = .manual
    @State private var serverURL = ""
    @State private var lanURL = ""
    @State private var publicURL = ""
    @State private var showSaved = false

    enum NetworkMode: String, CaseIterable {
        case manual = "手动"
        case smart = "智能切换"
    }

    var body: some View {
        Form {
            // Mode picker
            Section {
                Picker("模式", selection: $mode) {
                    ForEach(NetworkMode.allCases, id: \.self) { m in
                        Text(m.rawValue).tag(m)
                    }
                }
                .pickerStyle(.segmented)
            } footer: {
                Text(mode == .manual
                    ? "直接填写服务器地址。"
                    : "App 自动检测网络，在局域网时用内网地址，在外出时用公网地址，速度更快且更省流量。")
            }

            if mode == .manual {
                manualSection
            } else {
                smartSection
            }

            Section {
                Button(action: save) {
                    HStack {
                        Spacer()
                        Text("保存").bold()
                        Spacer()
                    }
                }
            }
        }
        .navigationTitle("网络配置")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { loadValues() }
        .overlay(alignment: .bottom) {
            if showSaved {
                Text("已保存")
                    .font(.subheadline)
                    .padding(.horizontal, 16).padding(.vertical, 8)
                    .background(.thinMaterial, in: Capsule())
                    .padding(.bottom, 16)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut(duration: 0.2), value: showSaved)
        .animation(.easeInOut(duration: 0.2), value: mode)
    }

    private var manualSection: some View {
        Section {
            TextField("http://192.168.1.x:8080", text: $serverURL)
                .keyboardType(.URL)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
        } header: {
            Text("服务器地址")
        } footer: {
            Text("运行 bililive-go 的设备 IP 和端口。")
        }
    }

    private var smartSection: some View {
        Group {
            Section {
                TextField("http://192.168.1.x:8080", text: $lanURL)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
            } header: {
                HStack {
                    Image(systemName: "wifi").foregroundStyle(.green)
                    Text("局域网地址")
                }
            } footer: {
                Text("在家或店里，通过局域网直接访问，速度最快。")
            }

            Section {
                TextField("http://your-domain.com:8080", text: $publicURL)
                    .keyboardType(.URL)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
            } header: {
                HStack {
                    Image(systemName: "globe").foregroundStyle(.blue)
                    Text("公网地址")
                }
            } footer: {
                Text("在外出时通过公网访问。需要在路由器设置端口转发，或使用 frp/cloudflare 等内网穿透。")
            }

            if appConfig.autoSwitchNetwork {
                Section {
                    HStack(spacing: 12) {
                        Image(systemName: appConfig.isOnLAN ? "wifi" : "globe")
                            .foregroundStyle(appConfig.isOnLAN ? .green : .blue)
                            .frame(width: 24)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(appConfig.isOnLAN ? "当前：局域网" : "当前：公网")
                                .font(.subheadline)
                            Text(appConfig.activeURL.isEmpty ? "未配置" : appConfig.activeURL)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .padding(.vertical, 2)
                } header: {
                    Text("当前连接")
                }
            }
        }
    }

    private func loadValues() {
        mode = appConfig.autoSwitchNetwork ? .smart : .manual
        serverURL = appConfig.serverURL
        lanURL = appConfig.lanURL
        publicURL = appConfig.publicURL
    }

    private func save() {
        let isSmart = mode == .smart
        appConfig.applySettings(
            serverURL: isSmart ? "" : serverURL.trimmingCharacters(in: .whitespaces),
            apiKey: appConfig.apiKey
        )
        appConfig.applyNetworkSettings(
            lanURL: lanURL.trimmingCharacters(in: .whitespaces),
            publicURL: publicURL.trimmingCharacters(in: .whitespaces),
            autoSwitch: isSmart
        )
        showSaved = true
        Task {
            try? await Task.sleep(for: .seconds(1.5))
            await MainActor.run { showSaved = false }
        }
    }
}

// MARK: - API Key Sub-page

struct APIKeyView: View {
    @Environment(AppConfig.self) private var appConfig
    @State private var apiKey = ""
    @State private var showKey = false
    @State private var showSaved = false

    var body: some View {
        Form {
            Section {
                HStack {
                    Group {
                        if showKey {
                            TextField("留空表示不启用鉴权", text: $apiKey)
                        } else {
                            SecureField("留空表示不启用鉴权", text: $apiKey)
                        }
                    }
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)

                    Button {
                        showKey.toggle()
                    } label: {
                        Image(systemName: showKey ? "eye.slash" : "eye")
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            } header: {
                Text("API Key")
            } footer: {
                Text("在服务器 config.yml 中设置 `security.enable_api_key: true` 并填写相同的 key。留空则不启用鉴权。")
            }

            if !apiKey.isEmpty {
                Section {
                    Button("清除", role: .destructive) {
                        apiKey = ""
                    }
                }
            }

            Section {
                Button(action: save) {
                    HStack {
                        Spacer()
                        Text("保存").bold()
                        Spacer()
                    }
                }
            }
        }
        .navigationTitle("API Key")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { apiKey = appConfig.apiKey }
        .overlay(alignment: .bottom) {
            if showSaved {
                Text("已保存")
                    .font(.subheadline)
                    .padding(.horizontal, 16).padding(.vertical, 8)
                    .background(.thinMaterial, in: Capsule())
                    .padding(.bottom, 16)
                    .transition(.move(edge: .bottom).combined(with: .opacity))
            }
        }
        .animation(.easeInOut(duration: 0.2), value: showSaved)
    }

    private func save() {
        appConfig.applySettings(
            serverURL: appConfig.serverURL,
            apiKey: apiKey.trimmingCharacters(in: .whitespaces)
        )
        showSaved = true
        Task {
            try? await Task.sleep(for: .seconds(1.5))
            await MainActor.run { showSaved = false }
        }
    }
}
