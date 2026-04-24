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
    @State private var isSeeking = false
    @State private var showControls = true
    @State private var isImmersive = true
    @State private var errorMessage: String?

    // Speed
    @State private var currentSpeed: Float = 1.0
    @State private var showSpeedMenu = false

    // Long-press speed boost
    @State private var speedBoostSide: PlayerSide?
    @State private var boostRestoreRate: Float = 1
    @State private var boostWasPlaying = false

    // Swipe feedback
    @State private var seekDelta: Double = 0
    @State private var showSeekIndicator = false
    @State private var volumeDelta: Float = 0
    @State private var showVolumeIndicator = false
    @State private var currentSystemVolume: Float = 0
    
    @State private var showBrightnessIndicator = false
    @State private var currentBrightness: CGFloat = 0

    @State private var controlsHideTask: Task<Void, Never>?
    private let togglePiPSubject = PassthroughSubject<Void, Never>()

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            if let player {
                PlayerSurface(player: player, togglePiP: togglePiPSubject)
                    .ignoresSafeArea()

                GestureHandlerView(
                    onSingleTap: { toggleControls() },
                    onDoubleTap: { location in
                        togglePlay()
                        revealControlsTemporarily()
                    },
                    onSwipeHorizontal: { delta in
                        let seconds = delta * 0.15 // sensitivity
                        seek(by: seconds)
                        seekDelta = seconds
                        showSeekIndicator = true
                        Task {
                            try? await Task.sleep(for: .milliseconds(600))
                            showSeekIndicator = false
                        }
                    },
                    onSwipeVertical: { location, delta in
                        let viewWidth = UIScreen.main.bounds.width
                        if location.x > viewWidth / 2 {
                            // delta: negative = swipe up (louder), positive = swipe down (quieter)
                            let volumeChange = Float(-delta) * 0.008
                            let newVol = min(max(currentSystemVolume + volumeChange, 0), 1)
                            setSystemVolume(newVol)
                            currentSystemVolume = newVol
                            volumeDelta = volumeChange
                            showVolumeIndicator = true
                            Task {
                                try? await Task.sleep(for: .milliseconds(600))
                                showVolumeIndicator = false
                            }
                        } else {
                            let brightnessChange = CGFloat(-delta) * 0.008
                            let newBright = min(max(currentBrightness + brightnessChange, 0), 1)
                            UIScreen.main.brightness = newBright
                            currentBrightness = newBright
                            showBrightnessIndicator = true
                            Task {
                                try? await Task.sleep(for: .milliseconds(600))
                                showBrightnessIndicator = false
                            }
                        }
                    },
                    onLongPressStart: { location, viewWidth in
                        let side: PlayerSide = location.x < viewWidth / 2 ? .left : .right
                        startSpeedBoost(side)
                    },
                    onLongPressEnd: {
                        stopSpeedBoost()
                    }
                )
                .ignoresSafeArea()

                // Seek delta indicator
                if showSeekIndicator {
                    SeekDeltaIndicator(delta: seekDelta)
                        .transition(.opacity)
                }

                // Volume indicator
                if showVolumeIndicator {
                    VolumeIndicator(volume: currentSystemVolume)
                        .transition(.opacity)
                }

                // Brightness indicator
                if showBrightnessIndicator {
                    BrightnessIndicator(brightness: currentBrightness)
                        .transition(.opacity)
                }

                if showControls {
                    controlsLayer
                        .transition(.opacity)
                }

                if let speedBoostSide {
                    SpeedBoostIndicator(side: speedBoostSide)
                        .transition(.scale.combined(with: .opacity))
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
        .animation(.easeInOut(duration: 0.18), value: showControls)
        .animation(.spring(response: 0.24, dampingFraction: 0.84), value: speedBoostSide)
        .animation(.easeInOut(duration: 0.15), value: showSeekIndicator)
        .animation(.easeInOut(duration: 0.15), value: showVolumeIndicator)
        .animation(.easeInOut(duration: 0.15), value: showBrightnessIndicator)
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
                    .font(.system(size: 16, weight: .semibold))
                    .frame(width: 42, height: 42)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("关闭播放器")

            VStack(alignment: .leading, spacing: 3) {
                Text(file.name)
                    .font(.headline)
                    .lineLimit(1)
                Text(file.sizeFormatted)
                    .font(.caption)
                    .foregroundStyle(.white.opacity(0.68))
            }

            Spacer()

            Button {
                togglePiPSubject.send()
                scheduleControlsAutoHide()
            } label: {
                Image(systemName: "pip.enter")
                    .font(.system(size: 16, weight: .semibold))
                    .frame(width: 42, height: 42)
                    .background(.ultraThinMaterial, in: Circle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel(isImmersive ? "退出全屏" : "进入全屏")
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 18)
        .padding(.top, isImmersive ? 56 : 18)
        .padding(.bottom, 48)
        .background(
            LinearGradient(colors: [.black.opacity(0.74), .black.opacity(0)], startPoint: .top, endPoint: .bottom)
                .allowsHitTesting(false)
        )
    }

    private var bottomControls: some View {
        VStack(spacing: 14) {
            // Custom Progress slider
            CustomSlider(
                value: $currentTime,
                range: 0...max(duration, 1),
                onEditingChanged: { editing in
                    isSeeking = editing
                    if !editing {
                        seek(to: currentTime)
                    }
                }
            )
            .frame(height: 24)

            // Time + controls
            HStack(spacing: 12) {
                Text(formatTime(currentTime))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.72))
                    .frame(width: 54, alignment: .leading)

                Spacer()

                Button {
                    seek(by: -10)
                } label: {
                    Image(systemName: "gobackward.10")
                }
                .accessibilityLabel("后退 10 秒")

                Button {
                    togglePlay()
                } label: {
                    Image(systemName: isPlaying ? "pause.fill" : "play.fill")
                        .font(.system(size: 28, weight: .semibold))
                        .frame(width: 58, height: 58)
                        .background(.white, in: Circle())
                        .foregroundStyle(.black)
                }
                .accessibilityLabel(isPlaying ? "暂停" : "播放")

                Button {
                    seek(by: 10)
                } label: {
                    Image(systemName: "goforward.10")
                }
                .accessibilityLabel("前进 10 秒")

                Spacer()

                Text(formatTime(duration))
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.white.opacity(0.72))
                    .frame(width: 54, alignment: .trailing)
            }
            .font(.system(size: 22, weight: .semibold))
            .buttonStyle(.plain)
            .foregroundStyle(.white)

            // Speed selector + hints
            HStack {
                // Speed menu button
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
                    HStack(spacing: 4) {
                        Image(systemName: "speedometer")
                        Text(String(format: "%.2gx", currentSpeed))
                            .monospacedDigit()
                    }
                    .font(.caption)
                    .foregroundStyle(.white.opacity(0.72))
                    .padding(.horizontal, 10)
                    .padding(.vertical, 6)
                    .background(.white.opacity(0.12), in: Capsule())
                }

                Spacer()

                VStack(alignment: .trailing, spacing: 2) {
                    Label("滑动: 快进/快退 · 音量", systemImage: "hand.draw")
                    Label("双击: 播放/暂停 · 长按: 加速", systemImage: "hand.tap")
                }
                .font(.caption2)
                .foregroundStyle(.white.opacity(0.45))
            }
        }
        .padding(.horizontal, 22)
        .padding(.top, 46)
        .padding(.bottom, isImmersive ? 36 : 18)
        .background(
            LinearGradient(colors: [.black.opacity(0), .black.opacity(0.82)], startPoint: .top, endPoint: .bottom)
                .allowsHitTesting(false)
        )
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
            if !isSeeking, time.seconds.isFinite {
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

    private func toggleControls() {
        withAnimation(.easeInOut(duration: 0.18)) {
            showControls.toggle()
        }
        if showControls {
            scheduleControlsAutoHide()
        } else {
            controlsHideTask?.cancel()
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
                    withAnimation(.easeInOut(duration: 0.2)) {
                        showControls = false
                    }
                }
            }
        }
    }

    private func togglePlay() {
        guard let player else { return }
        if player.timeControlStatus == .playing {
            player.pause()
            isPlaying = false
            controlsHideTask?.cancel()
        } else {
            player.play()
            if currentSpeed != 1.0 {
                player.rate = currentSpeed
            }
            isPlaying = true
            scheduleControlsAutoHide()
        }
    }

    private func seek(by offset: Double) {
        seek(to: min(max(currentTime + offset, 0), duration))
    }

    private func seek(to seconds: Double) {
        guard let player else { return }
        let target = CMTime(seconds: min(max(seconds, 0), max(duration, 0)), preferredTimescale: 600)
        player.seek(to: target, toleranceBefore: .zero, toleranceAfter: .zero)
        scheduleControlsAutoHide()
    }

    private func setSpeed(_ speed: Float) {
        currentSpeed = speed
        guard let player else { return }
        if player.timeControlStatus == .playing {
            player.rate = speed
        }
    }

    private func startSpeedBoost(_ side: PlayerSide) {
        guard speedBoostSide == nil, let player else { return }
        boostWasPlaying = player.timeControlStatus == .playing
        boostRestoreRate = player.rate > 0 ? player.rate : 1
        speedBoostSide = side
        player.playImmediately(atRate: 2)
    }

    private func stopSpeedBoost() {
        guard speedBoostSide != nil, let player else { return }
        if boostWasPlaying {
            player.rate = max(boostRestoreRate, 1)
        } else {
            player.pause()
        }
        speedBoostSide = nil
    }

    private func setSystemVolume(_ volume: Float) {
        let volumeView = MPVolumeView()
        if let slider = volumeView.subviews.first(where: { $0 is UISlider }) as? UISlider {
            slider.value = volume
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

// MARK: - PlayerSide

private enum PlayerSide {
    case left
    case right
}

// MARK: - Gesture Handler (UIKit)

private struct GestureHandlerView: UIViewRepresentable {
    var onSingleTap: () -> Void
    var onDoubleTap: (_ location: CGPoint) -> Void
    var onSwipeHorizontal: (_ deltaX: CGFloat) -> Void
    var onSwipeVertical: (_ location: CGPoint, _ deltaY: CGFloat) -> Void
    var onLongPressStart: (_ location: CGPoint, _ viewWidth: CGFloat) -> Void
    var onLongPressEnd: () -> Void

    func makeUIView(context: Context) -> GestureHandlerUIView {
        let view = GestureHandlerUIView()
        view.onSingleTap = onSingleTap
        view.onDoubleTap = onDoubleTap
        view.onSwipeHorizontal = onSwipeHorizontal
        view.onSwipeVertical = onSwipeVertical
        view.onLongPressStart = onLongPressStart
        view.onLongPressEnd = onLongPressEnd
        view.setupGestures()
        return view
    }

    func updateUIView(_ uiView: GestureHandlerUIView, context: Context) {
        uiView.onSingleTap = onSingleTap
        uiView.onDoubleTap = onDoubleTap
        uiView.onSwipeHorizontal = onSwipeHorizontal
        uiView.onSwipeVertical = onSwipeVertical
        uiView.onLongPressStart = onLongPressStart
        uiView.onLongPressEnd = onLongPressEnd
    }
}

private class GestureHandlerUIView: UIView {
    var onSingleTap: (() -> Void)?
    var onDoubleTap: ((_ location: CGPoint) -> Void)?
    var onSwipeHorizontal: ((_ deltaX: CGFloat) -> Void)?
    var onSwipeVertical: ((_ location: CGPoint, _ deltaY: CGFloat) -> Void)?
    var onLongPressStart: ((_ location: CGPoint, _ viewWidth: CGFloat) -> Void)?
    var onLongPressEnd: (() -> Void)?

    private var touchStartPoint: CGPoint?
    private var touchStartTime: TimeInterval = 0
    private var isLongPressing = false
    private var longPressTimer: Timer?
    private var tapCount = 0
    private var tapTimer: Timer?
    private var accumulatedSwipeX: CGFloat = 0
    private var accumulatedSwipeY: CGFloat = 0
    private var swipeDirection: SwipeDirection = .none

    private enum SwipeDirection {
        case none, horizontal, vertical
    }

    override init(frame: CGRect) {
        super.init(frame: frame)
        backgroundColor = .clear
        isMultipleTouchEnabled = false
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    func setupGestures() {
        // Gestures are handled via touch methods directly
    }

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let touch = touches.first else { return }
        let location = touch.location(in: self)
        touchStartPoint = location
        touchStartTime = touch.timestamp
        isLongPressing = false
        accumulatedSwipeX = 0
        accumulatedSwipeY = 0
        swipeDirection = .none

        // Start long-press timer (0.35s)
        longPressTimer?.invalidate()
        longPressTimer = Timer.scheduledTimer(withTimeInterval: 0.35, repeats: false) { [weak self] _ in
            guard let self, !self.isLongPressing else { return }
            self.isLongPressing = true
            let viewWidth = self.bounds.width
            DispatchQueue.main.async {
                self.onLongPressStart?(location, viewWidth)
            }
        }
    }

    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let touch = touches.first, let start = touchStartPoint else { return }
        let current = touch.location(in: self)
        let dx = current.x - start.x
        let dy = current.y - start.y

        // If long pressing, don't process swipes
        if isLongPressing { return }

        // Determine swipe direction on first significant movement
        if swipeDirection == .none {
            let absDx = abs(dx)
            let absDy = abs(dy)
            if absDx > 12 || absDy > 12 {
                if absDx > absDy {
                    swipeDirection = .horizontal
                    longPressTimer?.invalidate()
                } else {
                    swipeDirection = .vertical
                    longPressTimer?.invalidate()
                }
            }
        }

        // Accumulate delta and fire callbacks
        if swipeDirection == .horizontal {
            let step = dx - accumulatedSwipeX
            accumulatedSwipeX = dx
            if abs(step) > 2 {
                DispatchQueue.main.async {
                    self.onSwipeHorizontal?(step)
                }
            }
            touchStartPoint = current // reset base for continuous tracking
        } else if swipeDirection == .vertical {
            let step = dy - accumulatedSwipeY
            accumulatedSwipeY = dy
            if abs(step) > 2 {
                DispatchQueue.main.async {
                    self.onSwipeVertical?(current, step)
                }
            }
            touchStartPoint = current
        }
    }

    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        longPressTimer?.invalidate()
        longPressTimer = nil

        if isLongPressing {
            isLongPressing = false
            DispatchQueue.main.async {
                self.onLongPressEnd?()
            }
            return
        }

        guard let touch = touches.first else { return }
        let endPoint = touch.location(in: self)
        let duration = touch.timestamp - touchStartTime

        // If it was a swipe (significant movement), don't count as tap
        if swipeDirection != .none { return }

        // Short tap (< 0.3s, < 20px movement)
        if duration < 0.3, let start = touchStartPoint {
            let dist = hypot(endPoint.x - start.x, endPoint.y - start.y)
            if dist < 20 {
                handleTap(at: endPoint)
            }
        }
    }

    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        longPressTimer?.invalidate()
        longPressTimer = nil
        if isLongPressing {
            isLongPressing = false
            DispatchQueue.main.async {
                self.onLongPressEnd?()
            }
        }
        touchStartPoint = nil
        swipeDirection = .none
    }

    private func handleTap(at location: CGPoint) {
        tapCount += 1
        tapTimer?.invalidate()

        if tapCount >= 2 {
            // Double tap
            tapCount = 0
            DispatchQueue.main.async {
                self.onDoubleTap?(location)
            }
        } else {
            // Wait for potential second tap
            tapTimer = Timer.scheduledTimer(withTimeInterval: 0.28, repeats: false) { [weak self] _ in
                guard let self else { return }
                if self.tapCount == 1 {
                    self.tapCount = 0
                    DispatchQueue.main.async {
                        self.onSingleTap?()
                    }
                }
            }
        }
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

// MARK: - Speed Boost Indicator

private struct SpeedBoostIndicator: View {
    let side: PlayerSide

    var body: some View {
        VStack {
            Spacer()
            HStack {
                if side == .right { Spacer() }
                HStack(spacing: 7) {
                    Image(systemName: "bolt.fill")
                    Text("2.0x")
                        .font(.headline.monospacedDigit())
                }
                .foregroundStyle(.white)
                .padding(.horizontal, 12)
                .padding(.vertical, 9)
                .background(.black.opacity(0.54), in: Capsule())
                .overlay(Capsule().stroke(.white.opacity(0.16), lineWidth: 1))
                .shadow(color: .black.opacity(0.22), radius: 10, y: 4)
                if side == .left { Spacer() }
            }
            .padding(.horizontal, 24)
            .padding(.bottom, 110)
        }
        .allowsHitTesting(false)
    }
}

// MARK: - Seek Delta Indicator

private struct SeekDeltaIndicator: View {
    let delta: Double

    var body: some View {
        VStack(spacing: 4) {
            Image(systemName: delta > 0 ? "goforward" : "gobackward")
                .font(.system(size: 24))
            Text("\(delta > 0 ? "+" : "")\(Int(delta))s")
                .font(.title3.monospacedDigit().bold())
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 20)
        .padding(.vertical, 14)
        .background(.black.opacity(0.6), in: RoundedRectangle(cornerRadius: 14))
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
                .font(.system(size: 18))
                .frame(width: 24)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(.white.opacity(0.2))
                    Capsule()
                        .fill(.white)
                        .frame(width: geo.size.width * CGFloat(volume))
                }
            }
            .frame(width: 120, height: 4)
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 18)
        .padding(.vertical, 12)
        .background(.black.opacity(0.6), in: RoundedRectangle(cornerRadius: 12))
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
                .font(.system(size: 18))
                .frame(width: 24)

            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(.white.opacity(0.2))
                    Capsule()
                        .fill(.white)
                        .frame(width: geo.size.width * brightness)
                }
            }
            .frame(width: 120, height: 4)
        }
        .foregroundStyle(.white)
        .padding(.horizontal, 18)
        .padding(.vertical, 12)
        .background(.black.opacity(0.6), in: RoundedRectangle(cornerRadius: 12))
        .shadow(color: .black.opacity(0.3), radius: 12)
        .allowsHitTesting(false)
    }
}

// MARK: - Custom Slider

private struct CustomSlider: View {
    @Binding var value: Double
    var range: ClosedRange<Double>
    var onEditingChanged: (Bool) -> Void

    @State private var dragPercent: Double? = nil

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let percent = dragPercent ?? (max(0, min(value / max(range.upperBound, 1), 1)))
            
            ZStack(alignment: .leading) {
                // Background track
                Capsule()
                    .fill(Color.white.opacity(0.3))
                    .frame(height: 4)
                
                // Filled track
                Capsule()
                    .fill(Color.red)
                    .frame(width: max(0, width * percent), height: 4)
                
                // Thumb
                Circle()
                    .fill(Color.white)
                    .frame(width: 14, height: 14)
                    .offset(x: max(0, min(width * percent - 7, width - 14)))
                    .shadow(radius: 2)
                    .scaleEffect(dragPercent != nil ? 1.4 : 1.0)
                    .animation(.spring(response: 0.2, dampingFraction: 0.7), value: dragPercent != nil)
            }
            .frame(minHeight: 24)
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { gesture in
                        if dragPercent == nil {
                            onEditingChanged(true)
                        }
                        let p = min(max(gesture.location.x / width, 0), 1)
                        dragPercent = p
                        value = p * range.upperBound
                    }
                    .onEnded { gesture in
                        let p = min(max(gesture.location.x / width, 0), 1)
                        value = p * range.upperBound
                        dragPercent = nil
                        onEditingChanged(false)
                    }
            )
        }
    }
}


