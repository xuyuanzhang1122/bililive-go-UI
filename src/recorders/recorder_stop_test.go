package recorders

import (
	"context"
	"sync"
	"testing"

	livemock "github.com/bililive-go/bililive-go/src/live/mock"
	"github.com/bililive-go/bililive-go/src/pkg/events"
	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	parsermock "github.com/bililive-go/bililive-go/src/pkg/parser/mock"
	"go.uber.org/mock/gomock"
)

func TestCloseBeforeParserAssignmentStopsNewParser(t *testing.T) {
	ctrl := gomock.NewController(t)
	room := livemock.NewMockLive(ctrl)
	room.EXPECT().GetLogger().Return(livelogger.New(0, nil)).AnyTimes()
	r := &recorder{
		Live:       room,
		ed:         events.NewDispatcher(context.Background()),
		parserLock: new(sync.RWMutex),
		stop:       make(chan struct{}),
		state:      running,
	}
	r.Close()
	p := parsermock.NewMockParser(ctrl)
	p.EXPECT().Stop().Return(nil)
	if r.setAndCloseParser(p) {
		t.Fatal("已经停止的录制器不能接纳新的下载进程")
	}
	if r.getParser() != nil {
		t.Fatal("被拒绝的下载进程不能成为当前解析器")
	}
}
