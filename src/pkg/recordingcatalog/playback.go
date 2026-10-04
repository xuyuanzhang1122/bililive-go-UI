package recordingcatalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrPlaybackChanged = errors.New("录播来源或所选资产已变化")

type Recording struct {
	RecordingID   string `json:"recording_id"`
	SourceVersion string `json:"source_version"`
	Path          string `json:"path"`
	Room          string `json:"room"`
	Size          int64  `json:"size_bytes"`
}
type PlaybackFile struct {
	RecordingID, SourceVersion, AssetVersion              string
	Kind, RelativePath, Path, FileVersion, ProfileVersion string
}

const recordingQuery = `SELECT r.id,r.source_version,MIN(a.path),MIN(a.room),SUM(a.size) FROM recordings r JOIN recording_aliases a ON a.recording_id=r.id AND a.present=1`

func (m *Manager) ListRecordings(ctx context.Context, after string, limit int) ([]Recording, error) {
	if limit < 1 || limit > 100 {
		return nil, fmt.Errorf("分页上限必须为 1–100")
	}
	rows, err := m.store.db.QueryContext(ctx, recordingQuery+` WHERE r.id>? GROUP BY r.id,r.source_version ORDER BY r.id LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Recording, 0)
	for rows.Next() {
		var r Recording
		if err := rows.Scan(&r.RecordingID, &r.SourceVersion, &r.Path, &r.Room, &r.Size); err != nil {
			return nil, err
		}
		items = append(items, r)
	}
	return items, rows.Err()
}
func (m *Manager) PlaybackRecording(ctx context.Context, id string) (Recording, error) {
	var r Recording
	err := m.store.db.QueryRowContext(ctx, recordingQuery+` WHERE r.id=? GROUP BY r.id,r.source_version`, id).Scan(&r.RecordingID, &r.SourceVersion, &r.Path, &r.Room, &r.Size)
	return r, err
}

// PlaybackFiles 只读取一个逻辑录播的有限候选；分片集合由 FindHLS 单独查询。
func (m *Manager) PlaybackFiles(ctx context.Context, id, source string) ([]PlaybackFile, error) {
	r, err := m.PlaybackRecording(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.SourceVersion != source {
		return nil, ErrPlaybackChanged
	}
	rows, err := m.store.db.QueryContext(ctx, `SELECT a.path,a.file_id,a.size,a.mtime_ns,COALESCE(m.asset_version,''),COALESCE(p.profile_version,'') FROM recording_aliases a LEFT JOIN media_assets m ON m.path=a.path AND m.recording_id=a.recording_id AND m.source_version=? AND m.status='verified' LEFT JOIN prepared_verifications p ON p.path=a.path AND p.recording_id=a.recording_id AND p.source_version=? AND p.valid=1 AND m.path IS NOT NULL WHERE a.recording_id=? AND a.present=1 ORDER BY p.valid DESC,a.path LIMIT 65`, source, source, id)
	if err != nil {
		return nil, err
	}
	files := make([]PlaybackFile, 0)
	for rows.Next() {
		var path, identity, asset, profile string
		var size, mtime int64
		if err := rows.Scan(&path, &identity, &size, &mtime, &asset, &profile); err != nil {
			rows.Close()
			return nil, err
		}
		fv := version(identity, size, mtime)
		if fv != source && profile == "" {
			continue
		}
		kind := "source"
		if profile != "" {
			kind = "main"
			asset = source + "-" + profile + "-" + fv
		}
		if asset == "" {
			asset = source + "-raw-" + fv
		}
		files = append(files, PlaybackFile{id, source, asset, kind, path, filepath.Join(m.store.root, filepath.FromSlash(path)), fv, profile})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(files) > 64 {
		return nil, fmt.Errorf("录播候选数量超限")
	}
	rows, err = m.store.db.QueryContext(ctx, `SELECT a.path,a.asset_version,p.file_version,p.profile_version FROM media_assets a JOIN prepared_verifications p ON p.path=a.path AND p.recording_id=a.recording_id AND p.source_version=a.source_version WHERE a.recording_id=? AND a.source_version=? AND a.kind='compat' AND a.status='verified' AND p.valid=1 ORDER BY a.path LIMIT 17`, id, source)
	if err != nil {
		return nil, err
	}
	count := 0
	for rows.Next() {
		var rel, asset, fv, profile string
		if err := rows.Scan(&rel, &asset, &fv, &profile); err != nil {
			rows.Close()
			return nil, err
		}
		count++
		files = append(files, PlaybackFile{id, source, asset, "compat", rel, filepath.Join(m.store.assetRoot, filepath.FromSlash(strings.TrimPrefix(rel, "@derived/"))), fv, profile})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	if count > 16 {
		return nil, fmt.Errorf("兼容资产数量超限")
	}
	return files, nil
}
func boundedPlaybackPath(root, rel string) (string, error) {
	if rel == "" || strings.Contains(rel, "\\") || filepath.ToSlash(filepath.Clean(rel)) != rel || strings.HasPrefix(rel, "/") || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", ErrPlaybackChanged
	}
	path := root
	for _, part := range append([]string{""}, strings.Split(rel, "/")...) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrPlaybackChanged
		}
	}
	return path, nil
}
func (m *Manager) VerifyPlaybackFile(ctx context.Context, f PlaybackFile) error {
	r, err := m.PlaybackRecording(ctx, f.RecordingID)
	if err != nil {
		return err
	}
	if r.SourceVersion != f.SourceVersion {
		return ErrPlaybackChanged
	}
	root, rel := m.store.root, f.RelativePath
	if strings.HasPrefix(rel, "@derived/") {
		root = m.store.assetRoot
		rel = strings.TrimPrefix(rel, "@derived/")
	}
	path, err := boundedPlaybackPath(root, rel)
	if err != nil {
		return err
	}
	if path != f.Path {
		return ErrPlaybackChanged
	}
	if IsRecording(path) {
		return ErrRecordingActive
	}
	actual, err := SnapshotVersion(path)
	if err != nil {
		return err
	}
	if actual != f.FileVersion {
		return ErrPlaybackChanged
	}
	if f.ProfileVersion != "" {
		if err := m.VerifyPreparedAsset(ctx, Entry{RecordingID: f.RecordingID, SourceVersion: f.SourceVersion}, path, f.FileVersion, f.ProfileVersion); err != nil {
			return err
		}
		// 现存原始来源必须仍有效；只有后台确认删源后才允许可信主产物单独存续。
		rows, err := m.store.db.QueryContext(ctx, `SELECT file_id,size,mtime_ns FROM recording_aliases WHERE recording_id=? AND present=1`, f.RecordingID)
		if err != nil {
			return err
		}
		hasSource := false
		for rows.Next() {
			var identity string
			var size, mtime int64
			if err := rows.Scan(&identity, &size, &mtime); err != nil {
				rows.Close()
				return err
			}
			hasSource = hasSource || version(identity, size, mtime) == f.SourceVersion
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if hasSource {
			_, err = m.HLSSource(ctx, Entry{RecordingID: f.RecordingID, SourceVersion: f.SourceVersion})
			return err
		}
		return nil
	}
	if actual != f.SourceVersion {
		return ErrPlaybackChanged
	}
	var identity string
	var size, mtime int64
	err = m.store.db.QueryRowContext(ctx, `SELECT file_id,size,mtime_ns FROM recording_aliases WHERE path=? AND recording_id=? AND present=1`, f.RelativePath, f.RecordingID).Scan(&identity, &size, &mtime)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrPlaybackChanged
	}
	if err != nil {
		return err
	}
	if version(identity, size, mtime) != actual {
		return ErrPlaybackChanged
	}
	return nil
}

// RebindPlaybackFile 只跟随已核验的同一资产身份改名，不重选其他播放版本。
func (m *Manager) RebindPlaybackFile(ctx context.Context, f PlaybackFile) (PlaybackFile, error) {
	files, err := m.PlaybackFiles(ctx, f.RecordingID, f.SourceVersion)
	if err != nil {
		return PlaybackFile{}, err
	}
	for _, current := range files {
		if current.AssetVersion == f.AssetVersion && current.FileVersion == f.FileVersion && current.ProfileVersion == f.ProfileVersion {
			if err := m.VerifyPlaybackFile(ctx, current); err != nil {
				return PlaybackFile{}, err
			}
			return current, nil
		}
	}
	return PlaybackFile{}, ErrPlaybackChanged
}

func (m *Manager) RecordingForPath(ctx context.Context, path string) (Recording, error) {
	e, err := m.store.Lookup(ctx, path)
	if err != nil {
		return Recording{}, err
	}
	return m.PlaybackRecording(ctx, e.RecordingID)
}
