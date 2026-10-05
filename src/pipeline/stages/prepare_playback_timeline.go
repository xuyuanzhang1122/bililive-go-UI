package stages

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type preparationTrackTimeline struct {
	count int64
	start float64
	end   float64
	last  float64
}

type preparationPacketTimeline struct {
	video preparationTrackTimeline
	audio preparationTrackTimeline
}

// 仅在损坏 FLV 的来源探测中使用：逐包扫描，避免长录播的包列表占满内存。
func probePreparationTimeline(ctx context.Context, probe, path string, videoIndex, audioIndex int, hasAudio bool) (preparationPacketTimeline, string, error) {
	var timeline preparationPacketTimeline
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe, "-v", "error", "-show_packets", "-show_entries", "packet=stream_index,pts_time,dts_time,duration_time", "-of", "compact=p=0:nk=0", path)
	stderr := &mp4LimitedBuffer{}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return timeline, "", err
	}
	if err := cmd.Start(); err != nil {
		return timeline, "", err
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		packet := make(map[string]string, 4)
		for _, field := range strings.Split(scanner.Text(), "|") {
			key, value, ok := strings.Cut(field, "=")
			if ok {
				packet[key] = value
			}
		}
		index, err := strconv.Atoi(packet["stream_index"])
		if err != nil {
			continue // 其他流或 FFprobe 附加字段不影响选中轨道。
		}
		var track *preparationTrackTimeline
		switch {
		case index == videoIndex:
			track = &timeline.video
		case hasAudio && index == audioIndex:
			track = &timeline.audio
		default:
			continue
		}
		if err := track.add(packet); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return timeline, stderr.String(), fmt.Errorf("轨道 %d 包时间戳异常: %w", index, err)
		}
	}
	if err := scanner.Err(); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return timeline, stderr.String(), fmt.Errorf("扫描包时间线失败: %w", err)
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return timeline, stderr.String(), ctx.Err()
		}
		return timeline, stderr.String(), fmt.Errorf("ffprobe 扫描包失败: %w: %s", err, stderr.String())
	}
	if timeline.video.count < 2 || timeline.video.end <= timeline.video.start || !finitePreparationTime(timeline.video.end) {
		return timeline, stderr.String(), fmt.Errorf("缺少连续有效的视频包时间线")
	}
	if hasAudio && (timeline.audio.count < 1 || !finitePreparationTime(timeline.audio.end) || math.Abs(timeline.video.start-timeline.audio.start) > 2 || math.Abs(timeline.video.end-timeline.audio.end) > 2) {
		return timeline, stderr.String(), fmt.Errorf("音视频时间线不一致")
	}
	return timeline, stderr.String(), nil
}

func (track *preparationTrackTimeline) add(packet map[string]string) error {
	dts, dtsOK := parsePreparationTime(packet["dts_time"])
	pts, ptsOK := parsePreparationTime(packet["pts_time"])
	if !dtsOK && !ptsOK {
		return fmt.Errorf("缺少有效 PTS/DTS")
	}
	if !dtsOK {
		dts = pts
	}
	if !ptsOK {
		pts = dts
	}
	if math.Abs(pts-dts) > 60 {
		return fmt.Errorf("PTS/DTS 偏移超过 60 秒")
	}
	duration, ok := parsePreparationTime(packet["duration_time"])
	if !ok || duration < 0 {
		duration = 0
	}
	if track.count == 0 {
		track.start = dts
		track.end = pts + duration
	} else {
		if dts < track.last-0.001 || dts-track.last > 60 {
			return fmt.Errorf("解码时间戳倒退或间隔超过 60 秒")
		}
		track.end = math.Max(track.end, pts+duration)
	}
	if !finitePreparationTime(track.end) {
		return fmt.Errorf("结束时间无效")
	}
	track.count++
	track.last = dts
	return nil
}

func parsePreparationTime(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(value, 64)
	return parsed, err == nil && finitePreparationTime(parsed)
}

func finitePreparationTime(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
