import AVFoundation
import AVFAudio
import Combine
import MediaPlayer
import SwiftUI
import UIKit
import AVKit

// MARK: - Speed Presets

private let speedPresets: [Float] = [0.78, 1.0, 1.25, 1.5, 2.0]

// MARK: - PlayerView

struct PlayerView: View {
    let file: VideoFileInfo
    let client: APIClient

    @Environment(\.dismiss) private var dismiss

    @State private var player: AVPlayer?
    @State private var timeObserver: Any?
    @State private var duration: Double = 0
    @State private var currentTime: Double = 0
    @State private var isPlaying = false
    @State private var showControls = true
    @State private var isImmersive = true
    @State private var errorMessage: String?

    // Speed
    @State private var currentSpeed: Float = 1.0

    // Swipe seek (accumulated per-gesture)
    @State private var seekBaseTime: Double = 0
    @State private var seekDelta: Double = 0
    @State private var showSeekIndicator = false

    // Volume / brightness swipe
    @State private var showVolumeIndicator = false
    @State private var currentSystemVolume: Float = 0
    @State private var showBrightnessIndicator = false
    @State private var currentBrightness: CGFloat = 0

    // Long-press speed boost
    @State private var speedBoosting = false

    // Controls auto-hide
    @State private var controlsHideTask: Task<Void, Never>?
    private let togglePiPSubject = PassthroughSubject<Void, Never>()

    // Progress bar scrub state
    @State private var isScrubbing = false

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            if let player {
                PlayerSurface(player: player, togglePiP: togglePiPSubject)
                    .ignoresSafeArea()

                // Gesture layer — pure SwiftUI, no UIKit touch interception
                Color.clear
                    .contentShape(Rectangle())
                    .gesture(TapGesture(count: 2).onEnded {
                        togglePlay()
                        revealControlsTemporarily()
                    })
                    .gesture(TapGesture(count: 1).onEnded {
                        toggleControls()
                    })
                    .gesture(tapAndSwipeGesture)
                    .gesture(longPressBoostGesture)

                // Seek indicator
                if showSeekIndicator {
                    SeekDeltaIndicator(delta: seekDelta, targetTime: currentTime, valueFormatter: formatTime)
                        .transition(.opacity)
                        .allowsHitTesting(false)
                }

                // Volume indicator
                if showVolumeIndicator {
                    VolumeIndicator(volume: currentSystemVolume)
                        .transition(.opacity)
                        .allowsHitTesting(false)
                }

                // Brightness indicator
                if showBrightnessIndicator {
                    BrightnessIndicator(brightness: currentBrightness)
                        .transition(.opacity)
                        .allowsHitTesting(false)
                }

                if showControls {
                    controlsLayer
                        .transition(.opacity.combined(with: .move(edge: .bottom)))
                }
            } else if let errorMessage {
                ContentUnavailableView("无法播放", systemImage: "exclamationmark.triangle", description: Text(errorMessage))
                    .foregroundStyle(.white)
                    .padding()
            } else {
                VStack(spacing: 14) {
                    ProgressView()
                        .tint(.white)
                    Text("准备播放…")
                        .font(.subheadline)
                        .foregroundStyle(.white.opacity(0.72))
                }
            }
        }
        .preferredColorScheme(.dark)
        .statusBarHidden(isImmersive)
        .task {
            currentSystemVolume = AVAudioSession.sharedInstance().outputVolume
            currentBrightness = UIScreen.main.brightness
            await preparePlayer()
        }
        .onDisappear { cleanupPlayer() }
        .animation(.easeInOut(duration: 0.22), value: showControls)
        .animation(.easeInOut(duration: 0.15), value: showSeekIndicator)
        .animation(.easeInOut(duration: 0.15), value: showVolumeIndicator)
        .animation(.easeInOut(duration: 0.15), value: showBrightnessIndicator)
    }

    // MARK: - SwiftUI Gestures

    /// Tap + swipe gesture. Tap for play/pause/controls, swipe for seek/volume/brightness.
    private var tapAndSwipeGesture: some Gesture {
        DragGesture(minimumDistance: 12)
            .onChanged { value in
                controlsHideTask?.cancel()
                let absX = abs(value.translation.width)
                let absY = abs(value.translation.height)

                if absX > absY {
                    // Horizontal → seek: compute delta from accumulated translation
                    if !showSeekIndicator { seekBaseTime = currentTime }
                    let span = horizontalSeekSpan()
                    let frac = Double(value.translation.width / max(UIScreen.main.bounds.width, 1))
                    let target = clampedTime(seekBaseTime + frac * span)
                    currentTime = target
                    seekDelta = target - seekBaseTime
                    showSeekIndicator = true
                    withAnimation(.easeInOut(duration: 0.18)) { showControls = true }
                } else {
                    // Vertical → volume (right side) / brightness (left side)
                    let locationX = value.startLocation.x
                    let viewWidth = UIScreen.main.bounds.width
                    let step = value.translation.height
                    if locationX > viewWidth / 2 {
                        let volumeChange = Float(-step) * 0.004
                        let newVol = min(max(currentSystemVolume + volumeChange, 0), 1)
                        setSystemVolume(newVol)
                        currentSystemVolume = newVol
                        showVolumeIndicator = true
                        hideVolumeIndicatorSoon()
                    } else {
                        let brightnessChange = CGFloat(-step) * 0.004
                        let newBright = min(max(currentBrightness + brightnessChange, 0), 1)
                        UIScreen.main.brightness = newBright
                        currentBrightness = newBright
                        showBrightnessIndicator = true
                        hideBrightnessIndicatorSoon()
                    }
                }
            }
            .onEnded { value in
                let absX = abs(value.translation.width)
                let absY = abs(value.translation.height)

                if absX > absY {
                    // Commit seek
                    let span = horizontalSeekSpan()
                    let frac = Double(value.translation.width / max(UIScreen.main.bounds.width, 1))
                    let target = clampedTime(seekBaseTime + frac * span)
                    seek(to: target)
                    hideSeekIndicatorSoon()
                } else {
                    scheduleControlsAutoHide()
                }

                // If very short drag (< 20px, < 0.3s) → treat as tap
                let dragDist = hypot(value.translation.width, value.translation.height)
                if dragDist < 20 {
                    toggleControls()
                }
            }
    }

    /// Long press → temporary 2x speed boost while held.
    private var longPressBoostGesture: some Gesture {
        LongPressGesture(minimumDuration: 0.35)
            .sequenced(before: DragGesture(minimumDistance: 0))
            .onChanged { value in
                switch value {
                case .second(true, _):
                    // Long press active
                    if !speedBoosting {
                        speedBoosting = true
                        guard let player else { return }
                        player.playImmediately(atRate: 2)
                        UIImpactFeedbackGenerator(style: .medium).impactOccurred()
                    }
                case .first, .second(false, _):
                    break
                }
            }
            .onEnded { _ in
                if speedBoosting {
                    speedBoosting = false
                    guard let player else { return }
                    if isPlaying {
                        player.rate = currentSpeed
                    } else {
                        player.pause()
                    }
                }
            }
    }

    // MARK: - Controls Layer

    private var controlsLayer: some View {
        VStack(spacing: 0) {
            topBar
            Spacer(minLength: 0)
            bottomControls
        }
        .ignoresSafeArea(edges: isImmersive ? .all : [])
    }

    private var topBar: some View {
        HStack(spacing: 12) {
            Button {
                dismiss()
            } label: {
                Image(systemName: "xmark")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(.white)
                    .frame(width: 36, height: 36)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .buttonStyle(.plain)

            VStack(alignment: .leading, spacing: 2) {
                Text(file.name)
                    .font(.subheadline.weight(.medium))
                    .lineLimit(1)
                    .foregroundStyle(.white)
                Text(file.sizeFormatted)
                    .font(.caption2)
                    .foregroundStyle(.white.opacity(0.55))
            }

            Spacer()

            Button {
                togglePiPSubject.send()
                scheduleControlsAutoHide()
            } label: {
                Image(systemName: "pip.enter")
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(.white)
                    .frame(width: 36, height: 36)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .buttonStyle(.plain)
        }
        .padding(.horizontal, 18)
        .padding(.top, isImmersive ? 56 : 18)
        .padding(.bottom, 48)
        .background(
            LinearGradient(colors: [.black.opacity(0.7), .black.opacity(0)], startPoint: .top, endPoint: .bottom)
                .allowsHitTesting(false)
        )
    }

    private var bottomControls: some View {
        VStack(spacing: 20) {
            // Progress bar
            progressBar

            // Time labels + buttons
            HStack(spacing: 14) {
                Text(formatTime(currentTime))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.7))
                    .frame(width: 46, alignment: .leading)

                Spacer()

                // Skip back 10s
                ControlPillButton(
                    icon: "gobackward.10",
                    size: 18,
                    action: { seek(by: -10); UIImpactFeedbackGenerator(style: .light).impactOccurred() }
                )

                // Play/pause
                ControlPillButton(
                    icon: isPlaying ? "pause.fill" : "play.fill",
                    size: 22,
                    isProminent: true,
                    action: { togglePlay(); UIImpactFeedbackGenerator(style: .medium).impactOccurred() }
                )

                // Skip forward 10s
                ControlPillButton(
                    icon: "goforward.10",
                    size: 18,
                    action: { seek(by: 10); UIImpactFeedbackGenerator(style: .light).impactOccurred() }
                )

                Spacer()

                Text(formatTime(duration))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.55))
                    .frame(width: 46, alignment: .trailing)
            }

            // Speed + hints
            HStack {
                Menu {
                    ForEach(speedPresets, id: \.self) { speed in
                        Button {
                            setSpeed(speed)
                        } label: {
                            HStack {
                                Text(String(format: "%.2gx", speed))
                                if currentSpeed == speed {
                                    Image(systemName: "checkmark")
                                }
                            }
                        }
                    }
                } label: {
                    HStack(spacing: 5) {
                        Image(systemName: "speedometer")
                            .font(.system(size: 11))
                        Text(String(format: "%.2gx", currentSpeed))
                            .monospacedDigit()
                            .font(.caption2.weight(.medium))
                    }
                    .foregroundStyle(.white.opacity(0.65))
                    .padding(.horizontal, 12)
                    .padding(.vertical, 7)
                    .background(.ultraThinMaterial, in: Capsule())
                }

                Spacer()

                Text("滑动调整进度 · 左侧亮度 · 右侧音量")
                    .font(.caption2)
                    .foregroundStyle(.white.opacity(0.35))
            }
        }
        .padding(.horizontal, 22)
        .padding(.top, 48)
        .padding(.bottom, isImmersive ? 36 : 18)
        .background(
            LinearGradient(colors: [.black.opacity(0), .black.opacity(0.78)], startPoint: .top, endPoint: .bottom)
                .allowsHitTesting(false)
        )
    }

    // MARK: - Progress Bar

    private var progressBar: some View {
        let clampedDuration = max(duration, 1)
        let progress = clampedDuration > 0 ? min(max(currentTime / clampedDuration, 0), 1) : 0

        return GeometryReader { geometry in
            let width = geometry.size.width
            let usableWidth = max(width, 1)
            let thumbX = usableWidth * progress
            let trackHeight: CGFloat = isScrubbing ? 8 : 4

            ZStack(alignment: .leading) {
                // Background track
                Capsule()
                    .fill(.white.opacity(0.15))
                    .frame(height: trackHeight)

                // Active track with gradient
                Capsule()
                    .fill(
                        LinearGradient(
                            colors: [
                                Color(red: 1, green: 0.18, blue: 0.22),
                                Color(red: 1, green: 0.45, blue: 0.18),
                                Color(red: 1, green: 0.73, blue: 0.22),
                            ],
                            startPoint: .leading,
                            endPoint: .trailing
                        )
                    )
                    .frame(width: max(0, thumbX), height: trackHeight)

                // Thumb — only visible when scrubbing
                if isScrubbing {
                    Circle()
                        .fill(.white)
                        .frame(width: 22, height: 22)
                        .overlay(Circle().stroke(.black.opacity(0.12), lineWidth: 0.5))
                        .shadow(color: .black.opacity(0.3), radius: 6, y: 2)
                        .position(x: thumbX, y: geometry.size.height / 2)
                }
            }
            .frame(maxHeight: .infinity)
            .contentShape(Rectangle().inset(by: -20)) // expand hit area
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { gesture in
                        if !isScrubbing {
                            isScrubbing = true
                            controlsHideTask?.cancel()
                            withAnimation(.easeInOut(duration: 0.18)) {
                                showControls = true
                            }
                        }
                        let p = min(max(gesture.location.x / usableWidth, 0), 1)
                        currentTime = clampedDuration * p
                    }
                    .onEnded { gesture in
                        let p = min(max(gesture.location.x / usableWidth, 0), 1)
                        let target = clampedDuration * p
                        currentTime = target
                        seek(to: target)
                        isScrubbing = false
                        UIImpactFeedbackGenerator(style: .light).impactOccurred()
                        scheduleControlsAutoHide()
                    }
            )
            .animation(.spring(response: 0.24, dampingFraction: 0.78), value: isScrubbing)
            .animation(.spring(response: 0.24, dampingFraction: 0.78), value: trackHeight)
        }
        .frame(height: 40)
    }

    // MARK: - Player Logic

    private func preparePlayer() async {
        guard player == nil else { return }
        guard let url = client.playbackURL(for: file) else {
            errorMessage = "无法获取播放地址"
            return
        }

        try? AVAudioSession.sharedInstance().setCategory(.playback, mode: .moviePlayback, options: [])
        try? AVAudioSession.sharedInstance().setActive(true)

        let item = AVPlayerItem(url: url)
        let freshPlayer = AVPlayer(playerItem: item)
        player = freshPlayer
        installTimeObserver(for: freshPlayer)
        freshPlayer.play()
        isPlaying = true
        scheduleControlsAutoHide()
    }

    private func installTimeObserver(for player: AVPlayer) {
        if let timeObserver {
            self.player?.removeTimeObserver(timeObserver)
            self.timeObserver = nil
        }
        timeObserver = player.addPeriodicTimeObserver(forInterval: CMTime(seconds: 0.25, preferredTimescale: 600), queue: .main) { time in
            if !isScrubbing, time.seconds.isFinite {
                currentTime = time.seconds
            }
            if let seconds = player.currentItem?.duration.seconds, seconds.isFinite {
                duration = seconds
            }
            isPlaying = player.timeControlStatus == .playing
        }
    }

    private func cleanupPlayer() {
        controlsHideTask?.cancel()
        controlsHideTask = nil
        if let timeObserver {
            player?.removeTimeObserver(timeObserver)
            self.timeObserver = nil
        }
        player?.pause()
        player = nil
    }

    private func togglePlay() {
        guard let player else { return }
        if player.timeControlStatus == .playing {
            player.pause()
            isPlaying = false
            controlsHideTask?.cancel()
        } else {
            player.play()
            if currentSpeed != 1.0 { player.rate = currentSpeed }
            isPlaying = true
            scheduleControlsAutoHide()
        }
    }

    private func revealControlsTemporarily() {
        withAnimation(.easeInOut(duration: 0.18)) {
            showControls = true
        }
        scheduleControlsAutoHide()
    }

    private func scheduleControlsAutoHide() {
        controlsHideTask?.cancel()
        guard isPlaying else { return }
        controlsHideTask = Task {
            try? await Task.sleep(for: .seconds(3))
            if !Task.isCancelled {
                await MainActor.run {
                    withAnimation(.easeInOut(duration: 0.3)) {
                        showControls = false
                    }
                }
            }
        }
    }

    private func seek(by offset: Double) {
        seek(to: clampedTime(currentTime + offset))
    }

    private func seek(to seconds: Double) {
        guard let player else { return }
        let clamped = clampedTime(seconds)
        currentTime = clamped
        let target = CMTime(seconds: clamped, preferredTimescale: 600)
        player.seek(to: target, toleranceBefore: .zero, toleranceAfter: .zero)
        scheduleControlsAutoHide()
    }

    private func clampedTime(_ seconds: Double) -> Double {
        min(max(seconds, 0), max(duration, 0))
    }

    private func horizontalSeekSpan() -> Double {
        guard duration > 0 else { return 60 }
        return min(max(duration * 0.04, 45), 180)
    }

    private func setSpeed(_ speed: Float) {
        currentSpeed = speed
        guard let player else { return }
        if player.timeControlStatus == .playing {
            player.rate = speed
        }
    }

    private func setSystemVolume(_ volume: Float) {
        Task { @MainActor in
            SystemVolumeController.shared.setVolume(volume)
        }
    }

    private func hideSeekIndicatorSoon() {
        Task {
            try? await Task.sleep(for: .milliseconds(700))
            await MainActor.run { showSeekIndicator = false }
        }
    }

    private func hideVolumeIndicatorSoon() {
        Task {
            try? await Task.sleep(for: .milliseconds(650))
            await MainActor.run {
                showVolumeIndicator = false
                scheduleControlsAutoHide()
            }
        }
    }

    private func hideBrightnessIndicatorSoon() {
        Task {
            try? await Task.sleep(for: .milliseconds(650))
            await MainActor.run {
                showBrightnessIndicator = false
                scheduleControlsAutoHide()
            }
        }
    }

    private func formatTime(_ seconds: Double) -> String {
        guard seconds.isFinite && seconds >= 0 else { return "00:00" }
        let total = Int(seconds.rounded())
        let hours = total / 3600
        let minutes = (total % 3600) / 60
        let secs = total % 60
        if hours > 0 {
            return String(format: "%d:%02d:%02d", hours, minutes, secs)
        }
        return String(format: "%02d:%02d", minutes, secs)
    }
}

// MARK: - Control Pill Button

private struct ControlPillButton: View {
    let icon: String
    var size: CGFloat = 18
    var isProminent: Bool = false
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: icon)
                .font(.system(size: size, weight: isProminent ? .semibold : .medium))
                .foregroundStyle(isProminent ? .black : .white)
                .frame(
                    width: isProminent ? 52 : 40,
                    height: isProminent ? 52 : 40
                )
                .background(
                    isProminent
                        ? AnyShapeStyle(.white)
                        : AnyShapeStyle(.ultraThinMaterial)
                )
                .clipShape(Circle())
                .overlay(
                    Circle()
                        .stroke(.white.opacity(isProminent ? 0 : 0.12), lineWidth: 0.5)
                )
        }
        .buttonStyle(.plain)
        .scaleButtonOnPress()
    }
}

// MARK: - Scale Button Modifier

private struct ScaleButtonStyle: ViewModifier {
    @State private var pressed = false

    func body(content: Content) -> some View {
        content
            .scaleEffect(pressed ? 0.92 : 1)
            .animation(.spring(response: 0.22, dampingFraction: 0.7), value: pressed)
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { _ in pressed = true }
                    .onEnded { _ in pressed = false }
            )
    }
}

extension View {
    func scaleButtonOnPress() -> some View {
        modifier(ScaleButtonStyle())
    }
}

// MARK: - Player Surface

private struct PlayerSurface: UIViewRepresentable {
    let player: AVPlayer
    let togglePiP: PassthroughSubject<Void, Never>

    func makeUIView(context: Context) -> PlayerSurfaceView {
        let view = PlayerSurfaceView()
        view.playerLayer.player = player
        view.playerLayer.videoGravity = .resizeAspect
        view.setupPiP(togglePiP: togglePiP)
        return view
    }

    func updateUIView(_ uiView: PlayerSurfaceView, context: Context) {
        uiView.playerLayer.player = player
    }
}

private final class PlayerSurfaceView: UIView {
    override class var layerClass: AnyClass { AVPlayerLayer.self }
    var playerLayer: AVPlayerLayer { layer as! AVPlayerLayer }

    private var pipController: AVPictureInPictureController?
    private var pipCancellable: AnyCancellable?

    func setupPiP(togglePiP: PassthroughSubject<Void, Never>) {
        guard AVPictureInPictureController.isPictureInPictureSupported() else { return }
        pipController = AVPictureInPictureController(playerLayer: playerLayer)
        pipController?.canStartPictureInPictureAutomaticallyFromInline = true
        pipCancellable = togglePiP.sink { [weak self] in
            guard let pip = self?.pipController else { return }
            if pip.isPictureInPictureActive {
                pip.stopPictureInPicture()
            } else {
                pip.startPictureInPicture()
            }
        }
    }

    deinit {
        pipCancellable?.cancel()
    }
}

// MARK: - Seek Delta Indicator

private struct SeekDeltaIndicator: View {
    let delta: Double
    let targetTime: Double
    let valueFormatter: (Double) -> String

    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: delta > 0 ? "goforward" : "gobackward")
                .font(.system(size: 25, weight: .semibold))
            Text("\(delta > 0 ? "+" : "")\(Int(delta.rounded()))s")
                .font(.title3.monospacedDigit().bold())
            Text(valueFormatter(targetTime))
                .font(.caption.monospacedDigit().weight(.medium))
                .foregroundStyle(.white.opacity(0.72))
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 22)
        .padding(.vertical, 16)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 16))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(.white.opacity(0.12), lineWidth: 1))
        .shadow(color: .black.opacity(0.3), radius: 12)
        .allowsHitTesting(false)
    }
}

// MARK: - Volume Indicator

private struct VolumeIndicator: View {
    let volume: Float

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: volume <= 0 ? "speaker.slash.fill" : volume < 0.33 ? "speaker.wave.1.fill" : volume < 0.66 ? "speaker.wave.2.fill" : "speaker.wave.3.fill")
                .font(.system(size: 16))
                .frame(width: 24)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule().fill(.white.opacity(0.2))
                    Capsule().fill(.white).frame(width: geo.size.width * CGFloat(volume))
                }
            }
            .frame(width: 120, height: 4)
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.3), radius: 12)
        .allowsHitTesting(false)
    }
}

// MARK: - Brightness Indicator

private struct BrightnessIndicator: View {
    let brightness: CGFloat

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: "sun.max.fill")
                .font(.system(size: 16))
                .frame(width: 24)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule().fill(.white.opacity(0.2))
                    Capsule().fill(.white).frame(width: geo.size.width * brightness)
                }
            }
            .frame(width: 120, height: 4)
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.3), radius: 12)
        .allowsHitTesting(false)
    }
}

// MARK: - System Volume

@MainActor
private final class SystemVolumeController {
    static let shared = SystemVolumeController()

    private let volumeView = MPVolumeView(frame: CGRect(x: -1000, y: -1000, width: 1, height: 1))
    private weak var slider: UISlider?
    private var isInstalled = false

    private init() {
        volumeView.alpha = 0.01
        volumeView.isUserInteractionEnabled = false
    }

    func setVolume(_ volume: Float) {
        let normalizedVolume = min(max(volume, 0), 1)
        installIfNeeded()
        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(10))
            let slider = self.slider ?? self.findSlider()
            slider?.setValue(normalizedVolume, animated: false)
            slider?.sendActions(for: .valueChanged)
        }
    }

    private func installIfNeeded() {
        guard !isInstalled else { return }
        guard let windowScene = UIApplication.shared.connectedScenes
            .compactMap({ $0 as? UIWindowScene })
            .first(where: { $0.activationState == .foregroundActive }),
              let window = windowScene.windows.first(where: { $0.isKeyWindow }) ?? windowScene.windows.first
        else { return }

        window.addSubview(volumeView)
        slider = findSlider()
        isInstalled = true
    }

    private func findSlider() -> UISlider? {
        if let slider = volumeView.subviews.first(where: { $0 is UISlider }) as? UISlider {
            self.slider = slider
            return slider
        }
        return nil
    }
}
