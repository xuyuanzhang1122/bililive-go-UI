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

    // Long-press speed boost
    @State private var speedBoostSide: PlayerSide?
    @State private var boostRestoreRate: Float = 1
    @State private var boostWasPlaying = false

    // Swipe feedback
    @State private var seekDelta: Double = 0
    @State private var showSeekIndicator = false
    @State private var showVolumeIndicator = false
    @State private var currentSystemVolume: Float = 0

    @State private var showBrightnessIndicator = false
    @State private var currentBrightness: CGFloat = 0
    @State private var seekGestureStartTime: Double = 0
    @State private var seekIndicatorHideTask: Task<Void, Never>?
    @State private var volumeIndicatorHideTask: Task<Void, Never>?
    @State private var brightnessIndicatorHideTask: Task<Void, Never>?

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
                    onSwipeStart: { direction, _ in
                        controlsHideTask?.cancel()
                        if direction == .horizontal {
                            isSeeking = true
                            seekGestureStartTime = currentTime
                            showSeekIndicator = true
                            withAnimation(.easeInOut(duration: 0.18)) {
                                showControls = true
                            }
                        }
                    },
                    onSwipeHorizontal: { _, translation, width in
                        let span = horizontalSeekSpan()
                        let seconds = Double(translation / max(width, 1)) * span
                        let target = clampedTime(seekGestureStartTime + seconds)
                        currentTime = target
                        seekDelta = target - seekGestureStartTime
                        showSeekIndicator = true
                    },
                    onSwipeVertical: { location, delta in
                        let viewWidth = UIScreen.main.bounds.width
                        if location.x > viewWidth / 2 {
                            // delta: negative = swipe up (louder), positive = swipe down (quieter)
                            let volumeChange = Float(-delta) * 0.006
                            let newVol = min(max(currentSystemVolume + volumeChange, 0), 1)
                            setSystemVolume(newVol)
                            currentSystemVolume = newVol
                            showVolumeIndicator = true
                            hideVolumeIndicatorSoon()
                        } else {
                            let brightnessChange = CGFloat(-delta) * 0.006
                            let newBright = min(max(currentBrightness + brightnessChange, 0), 1)
                            UIScreen.main.brightness = newBright
                            currentBrightness = newBright
                            showBrightnessIndicator = true
                            hideBrightnessIndicatorSoon()
                        }
                    },
                    onSwipeEnd: { direction in
                        if direction == .horizontal {
                            isSeeking = false
                            seek(to: currentTime)
                            hideSeekIndicatorSoon()
                        } else {
                            scheduleControlsAutoHide()
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
                    SeekDeltaIndicator(delta: seekDelta, targetTime: currentTime, valueFormatter: formatTime)
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
        VStack(spacing: 16) {
            PrecisionTimelineSlider(
                value: $currentTime,
                range: 0...max(duration, 1),
                valueFormatter: formatTime,
                onEditingChanged: { editing in
                    isSeeking = editing
                    if editing {
                        controlsHideTask?.cancel()
                        withAnimation(.easeInOut(duration: 0.18)) {
                            showControls = true
                        }
                    } else {
                        seek(to: currentTime)
                    }
                }
            )
            .frame(height: 52)

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
                    Label("左侧亮度 · 右侧音量", systemImage: "sun.max")
                    Label("横滑预览进度 · 松手跳转", systemImage: "arrow.left.and.right")
                }
                .font(.caption2)
                .foregroundStyle(.white.opacity(0.45))
            }
        }
        .padding(.horizontal, 22)
        .padding(.top, 54)
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
        seekIndicatorHideTask?.cancel()
        volumeIndicatorHideTask?.cancel()
        brightnessIndicatorHideTask?.cancel()
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
        Task { @MainActor in
            SystemVolumeController.shared.setVolume(volume)
        }
    }

    private func hideSeekIndicatorSoon() {
        seekIndicatorHideTask?.cancel()
        seekIndicatorHideTask = Task {
            try? await Task.sleep(for: .milliseconds(700))
            guard !Task.isCancelled else { return }
            await MainActor.run { showSeekIndicator = false }
        }
    }

    private func hideVolumeIndicatorSoon() {
        volumeIndicatorHideTask?.cancel()
        volumeIndicatorHideTask = Task {
            try? await Task.sleep(for: .milliseconds(650))
            guard !Task.isCancelled else { return }
            await MainActor.run {
                showVolumeIndicator = false
                scheduleControlsAutoHide()
            }
        }
    }

    private func hideBrightnessIndicatorSoon() {
        brightnessIndicatorHideTask?.cancel()
        brightnessIndicatorHideTask = Task {
            try? await Task.sleep(for: .milliseconds(650))
            guard !Task.isCancelled else { return }
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

// MARK: - PlayerSide

private enum PlayerSide {
    case left
    case right
}

private enum PlayerGestureDirection {
    case horizontal
    case vertical
}

// MARK: - Gesture Handler (UIKit)

private struct GestureHandlerView: UIViewRepresentable {
    var onSingleTap: () -> Void
    var onDoubleTap: (_ location: CGPoint) -> Void
    var onSwipeStart: (_ direction: PlayerGestureDirection, _ location: CGPoint) -> Void
    var onSwipeHorizontal: (_ location: CGPoint, _ translationX: CGFloat, _ viewWidth: CGFloat) -> Void
    var onSwipeVertical: (_ location: CGPoint, _ deltaY: CGFloat) -> Void
    var onSwipeEnd: (_ direction: PlayerGestureDirection) -> Void
    var onLongPressStart: (_ location: CGPoint, _ viewWidth: CGFloat) -> Void
    var onLongPressEnd: () -> Void

    func makeUIView(context: Context) -> GestureHandlerUIView {
        let view = GestureHandlerUIView()
        view.onSingleTap = onSingleTap
        view.onDoubleTap = onDoubleTap
        view.onSwipeStart = onSwipeStart
        view.onSwipeHorizontal = onSwipeHorizontal
        view.onSwipeVertical = onSwipeVertical
        view.onSwipeEnd = onSwipeEnd
        view.onLongPressStart = onLongPressStart
        view.onLongPressEnd = onLongPressEnd
        view.setupGestures()
        return view
    }

    func updateUIView(_ uiView: GestureHandlerUIView, context: Context) {
        uiView.onSingleTap = onSingleTap
        uiView.onDoubleTap = onDoubleTap
        uiView.onSwipeStart = onSwipeStart
        uiView.onSwipeHorizontal = onSwipeHorizontal
        uiView.onSwipeVertical = onSwipeVertical
        uiView.onSwipeEnd = onSwipeEnd
        uiView.onLongPressStart = onLongPressStart
        uiView.onLongPressEnd = onLongPressEnd
    }
}

private class GestureHandlerUIView: UIView {
    var onSingleTap: (() -> Void)?
    var onDoubleTap: ((_ location: CGPoint) -> Void)?
    var onSwipeStart: ((_ direction: PlayerGestureDirection, _ location: CGPoint) -> Void)?
    var onSwipeHorizontal: ((_ location: CGPoint, _ translationX: CGFloat, _ viewWidth: CGFloat) -> Void)?
    var onSwipeVertical: ((_ location: CGPoint, _ deltaY: CGFloat) -> Void)?
    var onSwipeEnd: ((_ direction: PlayerGestureDirection) -> Void)?
    var onLongPressStart: ((_ location: CGPoint, _ viewWidth: CGFloat) -> Void)?
    var onLongPressEnd: (() -> Void)?

    private var touchStartPoint: CGPoint?
    private var lastTouchPoint: CGPoint?
    private var touchStartTime: TimeInterval = 0
    private var isLongPressing = false
    private var longPressTimer: Timer?
    private var tapCount = 0
    private var tapTimer: Timer?
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
        lastTouchPoint = location
        touchStartTime = touch.timestamp
        isLongPressing = false
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
                    DispatchQueue.main.async {
                        self.onSwipeStart?(.horizontal, current)
                    }
                } else {
                    swipeDirection = .vertical
                    longPressTimer?.invalidate()
                    DispatchQueue.main.async {
                        self.onSwipeStart?(.vertical, current)
                    }
                }
            }
        }

        if swipeDirection == .horizontal {
            DispatchQueue.main.async {
                self.onSwipeHorizontal?(current, dx, self.bounds.width)
            }
        } else if swipeDirection == .vertical {
            let previous = lastTouchPoint ?? start
            let step = current.y - previous.y
            lastTouchPoint = current
            if abs(step) > 0.5 {
                DispatchQueue.main.async {
                    self.onSwipeVertical?(current, step)
                }
            }
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

        if swipeDirection != .none {
            let direction = swipeDirection
            swipeDirection = .none
            touchStartPoint = nil
            lastTouchPoint = nil
            DispatchQueue.main.async {
                self.onSwipeEnd?(direction == .horizontal ? .horizontal : .vertical)
            }
            return
        }

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
        if swipeDirection != .none {
            let direction = swipeDirection
            DispatchQueue.main.async {
                self.onSwipeEnd?(direction == .horizontal ? .horizontal : .vertical)
            }
        }
        touchStartPoint = nil
        lastTouchPoint = nil
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
        .background(.black.opacity(0.66), in: RoundedRectangle(cornerRadius: 16))
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

// MARK: - Precision Timeline Slider

private struct PrecisionTimelineSlider: View {
    @Binding var value: Double
    var range: ClosedRange<Double>
    var valueFormatter: (Double) -> String
    var onEditingChanged: (Bool) -> Void

    @State private var dragPercent: Double? = nil

    var body: some View {
        GeometryReader { geometry in
            let width = geometry.size.width
            let usableWidth = max(width, 1)
            let lower = range.lowerBound
            let upper = max(range.upperBound, lower + 1)
            let percent = dragPercent ?? min(max((value - lower) / (upper - lower), 0), 1)
            let thumbX = min(max(usableWidth * percent, 0), usableWidth)
            let isDragging = dragPercent != nil

            ZStack(alignment: .leading) {
                Capsule()
                    .fill(.white.opacity(0.18))
                    .frame(height: isDragging ? 7 : 5)

                Capsule()
                    .fill(
                        LinearGradient(
                            colors: [Color(red: 1, green: 0.22, blue: 0.22), Color(red: 1, green: 0.62, blue: 0.24)],
                            startPoint: .leading,
                            endPoint: .trailing
                        )
                    )
                    .frame(width: max(0, thumbX), height: isDragging ? 7 : 5)

                Circle()
                    .fill(.white)
                    .frame(width: isDragging ? 24 : 18, height: isDragging ? 24 : 18)
                    .overlay(Circle().stroke(.black.opacity(0.18), lineWidth: 1))
                    .shadow(color: .black.opacity(0.36), radius: 7, y: 3)
                    .position(x: thumbX, y: 26)

                if isDragging {
                    Text(valueFormatter(value))
                        .font(.caption.monospacedDigit().weight(.semibold))
                        .foregroundStyle(.black)
                        .padding(.horizontal, 9)
                        .padding(.vertical, 5)
                        .background(.white, in: Capsule())
                        .position(x: min(max(thumbX, 34), usableWidth - 34), y: 0)
                        .transition(.opacity.combined(with: .scale(scale: 0.92)))
                }
            }
            .frame(height: 52)
            .contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { gesture in
                        if dragPercent == nil {
                            onEditingChanged(true)
                        }
                        let p = min(max(gesture.location.x / usableWidth, 0), 1)
                        dragPercent = p
                        value = lower + p * (upper - lower)
                    }
                    .onEnded { gesture in
                        let p = min(max(gesture.location.x / usableWidth, 0), 1)
                        value = lower + p * (upper - lower)
                        dragPercent = nil
                        onEditingChanged(false)
                    }
            )
            .animation(.spring(response: 0.2, dampingFraction: 0.78), value: isDragging)
        }
    }
}
