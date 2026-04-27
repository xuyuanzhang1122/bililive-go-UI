package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bililive-go/bililive-go/src/livestate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStore 实现 livestate.Store 接口的最小集合（仅 reconcile 实际调用的方法）
type stubStore struct {
	rooms map[string]*livestate.LiveRoom
}

func newStubStore() *stubStore {
	return &stubStore{
		rooms: make(map[string]*livestate.LiveRoom),
	}
}

func (s *stubStore) UpsertLiveRoom(_ context.Context, room *livestate.LiveRoom) error {
	if room.LiveID == "" {
		return nil
	}
	s.rooms[room.LiveID] = room
	return nil
}

func (s *stubStore) GetAllLiveRooms(_ context.Context) ([]*livestate.LiveRoom, error) {
	var out []*livestate.LiveRoom
	for _, r := range s.rooms {
		out = append(out, r)
	}
	return out, nil
}

// 以下为 livestate.Store 接口的其他方法 —— 桩实现，测试中不调用
func (s *stubStore) GetLiveRoom(ctx context.Context, liveID string) (*livestate.LiveRoom, error) {
	return nil, nil
}
func (s *stubStore) GetRecordingLiveRooms(ctx context.Context) ([]*livestate.LiveRoom, error) {
	return nil, nil
}
func (s *stubStore) UpdateHeartbeat(ctx context.Context, liveID string, timestamp time.Time) error {
	return nil
}
func (s *stubStore) SetRecordingStatus(ctx context.Context, liveID string, isRecording bool) error {
	return nil
}
func (s *stubStore) UpdateLiveInfo(ctx context.Context, liveID, hostName, roomName string) error {
	return nil
}
func (s *stubStore) UpdateLiveStartTime(ctx context.Context, liveID string, startTime time.Time) error {
	return nil
}
func (s *stubStore) UpdateLiveEndTime(ctx context.Context, liveID string, endTime time.Time) error {
	return nil
}
func (s *stubStore) StartSession(ctx context.Context, liveID, hostName, roomName string, startTime time.Time) (int64, error) {
	return 0, nil
}
func (s *stubStore) EndSession(ctx context.Context, liveID string, endTime time.Time, reason string) error {
	return nil
}
func (s *stubStore) EndSessionByHeartbeat(ctx context.Context, liveID string, reason string) error {
	return nil
}
func (s *stubStore) GetOpenSessions(ctx context.Context) ([]*livestate.LiveSession, error) {
	return nil, nil
}
func (s *stubStore) GetSessionsByLiveID(ctx context.Context, liveID string, limit int) ([]*livestate.LiveSession, error) {
	return nil, nil
}
func (s *stubStore) RecordNameChange(ctx context.Context, liveID, nameType, oldValue, newValue string) error {
	return nil
}
func (s *stubStore) GetNameHistory(ctx context.Context, liveID string, limit int) ([]*livestate.NameChange, error) {
	return nil, nil
}
func (s *stubStore) SaveAvailableStreams(ctx context.Context, liveID string, streams []*livestate.AvailableStream) error {
	return nil
}
func (s *stubStore) GetAvailableStreams(ctx context.Context, liveID string) ([]*livestate.AvailableStream, error) {
	return nil, nil
}
func (s *stubStore) SaveAvailableStreamsAny(ctx context.Context, liveID string, streams interface{}) error {
	return nil
}
func (s *stubStore) Close() error { return nil }

// 分支 a：空目录 → 扫描 0 文件，无操作
func TestReconcile_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	store := newStubStore()
	result, err := Reconcile(context.Background(), store, dir, "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.ScannedFiles)
	assert.Equal(t, 0, result.NewRooms)
}

// 分支 b：空 OutputTmpl → 所有文件归入 unknown 占位
func TestReconcile_EmptyTmpl(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.flv"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "test.mp4"), []byte("x"), 0644))

	store := newStubStore()
	result, err := Reconcile(context.Background(), store, dir, "")
	require.NoError(t, err)
	assert.Equal(t, 2, result.ScannedFiles)
	assert.Equal(t, 2, result.UnknownFiles)
	// 两个文件各创建一个 unknown 占位
	assert.Equal(t, 2, result.NewRooms)
}

// 分支 c：模板可解析 → 正确提取 platform/host/room
func TestReconcile_WithTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "douyin", "host1"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "douyin", "host1", "room1.flv"), []byte("x"), 0644))

	tmpl := "{{ .Live.GetPlatformCNName }}/{{ .HostName }}/[{{ .RoomName }}].flv"
	store := newStubStore()
	result, err := Reconcile(context.Background(), store, dir, tmpl)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ScannedFiles)
	assert.Equal(t, 1, result.NewRooms)
	assert.Equal(t, 0, result.UnknownFiles)

	_, ok := store.rooms["douyin:host1:room1"]
	assert.True(t, ok, "应按模板解析出房间键 douyin:host1:room1")
}

// 分支 d：模板部分匹配 → 无法解析的文件归入 unknown
func TestReconcile_PartialMatch(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "douyin", "host1"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "single"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "douyin", "host1", "room1.mp4"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "single", "orphan.mp4"), []byte("x"), 0644))

	tmpl := "{{ .Live.GetPlatformCNName }}/{{ .HostName }}/[{{ .RoomName }}].flv"
	store := newStubStore()
	result, err := Reconcile(context.Background(), store, dir, tmpl)
	require.NoError(t, err)
	assert.Equal(t, 2, result.ScannedFiles)
	assert.Equal(t, 1, result.UnknownFiles, "single/orphan.mp4 路径不匹配 3 段模板应归入 unknown")
}

// 分支 e：DB 有房间但文件全丢 → 统计为孤儿
func TestReconcile_OrphanRooms(t *testing.T) {
	dir := t.TempDir()
	store := newStubStore()
	store.rooms["bilibili:oldhost:oldroom"] = &livestate.LiveRoom{
		LiveID:   "bilibili:oldhost:oldroom",
		Platform: "bilibili",
	}
	result, err := Reconcile(context.Background(), store, dir, "")
	require.NoError(t, err)
	assert.Equal(t, 0, result.ScannedFiles)
	assert.Equal(t, 1, result.OrphanRooms)
}

// parseTemplateFields 单元测试
func TestParseTemplateFields(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		expected []string
	}{
		{"empty", "", nil},
		{"standard", "{{ .Live.GetPlatformCNName }}/{{ .HostName }}/[{{ .RoomName }}].flv",
			[]string{"platform", "host", "room"}},
		{"two fields", "/srv/bililive/{{ .HostName }}/{{ .RoomName }}.mp4",
			[]string{"host", "room"}},
		{"unknown fields", "{{ .Foo }}/{{ .Bar }}.flv", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTemplateFields(tt.tmpl)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// classifyFile 单元测试
func TestClassifyFile(t *testing.T) {
	tmpl := "{{ .Live.GetPlatformCNName }}/{{ .HostName }}/[{{ .RoomName }}].flv"

	t.Run("empty tmpl returns unknown", func(t *testing.T) {
		key := classifyFile("douyin/host1/room1.flv", "")
		assert.Contains(t, key, "unknown:")
	})

	t.Run("known template produces room key", func(t *testing.T) {
		key := classifyFile("douyin/host1/room1.flv", tmpl)
		assert.Equal(t, "douyin:host1:room1", key)
	})

	t.Run("shallow path with template returns unknown", func(t *testing.T) {
		key := classifyFile("orphan.mp4", tmpl)
		assert.Contains(t, key, "unknown:", "缺少 platform/host 段应归入 unknown")
	})
}
