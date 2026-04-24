import SwiftUI

struct SettingsView: View {
    @Environment(AppConfig.self) private var appConfig
    let isInitialSetup: Bool

    @State private var serverURL = ""
    @State private var apiKey = ""
    @State private var lanURL = ""
    @State private var publicURL = ""
    @State private var autoSwitch = false
    @State private var showSaved = false

    var body: some View {
        NavigationStack {
            Form {
                if isInitialSetup || !autoSwitch {
                    Section {
                        TextField("http://192.168.1.x:8080", text: $serverURL)
                            .keyboardType(.URL)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                    } header: {
                        Text("服务器地址")
                    } footer: {
                        Text("填写运行 bililive-go 的设备 IP 和端口。")
                    }
                }

                if autoSwitch {
                    Section {
                        TextField("http://192.168.1.x:8080", text: $lanURL)
                            .keyboardType(.URL)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                    } header: {
                        Text("局域网地址")
                    } footer: {
                        Text("在店里通过局域网访问的地址，速度更快。")
                    }

                    Section {
                        TextField("http://your-domain.com:8080", text: $publicURL)
                            .keyboardType(.URL)
                            .autocorrectionDisabled()
                            .textInputAutocapitalization(.never)
                    } header: {
                        Text("公网地址")
                    } footer: {
                        Text("在外面通过公网访问的地址。需要在路由器设置端口转发或使用域名。")
                    }

                    Section {
                        HStack {
                            Image(systemName: appConfig.isOnLAN ? "wifi" : "globe")
                                .foregroundStyle(appConfig.isOnLAN ? .green : .orange)
                            Text(appConfig.isOnLAN ? "当前: 局域网连接" : "当前: 公网连接")
                                .font(.subheadline)
                        }
                    } header: {
                        Text("网络状态")
                    }
                }

                Section {
                    Toggle("智能网络切换", isOn: $autoSwitch)
                } header: {
                    Text("自动切换")
                } footer: {
                    Text("开启后，app 会自动检测是否在局域网内。在店里时自动使用局域网地址获取更快的速度，在外面时自动切换到公网地址。")
                }

                Section {
                    SecureField("留空表示不启用鉴权", text: $apiKey)
                        .autocorrectionDisabled()
                        .textInputAutocapitalization(.never)
                } header: {
                    Text("API Key（可选）")
                } footer: {
                    Text("在服务器 config.yml 中设置 security.enable_api_key: true 后填写。")
                }

                Section {
                    Button(action: save) {
                        HStack {
                            Spacer()
                            Text(isInitialSetup ? "开始使用" : "保存")
                                .bold()
                            Spacer()
                        }
                    }
                    .disabled(!isFormValid)
                }
            }
            .navigationTitle(isInitialSetup ? "连接服务器" : "设置")
            .onAppear {
                serverURL = appConfig.serverURL
                apiKey = appConfig.apiKey
                lanURL = appConfig.lanURL
                publicURL = appConfig.publicURL
                autoSwitch = appConfig.autoSwitchNetwork
            }
            .overlay(alignment: .bottom) {
                if showSaved {
                    Text("已保存").padding(8)
                        .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 8))
                        .padding(.bottom, 12)
                        .transition(.opacity)
                }
            }
            .animation(.easeInOut, value: showSaved)
        }
    }

    private var isFormValid: Bool {
        if autoSwitch {
            return !lanURL.trimmingCharacters(in: .whitespaces).isEmpty
                || !publicURL.trimmingCharacters(in: .whitespaces).isEmpty
        }
        return !serverURL.trimmingCharacters(in: .whitespaces).isEmpty
    }

    private func save() {
        appConfig.applySettings(
            serverURL: autoSwitch ? "" : serverURL.trimmingCharacters(in: .whitespaces),
            apiKey: apiKey.trimmingCharacters(in: .whitespaces)
        )
        appConfig.applyNetworkSettings(
            lanURL: lanURL.trimmingCharacters(in: .whitespaces),
            publicURL: publicURL.trimmingCharacters(in: .whitespaces),
            autoSwitch: autoSwitch
        )
        if !isInitialSetup {
            showSaved = true
            Task {
                try? await Task.sleep(for: .seconds(1.5))
                showSaved = false
            }
        }
    }
}
