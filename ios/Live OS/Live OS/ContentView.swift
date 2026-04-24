//
//  ContentView.swift
//  Live OS
//
//  Created by xu on 2026/4/24.
//

import SwiftUI

struct ContentView: View {
    @Environment(AppConfig.self) private var appConfig

    var body: some View {
        TabView {
            Tab("视频库", systemImage: "play.rectangle.on.rectangle") {
                VideoLibraryView()
            }
            Tab("直播间", systemImage: "antenna.radiowaves.left.and.right") {
                RoomListView()
            }
            Tab("设置", systemImage: "gear") {
                SettingsView(isInitialSetup: false)
            }
        }
        .overlay(alignment: .top) {
            if appConfig.activeURL.isEmpty {
                unconfiguredBanner
            }
        }
    }

    private var unconfiguredBanner: some View {
        HStack(spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
            Text("尚未配置服务器，请前往"设置"填写地址")
                .font(.caption)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 8)
        .background(.thinMaterial, in: Capsule())
        .padding(.top, 8)
        .transition(.move(edge: .top).combined(with: .opacity))
        .animation(.spring(response: 0.4, dampingFraction: 0.8), value: appConfig.activeURL.isEmpty)
    }
}

#Preview {
    ContentView()
        .environment(AppConfig())
}
