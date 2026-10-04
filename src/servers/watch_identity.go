package servers

import (
	"context"
	"database/sql"
	"errors"
	"github.com/bililive-go/bililive-go/src/pkg/metadata"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	"path/filepath"
	"sort"
)

// 只合并已绑定的逻辑身份；旧记录缺少身份时保留原路径，避免源替换串进度。
func canonicalWatchHistory(ctx context.Context, entries []metadata.WatchHistoryEntry, catalog *recordingcatalog.Manager) ([]metadata.WatchHistoryEntry, error) {
	result := make([]metadata.WatchHistoryEntry, 0, len(entries))
	seen := map[string]bool{}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].UpdatedAt == entries[j].UpdatedAt {
			return entries[i].ID > entries[j].ID
		}
		return entries[i].UpdatedAt > entries[j].UpdatedAt
	})
	for _, entry := range entries {
		if catalog != nil && entry.RecordingID != "" {
			recording, err := catalog.PlaybackRecording(ctx, entry.RecordingID)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if recording.SourceVersion != entry.SourceVersion || seen[entry.RecordingID] {
				continue
			}
			seen[entry.RecordingID] = true
			entry.VideoPath = recording.Path
			entry.VideoName = filepath.Base(recording.Path)
		}
		result = append(result, entry)
	}
	return result, nil
}
