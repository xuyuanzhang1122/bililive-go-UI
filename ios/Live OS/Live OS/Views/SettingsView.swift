import SwiftUI

struct SettingsView: View {
    @Environment(AppConfig.self) private var appConfig
    let isInitialSetup: Bool

    @State private var serverURL = ""
    @State private var apiKey = ""
    @State private var showSaved = false

    var body: some View {
        NavigationStack {
            Form {
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
                    .disabled(serverURL.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
            .navigationTitle(isInitialSetup ? "连接服务器" : "设置")
            .onAppear {
                serverURL = appConfig.serverURL
                apiKey    = appConfig.apiKey
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

    private func save() {
        appConfig.applySettings(
            serverURL: serverURL.trimmingCharacters(in: .whitespaces),
            apiKey: apiKey.trimmingCharacters(in: .whitespaces)
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
