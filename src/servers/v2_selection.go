package servers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/bililive-go/bililive-go/src/pkg/playbackdecision"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
)

func v2MediaParameters(p mediaProbe, protocol, container string) playbackdecision.Media {
	m := playbackdecision.Media{Protocol: protocol, Container: container}
	if container == "" {
		m.Container = p.Format.Container
	}
	if m.Container == "" {
		switch {
		case strings.Contains(p.Format.FormatName, "mpegts"):
			m.Container = "ts"
		case strings.Contains(p.Format.FormatName, "flv"):
			m.Container = "flv"
		}
	}
	for _, v := range p.Streams {
		if v.CodecType == "video" && m.Video.Codec == "" {
			fps := 0.0
			parts := strings.Split(v.FrameRate, "/")
			if len(parts) == 2 {
				n, _ := strconv.ParseFloat(parts[0], 64)
				d, _ := strconv.ParseFloat(parts[1], 64)
				if d > 0 {
					fps = n / d
				}
			} else {
				fps, _ = strconv.ParseFloat(v.FrameRate, 64)
			}
			depth, _ := strconv.Atoi(v.Bits)
			if depth == 0 {
				switch v.Pixel {
				case "yuv420p", "yuv422p", "yuv444p", "nv12", "nv21", "gbrp":
					depth = 8
				}
				for _, n := range []int{9, 10, 12, 14, 16} {
					if strings.HasSuffix(v.Pixel, "p"+strconv.Itoa(n)+"le") || strings.HasSuffix(v.Pixel, "p"+strconv.Itoa(n)+"be") {
						depth = n
					}
				}
			}
			m.Video = playbackdecision.Video{Codec: v.CodecName, Profile: v.Profile, Level: v.Level, Width: v.Width, Height: v.Height, FPS: fps, BitDepth: depth, HDR: v.Transfer == "smpte2084" || v.Transfer == "arib-std-b67"}
		}
		if v.CodecType == "audio" && m.Audio == nil {
			rate, _ := strconv.Atoi(v.SampleRate)
			profile := v.Profile
			if profile == "" && v.CodecName != "aac" {
				profile = "default"
			}
			m.Audio = &playbackdecision.Audio{Codec: v.CodecName, Profile: profile, Channels: v.Channels, SampleRate: rate}
		}
	}
	return m
}
func v2Probe(r *http.Request, cfg *configs.Config, path, protocol, container string) (playbackdecision.Media, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tool, err := findFFprobePath(ctx, cfg)
	if err != nil {
		return playbackdecision.Media{}, err
	}
	p, err := probeMediaCached(ctx, tool, path, filepath.Join(cfg.AppDataPath, "media-probes"))
	if err != nil {
		return playbackdecision.Media{}, err
	}
	return v2MediaParameters(p, protocol, container), nil
}
func v2SessionFailure(session v2Session, code, msg string, retry, invalid bool) v2Session {
	session.DTO.Status = "failed"
	session.DTO.Error = v2Problem(code, msg, retry)
	session.DTO.RetryAfter = session.DTO.Error.RetryAfter
	session.Invalidated = invalid
	return session
}
func (s *v2Service) resolve(r *http.Request, session v2Session) v2Session {
	cfg := configs.GetCurrentConfig()
	if cfg == nil || cfg.OutPutPath != session.Root {
		return v2SessionFailure(session, "source_changed", "服务录播根目录已变化", false, true)
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		return v2SessionFailure(session, "catalog_unavailable", "录播目录未就绪", true, false)
	}
	status, err := cat.Status(r.Context())
	if err != nil || status != "verified" {
		return v2SessionFailure(session, "catalog_unavailable", "录播目录未核验或不可用", true, false)
	}
	record, err := cat.PlaybackRecording(r.Context(), session.DTO.RecordingID)
	if errors.Is(err, sql.ErrNoRows) || err == nil && record.SourceVersion != session.DTO.SourceVersion {
		return v2SessionFailure(session, "source_changed", "录播已删除或来源版本变化", false, true)
	}
	if err != nil {
		return v2SessionFailure(session, "catalog_unavailable", "目录查询失败", true, false)
	}
	if session.File != nil {
		current, err := cat.RebindPlaybackFile(r.Context(), *session.File)
		if err != nil {
			return v2SessionFailure(session, "source_changed", "所选媒体或来源已变化", false, true)
		}
		session.File = &current
		session.DTO.Status = "ready"
		session.DTO.Error = nil
		session.DTO.RetryAfter = 0
		return session
	}
	if session.HLSProfile != "" {
		asset, err := cat.FindHLS(r.Context(), session.DTO.RecordingID, session.DTO.AssetVersion)
		if err == nil {
			_, err = verifyHLSManifest(r, cat, asset)
		}
		if err != nil {
			return v2SessionFailure(session, "source_changed", "所选 HLS 或来源已变化", false, true)
		}
		session.DTO.Status = "ready"
		session.DTO.Error = nil
		session.DTO.RetryAfter = 0
		return session
	}
	files, err := cat.PlaybackFiles(r.Context(), session.DTO.RecordingID, session.DTO.SourceVersion)
	if err != nil {
		return v2SessionFailure(session, "catalog_unavailable", "无法读取可信候选资产", true, false)
	}
	if len(files) == 0 {
		return v2SessionFailure(session, "source_changed", "录播没有有效现存来源", false, true)
	}
	source := files[0]
	for _, f := range files {
		if f.Kind == "source" {
			source = f
			break
		}
	}
	if err := cat.VerifyPlaybackFile(r.Context(), source); err != nil {
		if errors.Is(err, recordingcatalog.ErrRecordingActive) {
			return v2SessionFailure(session, "recording_active", "来源仍在录制", true, false)
		}
		return v2SessionFailure(session, "source_changed", "来源文件已变化，不能使用旧版本", false, true)
	}
	media, err := v2Probe(r, cfg, source.Path, "file", "")
	if err != nil {
		return v2SessionFailure(session, "media_probe_failed", "媒体参数探测失败: "+err.Error(), true, false)
	}
	if err := cat.VerifyPlaybackFile(r.Context(), source); err != nil {
		return v2SessionFailure(session, "source_changed", "来源在探测期间变化", false, true)
	}
	session.DTO.Decision = playbackdecision.Decide(media, session.Capabilities).Mode
	sort.SliceStable(files, func(i, j int) bool { return files[i].Kind != "source" && files[j].Kind == "source" })
	for _, f := range files {
		if cat.VerifyPlaybackFile(r.Context(), f) != nil {
			continue
		}
		candidate, err := v2Probe(r, cfg, f.Path, "file", "")
		if err != nil || !playbackdecision.Decide(candidate, session.Capabilities).Compatible {
			continue
		}
		if cat.VerifyPlaybackFile(r.Context(), f) != nil {
			continue
		}
		chosen := f
		session.File = &chosen
		session.DTO.AssetVersion = f.AssetVersion
		session.DTO.Protocol = "file"
		session.MimeType = mimeTypeForVideoExtension("." + candidate.Container)
		session.DTO.Status = "ready"
		session.DTO.Error = nil
		session.DTO.RetryAfter = 0
		return session
	}
	for _, profile := range []string{"source", "1080p", "720p"} {
		version := session.DTO.SourceVersion + "-" + hlsasset.ProfileVersion(profile)
		asset, err := cat.FindHLS(r.Context(), session.DTO.RecordingID, version)
		if err != nil {
			continue
		}
		manifest, err := verifyHLSManifest(r, cat, asset)
		if err != nil {
			continue
		}
		segment := ""
		for _, file := range manifest.Files {
			if strings.HasSuffix(file.Name, ".ts") {
				segment = file.Name
				break
			}
		}
		if segment == "" {
			continue
		}
		f, err := manifest.OpenFile(filepath.Dir(asset.Path), segment)
		if err != nil {
			continue
		}
		f.Close()
		candidate, err := v2Probe(r, cfg, manifest.FilePath(filepath.Dir(asset.Path), segment), "hls", "hls")
		if err != nil || !playbackdecision.Decide(candidate, session.Capabilities).Compatible {
			continue
		}
		f, err = manifest.OpenFile(filepath.Dir(asset.Path), segment)
		if err != nil {
			continue
		}
		f.Close()
		if _, err := verifyHLSManifest(r, cat, asset); err != nil {
			continue
		}
		session.HLSProfile = profile
		session.DTO.AssetVersion = version
		session.DTO.Protocol = "hls"
		session.DTO.Status = "ready"
		session.DTO.Error = nil
		session.DTO.RetryAfter = 0
		return session
	}
	profiles := v2PossiblePreparations(media, session.Capabilities)
	if len(profiles) == 0 {
		return v2SessionFailure(session, "no_matching_capabilities", "有限准备档位无法满足此设备能力", false, false)
	}
	inst := instance.GetInstance(r.Context())
	if inst != nil {
		if manager := pipeline.GetManager(inst); manager != nil {
			var failed, paused *pipeline.PreparationJob
			for _, profile := range profiles {
				job, err := manager.PreparationStatus(r.Context(), pipeline.PreparationKey{RecordingID: session.DTO.RecordingID, SourceVersion: session.DTO.SourceVersion, ProfileVersion: profile})
				if errors.Is(err, sql.ErrNoRows) || err == nil && (job.Status == pipeline.PreparationFailed || job.Status == pipeline.PreparationPaused) {
					stageName := pipeline.StageNamePreparePlayback
					if profile != stages.PreparationProfileVersion {
						stageName = pipeline.StageNamePrepareHLS
					}
					status, queuedErr := manager.ScheduledPreparationStatus(r.Context(), source.Path, session.DTO.SourceVersion, stageName, profile)
					if queuedErr == nil {
						job.Status, err = status, nil
					} else if !errors.Is(queuedErr, sql.ErrNoRows) {
						return v2SessionFailure(session, "preparation_unavailable", "准备队列查询失败", true, false)
					}
				}
				if errors.Is(err, sql.ErrNoRows) {
					continue
				}
				if err != nil {
					return v2SessionFailure(session, "preparation_unavailable", "准备状态查询失败", true, false)
				}
				switch job.Status {
				case pipeline.PreparationQueued, pipeline.PreparationRunning:
					session.DTO.Status = "processing"
					session.DTO.Error = nil
					session.DTO.RetryAfter = 2
					return session
				case pipeline.PreparationPaused:
					copy := job
					paused = &copy
				case pipeline.PreparationFailed:
					copy := job
					failed = &copy
				}
			}
			if paused != nil {
				session.DTO.Status = "paused"
				session.DTO.Error = v2Problem("preparation_paused", "视频准备已暂停，请检查服务器可用空间或稍后重试", true)
				session.DTO.RetryAfter = 2
				return session
			}
			if failed != nil {
				// 原始诊断保留在持久任务中，播放接口仅返回可操作的用户提示。
				return v2SessionFailure(session, "preparation_failed", "视频准备失败，请在任务列表重试；原始录像已保留", true, false)
			}
		}
	}
	return v2SessionFailure(session, "asset_not_ready", "这段录像尚未完成播放准备，请先在服务器准备该录像", true, false)
}

// 这里只判断有限策略是否可能满足能力，不宣称尚未生成的文件已兼容。
func v2PossiblePreparations(source playbackdecision.Media, caps playbackdecision.Capabilities) []string {
	baseline := playbackdecision.Capabilities{Containers: []string{"mp4", "flv", "mkv", "mov", "ts"}, Protocols: []string{"file"}, Video: []playbackdecision.VideoCapability{{Codec: "h264", Profiles: []string{"baseline", "main", "high"}, MaxLevel: 42, MaxWidth: 1920, MaxHeight: 1080, MaxFPS: 60, MaxBitDepth: 8}}, Audio: []playbackdecision.AudioCapability{{Codec: "aac", Profiles: []string{"lc"}, MaxChannels: 2, MaxSampleRate: 48000}}}
	expected := source
	expected.Container = "mp4"
	expected.Protocol = "file"
	mode := playbackdecision.Decide(source, baseline).Mode
	if mode == "video-transcode" {
		expected.Video = playbackdecision.Video{Codec: "h264", Profile: "main", Level: 41, Width: min(source.Video.Width, 1920), Height: min(source.Video.Height, 1080), FPS: 30, BitDepth: 8}
	}
	if source.Audio != nil && (mode == "audio-transcode" || !playbackdecision.Decide(playbackdecision.Media{Container: "mp4", Protocol: "file", Video: expected.Video, Audio: source.Audio}, baseline).Compatible) {
		expected.Audio = &playbackdecision.Audio{Codec: "aac", Profile: "lc", Channels: 2, SampleRate: 48000}
	}
	var result []string
	if playbackdecision.Decide(expected, caps).Compatible {
		result = append(result, stages.PreparationProfileVersion)
	}
	for _, profile := range []string{"source", "1080p", "720p"} {
		hls := expected
		hls.Container = "hls"
		hls.Protocol = "hls"
		if profile != "source" {
			width, height := 1920, 1080
			if profile == "720p" {
				width, height = 1280, 720
			}
			hls.Video = playbackdecision.Video{Codec: "h264", Profile: "main", Level: 41, Width: min(source.Video.Width, width), Height: min(source.Video.Height, height), FPS: 30, BitDepth: 8}
			if source.Audio != nil {
				hls.Audio = &playbackdecision.Audio{Codec: "aac", Profile: "lc", Channels: 2, SampleRate: 48000}
			}
		}
		if playbackdecision.Decide(hls, caps).Compatible {
			result = append(result, hlsasset.ProfileVersion(profile))
		}
	}
	return result
}
