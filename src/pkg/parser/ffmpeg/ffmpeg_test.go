package ffmpeg

import (
	"context"
	"errors"
	"net/url"
	"os/exec"
	"sync"
	"testing"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/live"
	livemock "github.com/bililive-go/bililive-go/src/live/mock"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"go.uber.org/mock/gomock"
)

func TestStopBeforeStartDoesNotLaunchFFmpeg(t *testing.T) {
	path, err := exec.LookPath("echo")
	if err != nil {
		t.Fatal(err)
	}
	previous := configs.GetCurrentConfig()
	t.Cleanup(func() { configs.SetCurrentConfig(previous) })
	configs.SetCurrentConfig(&configs.Config{FfmpegPath: path})

	ctrl := gomock.NewController(t)
	room := livemock.NewMockLive(ctrl)
	room.EXPECT().GetRawUrl().Return("http://localhost/stream.flv").AnyTimes()
	p := &Parser{closeOnce: new(sync.Once), timeoutInUs: "1000000", logger: new(livelogger.LiveLogger)}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	stream := &live.StreamUrlInfo{Url: &url.URL{Scheme: "http", Host: "localhost", Path: "/stream.flv"}}
	if err := p.ParseLiveStream(context.Background(), stream, room, t.TempDir()+"/output.flv"); !errors.Is(err, errStoppedBeforeStart) {
		t.Fatalf("停止后的解析器不应启动 FFmpeg，实际错误: %v", err)
	}
	if p.GetPID() != 0 {
		t.Fatal("停止后的解析器启动了下载进程")
	}
}
