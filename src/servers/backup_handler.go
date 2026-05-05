package servers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/sentry"
)

var backupIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{3,80}$`)

type backupPackage struct {
	SchemaVersion int                  `json:"schemaVersion"`
	ExportedAt    time.Time            `json:"exportedAt"`
	IOSConfig     map[string]any       `json:"iosConfig"`
	Server        backupServerSnapshot `json:"server"`
}

type backupServerSnapshot struct {
	RPCBind     string           `json:"rpc_bind"`
	OutPutPath  string           `json:"out_put_path"`
	AppDataPath string           `json:"app_data_path"`
	LiveRooms   []backupLiveRoom `json:"live_rooms"`
}

type backupLiveRoom struct {
	URL         string `json:"url"`
	IsListening bool   `json:"is_listening"`
	Scheme      string `json:"scheme,omitempty"`
}

type backupRestoreResult struct {
	Status  string `json:"status"`
	JobID   string `json:"job_id,omitempty"`
	Message string `json:"message,omitempty"`
}

var restoreJobs = struct {
	sync.RWMutex
	m map[string]backupRestoreResult
}{m: make(map[string]backupRestoreResult)}

func createBackup(writer http.ResponseWriter, r *http.Request) {
	var pkg backupPackage
	if err := json.NewDecoder(r.Body).Decode(&pkg); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "备份格式错误: " + err.Error()})
		return
	}
	if err := validateBackupPackage(&pkg); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: err.Error()})
		return
	}
	if pkg.ExportedAt.IsZero() {
		pkg.ExportedAt = time.Now().UTC()
	}
	id := newBackupID()
	path, err := backupFilePath(id)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: err.Error()})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: "创建备份目录失败: " + err.Error()})
		return
	}
	data, _ := json.MarshalIndent(pkg, "", "  ")
	if err := os.WriteFile(path, data, 0600); err != nil {
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: "保存备份失败: " + err.Error()})
		return
	}
	writeJsonWithStatusCode(writer, http.StatusCreated, map[string]any{
		"id":         id,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func getBackup(writer http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	pkg, err := loadBackupPackage(id)
	if err != nil {
		writeJsonWithStatusCode(writer, http.StatusNotFound, commonResp{ErrNo: http.StatusNotFound, ErrMsg: err.Error()})
		return
	}
	writeJSON(writer, pkg)
}

func restoreBackup(writer http.ResponseWriter, r *http.Request) {
	var req struct {
		ID      string          `json:"id"`
		Package json.RawMessage `json:"package"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "请求格式错误: " + err.Error()})
		return
	}
	var pkg *backupPackage
	if strings.TrimSpace(req.ID) != "" {
		loaded, err := loadBackupPackage(strings.TrimSpace(req.ID))
		if err != nil {
			writeJsonWithStatusCode(writer, http.StatusNotFound, commonResp{ErrNo: http.StatusNotFound, ErrMsg: err.Error()})
			return
		}
		pkg = loaded
	} else if len(req.Package) > 0 {
		var inline backupPackage
		if err := json.Unmarshal(req.Package, &inline); err != nil {
			writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "备份包格式错误: " + err.Error()})
			return
		}
		pkg = &inline
	} else {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: "缺少 id 或 package"})
		return
	}
	if err := validateBackupPackage(pkg); err != nil {
		writeJsonWithStatusCode(writer, http.StatusBadRequest, commonResp{ErrNo: http.StatusBadRequest, ErrMsg: err.Error()})
		return
	}

	jobID := "restore_" + randomHex(4)
	setRestoreJob(jobID, backupRestoreResult{Status: "running", JobID: jobID, Message: "正在写入配置"})
	result, err := applyBackupPackage(r.Context(), pkg, jobID)
	if err != nil {
		result = backupRestoreResult{Status: "failed", JobID: jobID, Message: err.Error()}
		setRestoreJob(jobID, result)
		writeJsonWithStatusCode(writer, http.StatusInternalServerError, commonResp{ErrNo: http.StatusInternalServerError, ErrMsg: err.Error()})
		return
	}
	setRestoreJob(jobID, result)
	writeJSON(writer, result)
}

func getRestoreStatus(writer http.ResponseWriter, r *http.Request) {
	jobID := mux.Vars(r)["job_id"]
	restoreJobs.RLock()
	result, ok := restoreJobs.m[jobID]
	restoreJobs.RUnlock()
	if !ok {
		writeJsonWithStatusCode(writer, http.StatusNotFound, commonResp{ErrNo: http.StatusNotFound, ErrMsg: "恢复任务不存在"})
		return
	}
	writeJSON(writer, result)
}

func applyBackupPackage(ctx context.Context, pkg *backupPackage, jobID string) (backupRestoreResult, error) {
	oldCfg := configs.GetCurrentConfig()
	if oldCfg == nil {
		return backupRestoreResult{}, fmt.Errorf("配置未加载")
	}
	newCfg := configs.CloneConfigShallow(oldCfg)
	oldBind := oldCfg.RPC.Bind
	newCfg.RPC.Bind = pkg.Server.RPCBind
	newCfg.OutPutPath = pkg.Server.OutPutPath
	newCfg.AppDataPath = pkg.Server.AppDataPath
	newCfg.LiveRooms = make([]configs.LiveRoom, 0, len(pkg.Server.LiveRooms))
	for _, room := range pkg.Server.LiveRooms {
		newCfg.LiveRooms = append(newCfg.LiveRooms, configs.LiveRoom{
			Url:         room.URL,
			IsListening: room.IsListening,
			SchemeUrl:   room.Scheme,
		})
	}
	if err := newCfg.Verify(); err != nil {
		return backupRestoreResult{}, err
	}
	configs.SetCurrentConfig(newCfg)
	if err := applyLiveRoomsByConfig(ctx, oldCfg, newCfg); err != nil {
		configs.SetCurrentConfig(oldCfg)
		return backupRestoreResult{}, err
	}
	if err := newCfg.Marshal(); err != nil {
		return backupRestoreResult{}, err
	}
	if oldBind != newCfg.RPC.Bind && shutdownFunc != nil {
		result := backupRestoreResult{Status: "restarting", JobID: jobID, Message: "配置已写入，正在重启服务"}
		setRestoreJob(jobID, result)
		sentry.Go(func() {
			time.Sleep(700 * time.Millisecond)
			shutdownFunc()
		})
		return result, nil
	}
	return backupRestoreResult{Status: "completed", JobID: jobID, Message: "配置已恢复"}, nil
}

func validateBackupPackage(pkg *backupPackage) error {
	if pkg == nil {
		return fmt.Errorf("备份包为空")
	}
	if pkg.SchemaVersion != 1 {
		return fmt.Errorf("不支持的备份版本: %d", pkg.SchemaVersion)
	}
	if strings.TrimSpace(pkg.Server.RPCBind) == "" {
		return fmt.Errorf("rpc_bind 不能为空")
	}
	if _, err := net.ResolveTCPAddr("tcp", pkg.Server.RPCBind); err != nil {
		return fmt.Errorf("rpc_bind 无效: %w", err)
	}
	if err := validateRestorePath("out_put_path", pkg.Server.OutPutPath); err != nil {
		return err
	}
	if err := validateRestorePath("app_data_path", pkg.Server.AppDataPath); err != nil {
		return err
	}
	for _, room := range pkg.Server.LiveRooms {
		if strings.TrimSpace(room.URL) == "" {
			return fmt.Errorf("live_rooms 中存在空 URL")
		}
	}
	return nil
}

func validateRestorePath(name, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s 不能为空", name)
	}
	if strings.Contains(value, "\x00") {
		return fmt.Errorf("%s 包含非法字符", name)
	}
	clean := filepath.Clean(value)
	if clean == "." || clean == string(filepath.Separator) {
		return fmt.Errorf("%s 不能是根目录或当前目录", name)
	}
	return nil
}

func loadBackupPackage(id string) (*backupPackage, error) {
	path, err := backupFilePath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("备份不存在")
	}
	var pkg backupPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("备份文件损坏: %w", err)
	}
	return &pkg, nil
}

func backupFilePath(id string) (string, error) {
	if !backupIDPattern.MatchString(id) {
		return "", fmt.Errorf("备份 ID 非法")
	}
	root := ".appdata"
	if cfg := configs.GetCurrentConfig(); cfg != nil && strings.TrimSpace(cfg.AppDataPath) != "" {
		root = cfg.AppDataPath
	}
	return filepath.Join(root, "backups", id+".json"), nil
}

func newBackupID() string {
	return "bgo_" + time.Now().Format("20060102") + "_" + randomHex(4)
}

func randomHex(n int) string {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func setRestoreJob(jobID string, result backupRestoreResult) {
	restoreJobs.Lock()
	restoreJobs.m[jobID] = result
	restoreJobs.Unlock()
}
