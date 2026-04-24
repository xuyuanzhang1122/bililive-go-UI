//
//  ContentView.swift
//  Live OS
//
//  Created by xu on 2026/4/24.
//

import SwiftUI

struct ContentView: View {
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
    }
}

#Preview {
    ContentView()
        .environment(AppConfig())
}
