package reconcile

import (
	"context"
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bililive-go/bililive-go/src/livestate"
	log "github.com/sirupsen/logrus"
)

const (
	reconcileSourceUnknown = "reconcile-unknown"
	reconcileBatchSize     = 500
)

// ReconcileResult 对账结果
type ReconcileResult struct {
	ScannedFiles int // 扫描到的视频文件数
	NewRooms     int // 新创建的占位房间数
	OrphanRooms  int // DB 中有但文件全部丢失的房间数
	UnknownFiles int // 模板反推失败归入 unknown 的文件数
}

// Reconcile 扫描 outPutPath 下的视频文件，与 lives store 对账。
// outputTmpl 为空时所有文件归入 unknown 占位房间。
func Reconcile(ctx context.Context, store livestate.Store, outPutPath, outputTmpl string) (*ReconcileResult, error) {
	result := &ReconcileResult{}

	// 收集视频文件
	type videoFile struct {
		absPath  string
		roomKey  string // 识别的房间键（unknown:<hash> 或模板反推值）
		fileName string
	}
	var files []videoFile

	extSet := map[string]bool{".flv": true, ".mp4": true, ".m3u8": true}
	err := filepath.WalkDir(outPutPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !extSet[ext] {
			return nil
		}
		rel, err := filepath.Rel(outPutPath, path)
		if err != nil {
			rel = path
		}
		result.ScannedFiles++
		roomKey := classifyFile(rel, outputTmpl)
		if strings.HasPrefix(roomKey, "unknown:") {
			result.UnknownFiles++
		}
		files = append(files, videoFile{
			absPath:  path,
			roomKey:  roomKey,
			fileName: filepath.Base(path),
		})
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("扫描 %s 失败: %w", outPutPath, err)
	}

	// 获取现有房间列表（即使无视频文件也查，用于统计孤儿）
	existingRooms, err := store.GetAllLiveRooms(ctx)
	if err != nil {
		return result, fmt.Errorf("获取现有房间列表失败: %w", err)
	}
	existingMap := make(map[string]bool, len(existingRooms))
	for _, r := range existingRooms {
		existingMap[r.LiveID] = true
	}

	// 收集需要新建的房间（去重）
	toCreate := make(map[string]string) // roomKey → fileName（用于显示名）
	for _, f := range files {
		if existingMap[f.roomKey] {
			continue
		}
		if _, ok := toCreate[f.roomKey]; !ok {
			toCreate[f.roomKey] = f.fileName
		}
	}

	// 分批写入
	var mu sync.Mutex
	var batch []*livestate.LiveRoom
	now := time.Now()
	for roomKey, displayName := range toCreate {
		platform := "unknown"
		hostName := displayName
		if !strings.HasPrefix(roomKey, "unknown:") {
			// 尝试从 roomKey 中解析出 platform/host
			// roomKey 格式: platform:host:room_id
			parts := strings.SplitN(roomKey, ":", 3)
			if len(parts) >= 1 {
				platform = parts[0]
			}
			if len(parts) >= 2 {
				hostName = parts[1]
			}
		}
		batch = append(batch, &livestate.LiveRoom{
			LiveID:    roomKey,
			Platform:  platform,
			HostName:  hostName,
			RoomName:  displayName,
			Source:    reconcileSourceUnknown,
			CreatedAt: now,
			UpdatedAt: now,
		})
		if len(batch) >= reconcileBatchSize {
			mu.Lock()
			for _, room := range batch {
				if err := store.UpsertLiveRoom(ctx, room); err != nil {
					log.Warnf("[reconcile] 创建占位房间失败: %s: %v", room.LiveID, err)
				} else {
					result.NewRooms++
				}
			}
			mu.Unlock()
			batch = nil
		}
	}
	// 剩余批次
	for _, room := range batch {
		if err := store.UpsertLiveRoom(ctx, room); err != nil {
			log.Warnf("[reconcile] 创建占位房间失败: %s: %v", room.LiveID, err)
		} else {
			result.NewRooms++
		}
	}

	// 统计孤儿房间（DB 有但文件全丢）
	for _, r := range existingRooms {
		hasFile := false
		for _, f := range files {
			if f.roomKey == r.LiveID {
				hasFile = true
				break
			}
		}
		if !hasFile && r.LiveID != "" {
			result.OrphanRooms++
		}
	}

	log.Infof("[reconcile] 对账完成: 扫描=%d 新建=%d 孤儿=%d 未知=%d",
		result.ScannedFiles, result.NewRooms, result.OrphanRooms, result.UnknownFiles)

	return result, nil
}

// classifyFile 根据文件相对路径和输出模板反推房间键。
// 若 outputTmpl 为空，返回 "unknown:<sha1[:12]>"。
func classifyFile(relPath, outputTmpl string) string {
	if outputTmpl == "" {
		hash := sha1.Sum([]byte(relPath))
		return fmt.Sprintf("unknown:%x", hash[:6])
	}

	// 解析模板占位符顺序
	// 支持的占位符: Platform, HostName, RoomName
	// 模板示例: {{ .Live.GetPlatformCNName }}/{{ .HostName }}/.../[{{ .RoomName }}].flv
	fieldOrder := parseTemplateFields(outputTmpl)
	if len(fieldOrder) == 0 {
		// 空模板无法解析，归入 unknown
		hash := sha1.Sum([]byte(relPath))
		return fmt.Sprintf("unknown:%x", hash[:6])
	}

	// 按 / 切段，去掉扩展名；先统一把 Windows 反斜杠归一为 /，保证跨平台行为一致
	cleanPath := strings.TrimSuffix(relPath, filepath.Ext(relPath))
	segments := strings.Split(strings.ReplaceAll(cleanPath, "\\", "/"), "/")

	var platform, host, roomID string
	for i, field := range fieldOrder {
		if i >= len(segments) {
			break
		}
		switch field {
		case "platform":
			platform = segments[i]
		case "host":
			host = segments[i]
		case "room":
			roomID = segments[i]
		}
	}

	// 路径段数必须 >= 模板字段数，否则归入 unknown
	requiredFields := 0
	for _, f := range fieldOrder {
		switch f {
		case "platform", "host", "room":
			requiredFields++
		}
	}
	if len(segments) < requiredFields || platform == "" || host == "" {
		hash := sha1.Sum([]byte(relPath))
		return fmt.Sprintf("unknown:%x", hash[:6])
	}

	if roomID == "" {
		roomID = host
	}
	return fmt.Sprintf("%s:%s:%s", platform, host, roomID)
}

// parseTemplateFields 从 Go template 中提取占位符字段顺序
// 支持的映射:
//
//	{{ .Live.GetPlatformCNName }} → platform
//	{{ .HostName }}              → host
//	{{ .RoomName }}              → room
func parseTemplateFields(tmpl string) []string {
	var fields []string
	s := tmpl
	for {
		start := strings.Index(s, "{{")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}}")
		if end == -1 {
			break
		}
		block := s[start : start+end+2]
		s = s[start+end+2:]

		switch {
		case strings.Contains(block, "Live.GetPlatformCNName") || strings.Contains(block, "Live.GetPlatform"):
			fields = append(fields, "platform")
		case strings.Contains(block, ".HostName"):
			fields = append(fields, "host")
		case strings.Contains(block, ".RoomName"):
			fields = append(fields, "room")
		default:
			// 非房间识别字段（如 now, filenameFilter），跳过
		}
	}
	return fields
}
