//
//  Live_OSApp.swift
//  Live OS
//
//  Created by xu on 2026/4/24.
//

import SwiftUI

@main
struct Live_OSApp: App {
    @State private var appConfig = AppConfig()

    var body: some Scene {
        WindowGroup {
            if appConfig.serverURL.isEmpty {
                SettingsView(isInitialSetup: true)
                    .environment(appConfig)
            } else {
                ContentView()
                    .environment(appConfig)
            }
        }
    }
}
