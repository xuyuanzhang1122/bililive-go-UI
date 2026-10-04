package servers

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
)

type v2RecordingDTO struct {
	recordingcatalog.Recording
	SizeText string `json:"size_text"`
}

func (s *v2Service) list(w http.ResponseWriter, r *http.Request) {
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "配置未就绪", true)
		return
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录未就绪", true)
		return
	}
	status, err := cat.Status(r.Context())
	if err != nil || status != "verified" {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录尚未核验或不可用", true)
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		v2Fail(w, r, http.StatusBadRequest, "invalid_request", "分页数量必须为 1–100", false)
		return
	}
	records, err := cat.ListRecordings(r.Context(), r.URL.Query().Get("after"), limit)
	if err != nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录读取失败", true)
		return
	}
	items := make([]v2RecordingDTO, 0, len(records))
	for _, record := range records {
		items = append(items, v2RecordingDTO{record, formatVideoLibrarySize(record.Size)})
	}
	next := ""
	if len(records) == limit {
		next = records[len(records)-1].RecordingID
	}
	v2Write(w, r, http.StatusOK, struct {
		Items  []v2RecordingDTO `json:"items"`
		Next   string           `json:"next_cursor"`
		Status string           `json:"catalog_status"`
	}{items, next, status}, nil)
}

type v2DeleteItem struct {
	RecordingID   string `json:"recording_id"`
	SourceVersion string `json:"source_version"`
}
type v2DeleteResult struct {
	RecordingID string   `json:"recording_id"`
	Status      string   `json:"status"`
	Deleted     int      `json:"deleted_files"`
	Error       *v2Error `json:"error"`
}

func (s *v2Service) batchDelete(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items []v2DeleteItem `json:"items"`
	}
	if v2Decode(w, r, &body) != nil || len(body.Items) == 0 || len(body.Items) > 100 {
		v2Fail(w, r, http.StatusBadRequest, "invalid_request", "批删需包含 1–100 项", false)
		return
	}
	seen := make(map[string]bool)
	for _, item := range body.Items {
		if !v2ValidIdentity(item.RecordingID, item.SourceVersion) || seen[item.RecordingID] {
			v2Fail(w, r, http.StatusBadRequest, "invalid_request", "批删身份无效或重复", false)
			return
		}
		seen[item.RecordingID] = true
	}
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "配置未就绪", true)
		return
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录未就绪", true)
		return
	}
	status, err := cat.Status(r.Context())
	if err != nil || status != "verified" {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "目录未核验或不可用", true)
		return
	}
	results := make([]v2DeleteResult, 0, len(body.Items))
	for _, item := range body.Items {
		deleted, err := cat.DeleteRecording(r.Context(), item.RecordingID, item.SourceVersion)
		result := v2DeleteResult{RecordingID: item.RecordingID, Deleted: deleted, Status: "deleted"}
		if err != nil {
			result.Status = "failed"
			code, retry := "delete_failed", true
			if errors.Is(err, recordingcatalog.ErrRecordingActive) {
				code = "recording_active"
			} else if errors.Is(err, recordingcatalog.ErrPlaybackChanged) {
				code = "source_changed"
				retry = false
			} else if errors.Is(err, sql.ErrNoRows) {
				code = "recording_not_found"
				retry = false
			}
			result.Error = v2Problem(code, err.Error(), retry)
		}
		results = append(results, result)
	}
	v2Write(w, r, http.StatusOK, struct {
		Items []v2DeleteResult `json:"items"`
	}{results}, nil)
}
