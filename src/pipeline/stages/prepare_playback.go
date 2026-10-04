package stages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
	"github.com/shirou/gopsutil/v3/disk"
)

// 修改准备规则时必须升版本，不能复用旧规则的完成标记。
const PreparationProfileVersion = "sdr-h264-main-1080p30-aac-stereo-v1"

type PreparePlaybackStage struct {
	commands  []string
	logs      string
	freeBytes func(string) (uint64, error)
}

func NewPreparePlaybackStage(config pipeline.StageConfig) (pipeline.Stage, error) {
	if expected, ok := config.Options["expected_profile_version"]; ok && expected != PreparationProfileVersion {
		return nil, fmt.Errorf("准备规则已变化，需要重新预览确认")
	}
	return &PreparePlaybackStage{}, nil
}
func (s *PreparePlaybackStage) Name() string          { return pipeline.StageNamePreparePlayback }
func (s *PreparePlaybackStage) GetCommands() []string { return s.commands }
func (s *PreparePlaybackStage) GetLogs() string       { return s.logs }
func (s *PreparePlaybackStage) free(path string) (uint64, error) {
	if s.freeBytes != nil {
		return s.freeBytes(path)
	}
	usage, err := disk.Usage(path)
	if err != nil {
		return 0, err
	}
	return usage.Free, nil
}

func (s *PreparePlaybackStage) Execute(ctx *pipeline.PipelineContext, input []pipeline.FileInfo) ([]pipeline.FileInfo, error) {
	if err := ctx.Ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Catalog == nil || ctx.Preparations == nil {
		return nil, fmt.Errorf("准备阶段缺少持久目录或任务存储")
	}
	for _, file := range input {
		if file.Type != pipeline.FileTypeVideo {
			continue
		}
		if err := s.prepare(ctx, file); err != nil {
			return nil, err
		}
	}
	// 兼容资产不进入旧的上传、修复或删除阶段；源仍交给原有流程。
	return input, nil
}

func (s *PreparePlaybackStage) prepare(ctx *pipeline.PipelineContext, file pipeline.FileInfo) (resultErr error) {
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion)
	if errors.Is(err, recordingcatalog.ErrCatalogPending) || errors.Is(err, recordingcatalog.ErrRecordingActive) {
		ctx.Catalog.Notify()
		return &pipeline.PreparationPausedError{Reason: err.Error()}
	}
	if err != nil {
		return err
	}
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion}
	job, err := ctx.Preparations.QueuePreparation(ctx.Ctx, key)
	if err != nil {
		return err
	}
	if job.Status == pipeline.PreparationReady {
		_, e := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion)
		if e == nil {
			e = ctx.Catalog.VerifyPreparedAsset(ctx.Ctx, entry, job.OutputPath, job.OutputVersion, PreparationProfileVersion)
		}
		if e == nil && job.OutputPath != file.Path {
			info, infoErr := os.Lstat(file.Path)
			if infoErr != nil {
				e = infoErr
			} else {
				_, e = preparationManifest(file.Path, job.OutputPath, info, filepath.Dir(job.OutputPath) != filepath.Dir(file.Path))
			}
		}
		current, versionErr := recordingcatalog.SnapshotVersion(job.OutputPath)
		if e == nil && versionErr == nil && current == job.OutputVersion {
			s.logs += "复用已核验准备产物\n"
			return nil
		}
		if err := ctx.Preparations.InvalidatePreparation(ctx.Ctx, key); err != nil {
			return err
		}
		if err := ctx.Catalog.InvalidatePreparedAsset(ctx.Ctx, entry, job.OutputPath); err != nil {
			return err
		}
		return fmt.Errorf("已完成准备产物身份变化，拒绝复用或覆盖")
	}
	if err := ctx.Preparations.ClaimPreparation(ctx.Ctx, key); err != nil {
		return err
	}
	output, outputVersion, candidateOutput := "", "", ""
	defer func() {
		status, reason := pipeline.PreparationReady, ""
		if resultErr != nil {
			status = pipeline.PreparationFailed
			reason = resultErr.Error()
			var paused *pipeline.PreparationPausedError
			if errors.As(resultErr, &paused) || ctx.Ctx.Err() != nil {
				status = pipeline.PreparationPaused
			}
			if candidateOutput != "" {
				if err := ctx.Catalog.InvalidatePreparedAsset(context.WithoutCancel(ctx.Ctx), entry, candidateOutput); err != nil {
					resultErr = errors.Join(resultErr, err)
				}
			}
		}
		if err := ctx.Preparations.FinishPreparation(context.WithoutCancel(ctx.Ctx), key, status, output, reason, outputVersion); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	limit := 3
	reserve := uint64(1_000_000_000)
	if cfg := configs.GetCurrentConfig(); cfg != nil {
		if cfg.HLSCache.MaxConcurrent > 0 {
			limit = cfg.HLSCache.MaxConcurrent
		}
		if cfg.HLSCache.MinFreeSpaceGB >= 0 {
			reserve = uint64(cfg.HLSCache.MinFreeSpaceGB * 1e9)
		}
	}
	release, err := mediawork.Shared.Acquire(ctx.Ctx, limit)
	if err != nil {
		return err
	}
	defer release()
	if _, err = ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
		if errors.Is(err, recordingcatalog.ErrRecordingActive) {
			return &pipeline.PreparationPausedError{Reason: err.Error()}
		}
		return err
	}
	ffmpeg := ctx.FFmpegPath
	if ffmpeg == "" {
		ffmpeg, err = utils.GetFFmpegPath(ctx.Ctx)
		if err != nil {
			return err
		}
	}
	sourceInfo, err := os.Lstat(file.Path)
	if err != nil {
		return err
	}
	media, err := probePreparation(ctx.Ctx, ffmpeg, file.Path)
	if err != nil {
		return err
	}
	mode := media.mode()
	derived := mode != "copy"
	target := strings.TrimSuffix(file.Path, filepath.Ext(file.Path)) + ".mp4"
	if strings.EqualFold(filepath.Ext(file.Path), ".mp4") {
		target = strings.TrimSuffix(file.Path, filepath.Ext(file.Path)) + ".prepared.mp4"
	}
	assetVersion := entry.SourceVersion + "-" + PreparationProfileVersion
	if derived {
		dir := filepath.Join(ctx.Catalog.AssetRoot(), entry.RecordingID, assetVersion)
		if err := safePreparationDir(ctx.Catalog.AssetRoot(), entry.RecordingID, assetVersion); err != nil {
			return err
		}
		target = filepath.Join(dir, "compat.mp4")
	}
	candidateOutput = target
	validatedVersion := ""
	validate := func(path string) error {
		before, e := recordingcatalog.SnapshotVersion(path)
		if e != nil {
			return e
		}
		inspected, e := probePreparation(ctx.Ctx, ffmpeg, path)
		if e != nil {
			return e
		}
		if inspected.mode() != "copy" || inspected.audio != media.audio || math.Abs(inspected.duration-media.duration) > math.Max(1, media.duration*0.01) {
			return fmt.Errorf("准备产物编码、轨道或时长不符")
		}
		if err := checkMP4Faststart(path); err != nil {
			return err
		}
		after, e := recordingcatalog.SnapshotVersion(path)
		if e != nil {
			return e
		}
		if after != before {
			return fmt.Errorf("媒体探测期间准备产物身份变化")
		}
		validatedVersion = after
		return nil
	}
	if strings.EqualFold(filepath.Ext(file.Path), ".mp4") && mode == "copy" && checkMP4Faststart(file.Path) == nil {
		target = file.Path
		candidateOutput = target
		validatedVersion = file.PreparationSourceVersion
	} else {
		recover := playbackasset.RecoverPublication
		publish := playbackasset.PublishValidated
		if derived {
			recover = playbackasset.RecoverDerivedPublication
			publish = playbackasset.PublishDerivedValidated
		}
		if _, err := recover(file.Path, target, sourceInfo, validate); err != nil {
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			_, e := preparationManifest(file.Path, target, sourceInfo, derived)
			if e != nil {
				return fmt.Errorf("已有产物来源不匹配，拒绝覆盖")
			}
			if err := validate(target); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		} else {
			estimate := math.Max(float64(sourceInfo.Size())*1.25, media.duration*800_000)
			free, e := s.free(filepath.Dir(target))
			if e != nil {
				return &pipeline.PreparationPausedError{Reason: "无法核验可用空间: " + e.Error()}
			}
			if float64(free) < float64(reserve)+estimate {
				return &pipeline.PreparationPausedError{Reason: "低空间暂停准备，源文件已保留"}
			}
			temp, e := os.CreateTemp(filepath.Dir(target), ".converting-*.mp4")
			if e != nil {
				return e
			}
			tempPath := temp.Name()
			defer os.Remove(tempPath)
			if err := temp.Close(); err != nil {
				return err
			}
			args := preparationArgs(file.Path, tempPath, mode, media)
			s.commands = append(s.commands, ffmpeg+" "+strings.Join(args, " "))
			if err := s.run(ctx.Ctx, ffmpeg, args, filepath.Dir(target), reserve); err != nil {
				return err
			}
			if err := validate(tempPath); err != nil {
				return err
			}
			if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
				return err
			}
			f, e := os.OpenFile(tempPath, os.O_RDWR, 0)
			if e != nil {
				return e
			}
			e = f.Sync()
			closeErr := f.Close()
			if e != nil {
				return e
			}
			if closeErr != nil {
				return closeErr
			}
			if err := publish(file.Path, target, tempPath, sourceInfo); err != nil {
				return err
			}
		}
	}
	if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
		return err
	}
	// 发布、恢复或直接复用都必须延续本次实际媒体验证的同一文件身份。
	currentOutput, err := recordingcatalog.SnapshotVersion(target)
	if err != nil {
		return err
	}
	if currentOutput != validatedVersion {
		return fmt.Errorf("准备产物在验证后已变化")
	}
	var committedManifest playbackasset.Manifest
	if target != file.Path {
		committedManifest, err = preparationManifest(file.Path, target, sourceInfo, derived)
		if err != nil {
			return err
		}
	}
	if derived {
		rel := filepath.ToSlash(filepath.Join(entry.RecordingID, assetVersion, "compat.mp4"))
		if err := ctx.Catalog.RegisterDerived(ctx.Ctx, entry.RecordingID, entry.SourceVersion, assetVersion, rel, "compat"); err != nil {
			return err
		}
	}
	// 完成标记只能晚于已提交资产和统计。扫描发生在受控后台任务，HTTP仍只读目录。
	if err := ctx.Catalog.Reconcile(ctx.Ctx); err != nil {
		return &pipeline.PreparationPausedError{Reason: err.Error()}
	}
	if !derived {
		if err := ctx.Catalog.RegisterPreparedMain(ctx.Ctx, entry, target, assetVersion); err != nil {
			return err
		}
	}
	outputVersion, err = recordingcatalog.SnapshotVersion(target)
	if err != nil {
		return err
	}
	if outputVersion != validatedVersion {
		return fmt.Errorf("准备产物提交期间身份变化")
	}
	if target != file.Path {
		current, e := preparationManifest(file.Path, target, sourceInfo, derived)
		if e != nil {
			return e
		}
		if current != committedManifest {
			return fmt.Errorf("准备产物提交期间关联变化")
		}
	}
	if err := ctx.Catalog.RecordPreparedVerification(ctx.Ctx, entry, target, validatedVersion, PreparationProfileVersion); err != nil {
		return err
	}
	if err := ctx.Catalog.VerifyPreparedAsset(ctx.Ctx, entry, target, validatedVersion, PreparationProfileVersion); err != nil {
		return err
	}
	if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
		return err
	}
	output = target
	s.logs += mode + " " + target + "\n"
	return nil
}

func preparationManifest(source, target string, sourceInfo os.FileInfo, derived bool) (playbackasset.Manifest, error) {
	m, err := playbackasset.Load(target)
	if err != nil {
		return m, err
	}
	if m.SourceName != filepath.Base(source) || m.Source.Size != sourceInfo.Size() || m.Source.ModTime != sourceInfo.ModTime().UnixNano() || (!derived && m.Version != 1) || (derived && (m.Version != 2 || m.SourcePath != source)) {
		return m, fmt.Errorf("准备产物来源关联不匹配")
	}
	return m, nil
}

func safePreparationDir(root, id, version string) error {
	for _, part := range []string{root, filepath.Join(root, id), filepath.Join(root, id, version)} {
		if err := os.MkdirAll(part, 0755); err != nil {
			return err
		}
		info, err := os.Lstat(part)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("派生目录包含符号链接或不是目录")
		}
	}
	return nil
}

func (s *PreparePlaybackStage) run(ctx context.Context, ffmpeg string, args []string, dir string, reserve uint64) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, ffmpeg, args...)
	stderr := &mp4LimitedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return fmt.Errorf("准备转换失败: %w: %s", err, stderr.String())
			}
			return nil
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		case <-ticker.C:
			free, err := s.free(dir)
			if err != nil || free < reserve {
				cancel()
				<-done
				return &pipeline.PreparationPausedError{Reason: "转换期间空间不足或核验失败，源文件已保留"}
			}
		}
	}
}

type preparationStream struct {
	Type       string `json:"codec_type"`
	Codec      string `json:"codec_name"`
	Profile    string `json:"profile"`
	Level      int    `json:"level"`
	Pixel      string `json:"pix_fmt"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FrameRate  string `json:"avg_frame_rate"`
	Transfer   string `json:"color_transfer"`
	Channels   int    `json:"channels"`
	SampleRate string `json:"sample_rate"`
	Packets    string `json:"nb_read_packets"`
}
type preparationMedia struct {
	video       preparationStream
	audio       bool
	audioStream preparationStream
	duration    float64
}

func probePreparation(ctx context.Context, ffmpeg, path string) (preparationMedia, error) {
	var media preparationMedia
	probe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe"+filepath.Ext(ffmpeg))
	if _, err := os.Stat(probe); err != nil {
		probe, err = exec.LookPath("ffprobe")
		if err != nil {
			return media, err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe, "-v", "error", "-count_packets", "-show_entries", "stream=codec_type,codec_name,profile,level,pix_fmt,width,height,avg_frame_rate,color_transfer,channels,sample_rate,nb_read_packets:format=duration", "-of", "json", path)
	var stdout bytes.Buffer
	stderr := &mp4LimitedBuffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return media, ctx.Err()
		}
		return media, fmt.Errorf("准备媒体探测失败: %w: %s", err, stderr.String())
	}
	if stderr.Len() > 0 {
		return media, fmt.Errorf("准备媒体读取错误: %s", stderr.String())
	}
	var data struct {
		Streams []preparationStream `json:"streams"`
		Format  struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		return media, err
	}
	media.duration, _ = strconv.ParseFloat(data.Format.Duration, 64)
	if media.duration <= 0 || math.IsNaN(media.duration) || math.IsInf(media.duration, 0) {
		return media, fmt.Errorf("源时长无效")
	}
	for _, stream := range data.Streams {
		if (stream.Type == "video" && media.video.Type == "") || (stream.Type == "audio" && !media.audio) {
			packets, _ := strconv.ParseInt(stream.Packets, 10, 64)
			if packets <= 0 {
				return media, fmt.Errorf("源轨道无可读包")
			}
			if stream.Type == "video" {
				media.video = stream
			} else {
				media.audio = true
				media.audioStream = stream
			}
		}
	}
	if media.video.Type == "" {
		return media, fmt.Errorf("缺少视频轨道")
	}
	return media, nil
}
func (m preparationMedia) mode() string {
	v := m.video
	fps := 0.0
	parts := strings.Split(v.FrameRate, "/")
	if len(parts) == 2 {
		num, _ := strconv.ParseFloat(parts[0], 64)
		den, _ := strconv.ParseFloat(parts[1], 64)
		if den > 0 {
			fps = num / den
		}
	}
	profile := v.Profile == "Constrained Baseline" || v.Profile == "Baseline" || v.Profile == "Main" || v.Profile == "High"
	if v.Codec != "h264" || !profile || v.Pixel != "yuv420p" || v.Level <= 0 || v.Level > 42 || v.Width <= 0 || v.Height <= 0 || v.Width > 1920 || v.Height > 1080 || fps <= 0 || fps > 60 || v.Transfer == "smpte2084" || v.Transfer == "arib-std-b67" {
		return "video-transcode"
	}
	if m.audio && !compatiblePreparationAudio(m.audioStream) {
		return "audio-transcode"
	}
	return "copy"
}
func preparationArgs(source, target, mode string, media preparationMedia) []string {
	args := []string{"-nostdin", "-v", "error", "-i", source, "-map", "0:v:0", "-map", "0:a:0?"}
	if mode == "video-transcode" {
		filter := "scale=w='min(1920,iw)':h='min(1080,ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"
		if media.video.Transfer == "smpte2084" || media.video.Transfer == "arib-std-b67" {
			filter = "zscale=t=linear:npl=100,format=gbrpf32le,tonemap=tonemap=hable:desat=0,zscale=p=bt709:t=bt709:m=bt709:r=tv,format=yuv420p," + filter
		}
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-maxrate", "6M", "-bufsize", "12M", "-profile:v", "main", "-level:v", "4.1", "-pix_fmt", "yuv420p", "-vf", filter, "-r", "30", "-color_primaries", "bt709", "-color_trc", "bt709", "-colorspace", "bt709", "-x264-params", "colorprim=bt709:transfer=bt709:colormatrix=bt709")
	} else {
		args = append(args, "-c:v", "copy")
	}
	if media.audio && (mode == "audio-transcode" || (mode == "video-transcode" && !compatiblePreparationAudio(media.audioStream))) {
		args = append(args, "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-ar", "48000")
	} else {
		args = append(args, "-c:a", "copy")
	}
	return append(args, "-movflags", "+faststart", "-y", target)
}

func compatiblePreparationAudio(a preparationStream) bool {
	rate, _ := strconv.Atoi(a.SampleRate)
	return a.Codec == "aac" && a.Profile == "LC" && a.Channels > 0 && a.Channels <= 2 && rate > 0 && rate <= 48000
}
