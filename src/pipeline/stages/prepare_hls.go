package stages

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/hlsasset"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/bililive-go/bililive-go/src/pkg/migration"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
)

type PrepareHLSStage struct {
	profiles []string
	runner   *PreparePlaybackStage
}

func NewPrepareHLSStage(cfg pipeline.StageConfig) (pipeline.Stage, error) {
	profiles := cfg.GetStringSliceOption("profiles")
	if len(profiles) == 0 {
		return nil, fmt.Errorf("持久 HLS 缺少档位")
	}
	if err := migration.ValidateHLSProfiles(profiles); err != nil {
		return nil, err
	}
	return &PrepareHLSStage{profiles: append([]string(nil), profiles...), runner: &PreparePlaybackStage{}}, nil
}
func (s *PrepareHLSStage) Name() string { return pipeline.StageNamePrepareHLS }
func (s *PrepareHLSStage) GetCommands() []string {
	if s.runner == nil {
		return nil
	}
	return s.runner.commands
}
func (s *PrepareHLSStage) GetLogs() string {
	if s.runner == nil {
		return ""
	}
	return s.runner.logs
}
func (s *PrepareHLSStage) Execute(ctx *pipeline.PipelineContext, input []pipeline.FileInfo) ([]pipeline.FileInfo, error) {
	if err := ctx.Ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Catalog == nil || ctx.Preparations == nil {
		return nil, fmt.Errorf("HLS 准备缺少目录或任务存储")
	}
	if err := migration.ValidateHLSProfiles(s.profiles); err != nil {
		return nil, err
	}
	if s.runner == nil {
		s.runner = &PreparePlaybackStage{}
	}
	for _, file := range input {
		if file.Type == pipeline.FileTypeVideo {
			for _, profile := range s.profiles {
				if err := s.prepare(ctx, file, profile); err != nil {
					return nil, err
				}
			}
		}
	}
	return input, nil
}
func (s *PrepareHLSStage) prepare(ctx *pipeline.PipelineContext, file pipeline.FileInfo, profile string) (resultErr error) {
	entry, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion)
	if errors.Is(err, recordingcatalog.ErrCatalogPending) || errors.Is(err, recordingcatalog.ErrRecordingActive) {
		ctx.Catalog.Notify()
		return &pipeline.PreparationPausedError{Reason: err.Error()}
	}
	if err != nil {
		return err
	}
	profileVersion := hlsasset.ProfileVersion(profile)
	key := pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: profileVersion}
	job, err := ctx.Preparations.QueuePreparation(ctx.Ctx, key)
	if err != nil {
		return err
	}
	assetVersion := entry.SourceVersion + "-" + profileVersion
	dir := filepath.Join(ctx.Catalog.AssetRoot(), entry.RecordingID, assetVersion)
	expected := hlsasset.Manifest{Version: 1, RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, SourcePath: file.Path, Profile: profile}
	if job.Status == pipeline.PreparationReady {
		m, ok, e := hlsasset.Recover(dir, expected)
		asset, ae := ctx.Catalog.FindHLS(ctx.Ctx, entry.RecordingID, assetVersion)
		if e == nil && ok && m.Digest() == job.OutputVersion && job.OutputPath == filepath.Join(dir, "index.m3u8") && ae == nil && asset.Path == job.OutputPath {
			current, checkErr := recordingcatalog.SnapshotVersion(asset.Path)
			if checkErr == nil && current == asset.FileVersion && asset.ManifestDigest == m.Digest() {
				return nil
			}
		}
		if e := ctx.Preparations.InvalidatePreparation(ctx.Ctx, key); e != nil {
			return e
		}
		if e := ctx.Catalog.InvalidatePreparedAsset(ctx.Ctx, entry, filepath.Join(dir, "index.m3u8")); e != nil {
			return e
		}
		return fmt.Errorf("已完成持久 HLS 失效，拒绝复用或覆盖")
	}
	if err := ctx.Preparations.ClaimPreparation(ctx.Ctx, key); err != nil {
		return err
	}
	output, digest := "", ""
	defer func() {
		status, reason := pipeline.PreparationReady, ""
		if resultErr != nil {
			status = pipeline.PreparationFailed
			reason = resultErr.Error()
			var paused *pipeline.PreparationPausedError
			if errors.As(resultErr, &paused) || ctx.Ctx.Err() != nil {
				status = pipeline.PreparationPaused
			}
			if err := ctx.Catalog.InvalidatePreparedAsset(context.WithoutCancel(ctx.Ctx), entry, filepath.Join(dir, "index.m3u8")); err != nil {
				resultErr = errors.Join(resultErr, err)
			}
		}
		if err := ctx.Preparations.FinishPreparation(context.WithoutCancel(ctx.Ctx), key, status, output, reason, digest); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}()
	limit, reserve := 3, uint64(1_000_000_000)
	if cfg := configs.GetCurrentConfig(); cfg != nil {
		if cfg.HLSCache.MaxConcurrent > 0 {
			limit = cfg.HLSCache.MaxConcurrent
		}
		reserve = uint64(cfg.HLSCache.MinFreeSpaceGB * 1e9)
	}
	release, err := mediawork.Shared.Acquire(ctx.Ctx, limit)
	if err != nil {
		return err
	}
	defer release()
	if err := safePreparationDir(ctx.Catalog.AssetRoot(), entry.RecordingID, assetVersion); err != nil {
		return err
	}
	m, exists, err := hlsasset.Recover(dir, expected)
	if err != nil {
		return err
	}
	if !exists {
		if err := hlsasset.CleanAbandoned(dir, expected); err != nil {
			return &pipeline.PreparationPausedError{Reason: "核验未发布 HLS 构建失败: " + err.Error()}
		}
		ffmpeg := ctx.FFmpegPath
		if ffmpeg == "" {
			ffmpeg, err = utils.GetFFmpegPath(ctx.Ctx)
			if err != nil {
				return err
			}
		}
		source := file.Path
		prepared, err := ctx.Preparations.QueuePreparation(ctx.Ctx, pipeline.PreparationKey{RecordingID: entry.RecordingID, SourceVersion: entry.SourceVersion, ProfileVersion: PreparationProfileVersion})
		if err != nil {
			return err
		}
		if prepared.Status == pipeline.PreparationReady {
			if err := ctx.Catalog.VerifyPreparedAsset(ctx.Ctx, entry, prepared.OutputPath, prepared.OutputVersion, PreparationProfileVersion); err != nil {
				return err
			}
			source = prepared.OutputPath
		}
		sourceVersion, err := recordingcatalog.SnapshotVersion(source)
		if err != nil {
			return err
		}
		media, err := probePreparation(ctx.Ctx, ffmpeg, source)
		if err != nil {
			return err
		}
		if media.mode() != "copy" {
			return fmt.Errorf("持久 HLS 需要先完成兼容播放准备")
		}
		free, err := s.runner.free(dir)
		if err != nil || float64(free) < float64(reserve)+math.Max(float64(media.duration)*800_000, 1_000_000) {
			return &pipeline.PreparationPausedError{Reason: "持久 HLS 可用空间不足或核验失败，源已保留"}
		}
		data, err := hlsasset.NewBuild(dir, expected)
		if err != nil {
			return err
		}
		keep := false
		defer func() {
			if !keep {
				_ = os.RemoveAll(data)
			}
		}()
		args := hlsPreparationArgs(source, filepath.Join(data, "index.m3u8"), profile, media)
		s.runner.commands = append(s.runner.commands, ffmpeg+" "+strings.Join(args, " "))
		if err := s.runner.run(ctx.Ctx, ffmpeg, args, dir, reserve); err != nil {
			return err
		}
		after, err := recordingcatalog.SnapshotVersion(source)
		if err != nil {
			return err
		}
		if after != sourceVersion {
			return fmt.Errorf("HLS 准备期间媒体来源变化")
		}
		if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
			return err
		}
		expected.Duration = media.duration
		m, err = hlsasset.Seal(data, expected)
		if err != nil {
			return err
		}
		// 实际解封装所有包并核验输出 profile、音轨与时长，再提交集合。
		inspected, err := probePreparation(ctx.Ctx, ffmpeg, filepath.Join(data, "index.m3u8"))
		if err != nil {
			return err
		}
		if inspected.mode() != "copy" || inspected.audio != media.audio || math.Abs(inspected.duration-media.duration) > math.Max(1, media.duration*0.01) {
			return fmt.Errorf("HLS 产物媒体核验失败")
		}
		if profile == "720p" && (inspected.video.Width > 1280 || inspected.video.Height > 720) {
			return fmt.Errorf("HLS 720p 档位越界")
		}
		if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
			return err
		}
		keep = true // 发布意图可能已经持久化，后续失败保留恢复数据。
		if err := hlsasset.Publish(dir, m, nil); err != nil {
			return err
		}
	}
	if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
		return err
	}
	if err := m.ValidateAll(dir); err != nil {
		return err
	}
	proofs := make(map[string]string)
	for _, f := range m.Files {
		path := m.FilePath(dir, f.Name)
		version, err := recordingcatalog.SnapshotVersion(path)
		if err != nil {
			return err
		}
		proofs[path] = version
	}
	if err := ctx.Catalog.RegisterHLS(ctx.Ctx, entry, profileVersion, m.Digest(), proofs); err != nil {
		return err
	}
	if err := ctx.Catalog.Reconcile(ctx.Ctx); err != nil {
		return &pipeline.PreparationPausedError{Reason: err.Error()}
	}
	if err := m.ValidateAll(dir); err != nil {
		return err
	}
	if _, err := ctx.Catalog.PreparationEntry(ctx.Ctx, file.Path, file.PreparationSourceVersion); err != nil {
		return err
	}
	output = filepath.Join(dir, "index.m3u8")
	digest = m.Digest()
	s.runner.logs += "持久 HLS 已准备 " + profile + "\n"
	return nil
}

func hlsPreparationArgs(source, playlist, profile string, media preparationMedia) []string {
	args := []string{"-nostdin", "-v", "error", "-i", source, "-map", "0:v:0", "-map", "0:a:0?"}
	if profile == "source" {
		args = append(args, "-c:v", "copy", "-c:a", "copy")
	} else {
		width, height, rate, buf := "1920", "1080", "6M", "12M"
		if profile == "720p" {
			width, height, rate, buf = "1280", "720", "3M", "6M"
		}
		filter := "scale=w='min(" + width + ",iw)':h='min(" + height + ",ih)':force_original_aspect_ratio=decrease:force_divisible_by=2"
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-profile:v", "main", "-level:v", "4.1", "-pix_fmt", "yuv420p", "-vf", filter, "-r", "30", "-g", "180", "-keyint_min", "180", "-sc_threshold", "0", "-maxrate", rate, "-bufsize", buf, "-c:a", "aac", "-b:a", "128k", "-ac", "2", "-ar", "48000")
	}
	return append(args, "-f", "hls", "-hls_time", "6", "-hls_list_size", "0", "-hls_playlist_type", "vod", "-hls_flags", "temp_file", "-hls_segment_filename", filepath.Join(filepath.Dir(playlist), "seg_%05d.ts"), playlist)
}
