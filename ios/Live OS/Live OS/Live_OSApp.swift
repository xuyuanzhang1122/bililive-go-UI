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

    init() {
        let cache = URLCache(memoryCapacity: 50_000_000, diskCapacity: 200_000_000)
        URLCache.shared = cache
    }

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environment(appConfig)
        }
    }
}
