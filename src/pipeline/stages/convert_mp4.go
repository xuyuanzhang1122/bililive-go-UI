package stages

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
	"github.com/bililive-go/bililive-go/src/pkg/playbackasset"
	"github.com/bililive-go/bililive-go/src/pkg/utils"
	"github.com/sirupsen/logrus"
)

// ConvertMp4Stage MP4 转换阶段
type ConvertMp4Stage struct {
	config       pipeline.StageConfig
	deleteSource bool
	commands     []string
	logs         string
}

// NewConvertMp4Stage 创建 MP4 转换阶段工厂
func NewConvertMp4Stage(config pipeline.StageConfig) (pipeline.Stage, error) {
	deleteSource := config.GetBoolOption(pipeline.OptionDeleteSource, false)
	return &ConvertMp4Stage{
		config:       config,
		deleteSource: deleteSource,
	}, nil
}

func (s *ConvertMp4Stage) Name() string {
	return pipeline.StageNameConvertMp4
}

func (s *ConvertMp4Stage) Execute(ctx *pipeline.PipelineContext, input []pipeline.FileInfo) ([]pipeline.FileInfo, error) {
	if len(input) == 0 {
		s.logs = "没有输入文件"
		return input, nil
	}

	limit := 3
	if cfg := configs.GetCurrentConfig(); cfg != nil && cfg.HLSCache.MaxConcurrent > 0 {
		limit = cfg.HLSCache.MaxConcurrent
	}
	release, err := mediawork.Shared.Acquire(ctx.Ctx, limit)
	if err != nil {
		return nil, err
	}
	defer release()

	ffmpegPath := ctx.FFmpegPath
	if ffmpegPath == "" {
		var err error
		ffmpegPath, err = utils.GetFFmpegPath(ctx.Ctx)
		if err != nil {
			s.logs = fmt.Sprintf("ffmpeg 不可用: %s", err.Error())
			return nil, fmt.Errorf("ffmpeg not available: %w", err)
		}
	}

	var output []pipeline.FileInfo

	for _, file := range input {
		// 只处理视频文件
		if file.Type != pipeline.FileTypeVideo {
			output = append(output, file)
			continue
		}

		sourceInfo, err := os.Stat(file.Path)
		if os.IsNotExist(err) && s.deleteSource {
			ext := filepath.Ext(file.Path)
			target := strings.TrimSuffix(file.Path, ext) + ".mp4"
			if strings.EqualFold(ext, ".mp4") {
				target = strings.TrimSuffix(file.Path, ext) + ".prepared.mp4"
			}
			manifest, loadErr := playbackasset.Load(target)
			if loadErr == nil && manifest.SourceName == filepath.Base(file.Path) {
				if _, err := inspectMP4Media(ctx.Ctx, ffmpegPath, target); err != nil {
					return nil, err
				}
				if err := checkMP4Faststart(target); err != nil {
					return nil, err
				}
				if _, err := playbackasset.RecoverSourceDeletion(target); err != nil {
					return nil, err
				}
				output = append(output, pipeline.FileInfo{Path: target, Type: pipeline.FileTypeVideo, SourcePath: file.Path})
				continue
			}
		}
		if err != nil {
			return nil, fmt.Errorf("读取源文件: %w", err)
		}
		if !sourceInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("源文件不是普通文件: %s", file.Path)
		}
		sourceMedia, err := inspectMP4Media(ctx.Ctx, ffmpegPath, file.Path)
		if err != nil {
			return nil, fmt.Errorf("检查源媒体: %w", err)
		}
		ext := filepath.Ext(file.Path)
		outputPath := strings.TrimSuffix(file.Path, ext) + ".mp4"
		if strings.EqualFold(ext, ".mp4") {
			if err := checkMP4Faststart(file.Path); err == nil {
				output = append(output, file)
				continue
			}
			outputPath = strings.TrimSuffix(file.Path, ext) + ".prepared.mp4"
		}
		validateOutput := func(path string) error {
			result, err := inspectMP4Media(ctx.Ctx, ffmpegPath, path)
			if err != nil {
				return err
			}
			if result.audio != sourceMedia.audio || math.Abs(result.duration-sourceMedia.duration) > math.Max(1, sourceMedia.duration*0.01) {
				return fmt.Errorf("产物轨道或时长不符")
			}
			return checkMP4Faststart(path)
		}
		if _, err := playbackasset.RecoverPublication(file.Path, outputPath, sourceInfo, validateOutput); err != nil {
			return nil, fmt.Errorf("恢复发布失败: %w", err)
		}
		reused := false
		if _, err := os.Lstat(outputPath); err == nil {
			manifest, loadErr := playbackasset.Load(outputPath)
			if loadErr != nil || manifest.SourceName != filepath.Base(file.Path) {
				return nil, fmt.Errorf("目标文件已存在且来源无法确认: %s", outputPath)
			}
			result, checkErr := inspectMP4Media(ctx.Ctx, ffmpegPath, outputPath)
			if checkErr != nil {
				return nil, checkErr
			}
			if result.audio != sourceMedia.audio || math.Abs(result.duration-sourceMedia.duration) > math.Max(1, sourceMedia.duration*0.01) {
				return nil, fmt.Errorf("已有产物轨道或时长不符")
			}
			if err := checkMP4Faststart(outputPath); err != nil {
				return nil, err
			}
			reused = true
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		if !reused {
			temp, err := os.CreateTemp(filepath.Dir(outputPath), ".converting-*.mp4")
			if err != nil {
				return nil, fmt.Errorf("创建临时文件: %w", err)
			}
			tempFile := temp.Name()
			if err := temp.Close(); err != nil {
				os.Remove(tempFile)
				return nil, err
			}
			// 只清理本次创建的临时文件，任何失败都保留源文件。
			err = func() error {
				defer os.Remove(tempFile)
				args := []string{"-nostdin", "-v", "error", "-i", file.Path, "-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-movflags", "+faststart", "-y", "-progress", "pipe:1", tempFile}
				s.commands = append(s.commands, fmt.Sprintf("%s %s", ffmpegPath, strings.Join(args, " ")))
				cmd := exec.CommandContext(ctx.Ctx, ffmpegPath, args...)
				stderr := &mp4LimitedBuffer{}
				cmd.Stderr = stderr
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					return err
				}
				if err := cmd.Start(); err != nil {
					stdout.Close()
					return err
				}
				// 读至 EOF 后再 Wait，避免 Wait 提前关闭管道；取消由 CommandContext 回收。
				s.parseProgress(ctx.Ctx, stdout, sourceMedia.duration)
				// 进度行过长导致 scanner 停止时仍排空管道，避免子进程阻塞。
				_, _ = io.Copy(io.Discard, stdout)
				if err := cmd.Wait(); err != nil {
					return fmt.Errorf("ffmpeg 转换失败: %w: %s", err, stderr.String())
				}
				if err := ctx.Ctx.Err(); err != nil {
					return err
				}
				result, err := inspectMP4Media(ctx.Ctx, ffmpegPath, tempFile)
				if err != nil {
					return fmt.Errorf("产物检查失败: %w", err)
				}
				if result.audio != sourceMedia.audio || math.Abs(result.duration-sourceMedia.duration) > math.Max(1, sourceMedia.duration*0.01) {
					return fmt.Errorf("产物轨道或时长与源文件不符")
				}
				if err := checkMP4Faststart(tempFile); err != nil {
					return err
				}
				current, err := os.Stat(file.Path)
				if err != nil {
					return err
				}
				if !os.SameFile(sourceInfo, current) || sourceInfo.Size() != current.Size() || !sourceInfo.ModTime().Equal(current.ModTime()) {
					return fmt.Errorf("转换期间源文件发生变化")
				}
				ready, err := os.OpenFile(tempFile, os.O_RDWR, 0)
				if err != nil {
					return err
				}
				syncErr := ready.Sync()
				closeErr := ready.Close()
				if syncErr != nil {
					return syncErr
				}
				if closeErr != nil {
					return closeErr
				}
				if err := playbackasset.PublishValidated(file.Path, outputPath, tempFile, sourceInfo); err != nil {
					return err
				}
				return nil
			}()
			if err != nil {
				s.logs += err.Error() + "\n"
				return nil, err
			}

		}

		// 添加输出文件
		output = append(output, pipeline.FileInfo{
			Path:       outputPath,
			Type:       pipeline.FileTypeVideo,
			SourcePath: file.Path,
		})

		// 删除原始文件
		if s.deleteSource && file.Path != outputPath {
			if err := playbackasset.DeleteSourceValidated(file.Path, outputPath, sourceInfo); err != nil {
				logrus.WithError(err).WithField("file", file.Path).Warn("failed to delete original file")
				s.logs += fmt.Sprintf("删除原始文件失败: %s\n", file.Path)
				return nil, err
			} else {
				s.logs += fmt.Sprintf("已删除原始文件: %s\n", file.Path)
				ctx.Logger.Infof("已删除原始文件: %s", file.Path)
			}
		} else {
			// 保留原始文件在输出中
			if !s.deleteSource {
				output = append(output, file)
			}
		}

		s.logs += fmt.Sprintf("转换完成: %s -> %s\n", filepath.Base(file.Path), filepath.Base(outputPath))
		ctx.Logger.Infof("MP4 转换完成: %s", outputPath)
	}

	return output, nil
}

// parseProgress 解析 ffmpeg 进度输出
func (s *ConvertMp4Stage) parseProgress(ctx context.Context, stdout io.Reader, totalDuration float64) {
	scanner := bufio.NewScanner(stdout)
	re := regexp.MustCompile(`out_time_us=(\d+)`)

	for scanner.Scan() {

		line := scanner.Text()
		matches := re.FindStringSubmatch(line)
		if len(matches) >= 2 && totalDuration > 0 {
			timeUs, _ := strconv.ParseFloat(matches[1], 64)
			currentTime := timeUs / 1000000
			progress := (currentTime / totalDuration) * 100
			_ = progress // 可以通过回调上报进度
		}
	}
}

func (s *ConvertMp4Stage) GetCommands() []string {
	return s.commands
}

func (s *ConvertMp4Stage) GetLogs() string {
	return s.logs
}
