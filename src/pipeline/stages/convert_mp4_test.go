package stages

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/bililive-go/bililive-go/src/pkg/mediawork"
)

func mp4TestContext(ffmpeg string) *pipeline.PipelineContext {
	return &pipeline.PipelineContext{Ctx: context.Background(), FFmpegPath: ffmpeg, Logger: livelogger.New(10, nil)}
}
func mp4FakeConverter(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "ffmpeg")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nfor last; do :; done\nprintf 'invalid media' > \"$last\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestConvertMP4PublishesOnlyValidatedOutput(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	mp4Fixture(t, source)
	s := &ConvertMp4Stage{}
	_, err := s.Execute(mp4TestContext(mp4FakeConverter(t, dir)), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil {
		t.Fatal("无效产物不应发布成功")
	}
	if _, err := os.Stat(filepath.Join(dir, "source.mp4")); !os.IsNotExist(err) {
		t.Fatal("无效产物已发布")
	}
}
func TestConvertMP4FailureKeepsSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	os.WriteFile(source, []byte("broken mp4"), 0600)
	s := &ConvertMp4Stage{deleteSource: true}
	_, err := s.Execute(mp4TestContext(mp4FakeConverter(t, dir)), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil {
		t.Fatal("损坏 MP4 不能按扩展名跳过")
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != "broken mp4" {
		t.Fatal("失败必须保留源文件")
	}
}
func TestConvertMP4DoesNotOverwriteExistingAsset(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	target := filepath.Join(dir, "source.mp4")
	mp4Fixture(t, source)
	os.WriteFile(target, []byte("other recording"), 0600)
	s := &ConvertMp4Stage{deleteSource: true}
	_, err := s.Execute(mp4TestContext(mp4FakeConverter(t, dir)), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil {
		t.Fatal("目标冲突必须失败")
	}
	data, _ := os.ReadFile(target)
	if string(data) != "other recording" {
		t.Fatal("已有录播被覆盖")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("冲突必须保留源文件")
	}
}
func TestConvertMP4RealRemux(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("未安装 ffprobe")
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-f", "lavfi", "-i", "sine=frequency=1000", "-t", "1", "-c:v", "libx264", "-c:a", "aac", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("样本生成: %v %s", err, out)
	}
	s := &ConvertMp4Stage{}
	out, err := s.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].SourcePath != source {
		t.Fatalf("关联或保源错误: %+v", out)
	}
	probe := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_name", "-of", "csv=p=0", out[0].Path)
	data, err := probe.Output()
	if err != nil || string(data) != "h264\naac\n" {
		t.Fatalf("编码未保留: %s %v", data, err)
	}
}

func mp4Fixture(t *testing.T, source string) {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("未安装 ffmpeg")
	}
	cmd := exec.Command(ffmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=64x64:rate=10", "-t", "1", "-c:v", "libx264", source)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("样本生成: %v %s", err, out)
	}
}
func TestConvertMP4RetryReusesValidatedAsset(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	mp4Fixture(t, source)
	ffmpeg, _ := exec.LookPath("ffmpeg")
	s := &ConvertMp4Stage{}
	first, err := s.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil {
		t.Fatal(err)
	}
	stamp, _ := os.Stat(first[0].Path)
	second := &ConvertMp4Stage{}
	out, err := second.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil {
		t.Fatalf("重试失败: %v", err)
	}
	current, _ := os.Stat(out[0].Path)
	if !os.SameFile(stamp, current) || len(second.commands) != 0 {
		t.Fatal("重试不应再次转换")
	}
}
func TestConvertMP4OptimizesExistingMP4(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp4")
	mp4Fixture(t, source)
	ffmpeg, _ := exec.LookPath("ffmpeg")
	original, _ := os.ReadFile(source)
	s := &ConvertMp4Stage{}
	out, err := s.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Path == source {
		t.Fatal("非 faststart MP4 不应跳过")
	}
	current, _ := os.ReadFile(source)
	if string(current) != string(original) {
		t.Fatal("不应原位改写源文件")
	}
	if err := checkMP4Faststart(out[0].Path); err != nil {
		t.Fatal(err)
	}
}

func TestConvertMP4PublicationRaceKeepsExistingAsset(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	target := filepath.Join(dir, "source.mp4")
	mp4Fixture(t, source)
	ffmpeg, _ := exec.LookPath("ffmpeg")
	wrapper := filepath.Join(dir, "converter")
	script := "#!/bin/sh\nprintf 'other recording' > '" + target + "'\nexec '" + ffmpeg + "' \"$@\"\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	s := &ConvertMp4Stage{deleteSource: true}
	_, err := s.Execute(mp4TestContext(wrapper), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil {
		t.Fatal("发布竞态必须失败")
	}
	content, _ := os.ReadFile(target)
	if string(content) != "other recording" {
		t.Fatal("已有文件被覆盖")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("源文件被删除")
	}
	matches, _ := filepath.Glob(filepath.Join(dir, ".converting-*.mp4"))
	if len(matches) != 0 {
		t.Fatal("失败临时文件未清理")
	}
}
func TestConvertMP4ManifestFailureKeepsSource(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	mp4Fixture(t, source)
	target := filepath.Join(dir, "source.mp4")
	if err := os.WriteFile(target+".playback.json", []byte("conflicting manifest"), 0600); err != nil {
		t.Fatal(err)
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	s := &ConvertMp4Stage{deleteSource: true}
	_, err := s.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil {
		t.Fatal("关联失败必须返回错误")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("关联失败不得删除源文件")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("关联失败应回滚本次产物")
	}
}

func TestConvertMP4RetryAfterSourceDeletion(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	mp4Fixture(t, source)
	ffmpeg, _ := exec.LookPath("ffmpeg")
	first := &ConvertMp4Stage{deleteSource: true}
	_, err := first.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil {
		t.Fatal(err)
	}
	retry := &ConvertMp4Stage{deleteSource: true}
	out, err := retry.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err != nil || len(out) != 1 {
		t.Fatalf("已安全删除源后的重试不能失败: %+v %v", out, err)
	}
}

func TestConvertMP4CancellationReapsProcess(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	mp4Fixture(t, source)
	pidPath := filepath.Join(dir, "pid")
	wrapper := filepath.Join(dir, "converter")
	script := "#!/bin/sh\nprintf '%s' \"$$\" > '" + pidPath + "'\nexec sleep 60\n"
	if err := os.WriteFile(wrapper, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pipelineCtx := mp4TestContext(wrapper)
	pipelineCtx.Ctx = ctx
	s := &ConvertMp4Stage{deleteSource: true}
	started := time.Now()
	_, err := s.Execute(pipelineCtx, []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("取消未及时结束: %v", err)
	}
	pid, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal("未运行转换进程", err)
	}
	if err := exec.Command("kill", "-0", strings.TrimSpace(string(pid))).Run(); err == nil {
		t.Fatal("转换进程尚未回收")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("取消删除了源文件")
	}
}

func TestConvertMP4WaitsForSharedBudget(t *testing.T) {
	cfg := &configs.Config{}
	cfg.HLSCache.MaxConcurrent = 1
	previous := configs.GetCurrentConfig()
	configs.SetCurrentConfig(cfg)
	defer configs.SetCurrentConfig(previous)
	release, err := mediawork.Shared.Acquire(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	dir := t.TempDir()
	source := filepath.Join(dir, "source.flv")
	os.WriteFile(source, []byte("source"), 0600)
	marker := filepath.Join(dir, "probe-started")
	os.WriteFile(filepath.Join(dir, "ffprobe"), []byte("#!/bin/sh\ntouch '"+marker+"'\nexit 1\n"), 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	pc := mp4TestContext(filepath.Join(dir, "ffmpeg"))
	pc.Ctx = ctx
	_, err = (&ConvertMp4Stage{}).Execute(pc, []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("预算等待应遵守取消: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("预算未释放不应开始探测")
	}
}
func TestConvertMP4RecoversPublicationIntent(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_link", true: "after_link"}[linked], func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "source.flv")
			mp4Fixture(t, source)
			target := filepath.Join(dir, "source.mp4")
			temp := filepath.Join(dir, ".converting-interrupted.mp4")
			ffmpeg, _ := exec.LookPath("ffmpeg")
			if out, err := exec.Command(ffmpeg, "-v", "error", "-i", source, "-c", "copy", "-movflags", "+faststart", temp).CombinedOutput(); err != nil {
				t.Fatalf("%s %v", out, err)
			}
			si, _ := os.Stat(source)
			ti, _ := os.Stat(temp)
			intent := map[string]any{"version": 1, "source_name": filepath.Base(source), "target_name": filepath.Base(target), "temp_name": filepath.Base(temp), "source": map[string]any{"size": si.Size(), "mtime_ns": si.ModTime().UnixNano()}, "target": map[string]any{"size": ti.Size(), "mtime_ns": ti.ModTime().UnixNano()}}
			data, _ := json.Marshal(intent)
			if err := os.WriteFile(target+".publishing.json", data, 0600); err != nil {
				t.Fatal(err)
			}
			if linked {
				if err := os.Link(temp, target); err != nil {
					t.Fatal(err)
				}
			}
			s := &ConvertMp4Stage{}
			out, err := s.Execute(mp4TestContext(ffmpeg), []pipeline.FileInfo{pipeline.NewVideoFileInfo(source)})
			if err != nil || len(out) != 2 {
				t.Fatalf("发布恢复失败: %+v %v", out, err)
			}
			if len(s.commands) != 0 {
				t.Fatal("恢复应复用已验证产物，不重复转封装")
			}
			if _, err := os.Stat(target + ".playback.json"); err != nil {
				t.Fatal("恢复未保存关联", err)
			}
		})
	}
}
