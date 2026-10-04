package servers

import (
	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/instance"
	"github.com/bililive-go/bililive-go/src/pipeline"
	"github.com/bililive-go/bililive-go/src/pipeline/stages"
	"github.com/gorilla/mux"
	"net/http"
)

func registerHistoryRoutes(route *mux.Router) {
	route.HandleFunc("/history-previews", previewHistory).Methods("POST")
	route.HandleFunc("/history-previews/{id}", getHistoryPreview).Methods("GET")
	route.HandleFunc("/history-previews/{id}/confirm", confirmHistoryPreview).Methods("POST")
	route.HandleFunc("/history-previews/{id}/control", controlHistoryPreview).Methods("POST")
}
func historyManager(w http.ResponseWriter, r *http.Request) *pipeline.Manager {
	inst := instance.GetInstance(r.Context())
	if inst == nil || inst.RecordingCatalog == nil || !inst.RecordingCatalog.MatchesRoot(configs.GetCurrentConfig().OutPutPath) {
		v2Fail(w, r, 503, "catalog_unavailable", "当前录播目录未就绪", true)
		return nil
	}
	pm := pipeline.GetManager(inst)
	if pm == nil {
		v2Fail(w, r, 503, "pipeline_unavailable", "持久准备队列未就绪", true)
	}
	return pm
}
func previewHistory(w http.ResponseWriter, r *http.Request) {
	pm := historyManager(w, r)
	if pm == nil {
		return
	}
	var body struct {
		After string `json:"after"`
		Limit *int   `json:"limit"`
	}
	if v2Decode(w, r, &body) != nil {
		v2Fail(w, r, 400, "invalid_request", "预览请求格式无效", false)
		return
	}
	limit := 100
	if body.Limit != nil {
		limit = *body.Limit
	}
	p, err := pm.PreviewHistory(r.Context(), currentAPIKeyUserID(r), body.After, limit, stages.PreparationProfileVersion)
	if err != nil {
		v2Fail(w, r, 409, "preview_unavailable", err.Error(), false)
		return
	}
	v2Write(w, r, 200, p, nil)
}
func getHistoryPreview(w http.ResponseWriter, r *http.Request) {
	pm := historyManager(w, r)
	if pm == nil {
		return
	}
	p, err := pm.GetHistoryPreview(r.Context(), currentAPIKeyUserID(r), mux.Vars(r)["id"])
	if err != nil {
		v2Fail(w, r, 404, "preview_not_found", "预览不存在或属于其他用户", false)
		return
	}
	v2Write(w, r, 200, p, nil)
}
func confirmHistoryPreview(w http.ResponseWriter, r *http.Request) {
	pm := historyManager(w, r)
	if pm == nil {
		return
	}
	var body struct {
		Digest  string `json:"digest"`
		Confirm bool   `json:"confirm"`
	}
	if v2Decode(w, r, &body) != nil || !body.Confirm || body.Digest == "" {
		v2Fail(w, r, 400, "confirmation_required", "需要明确确认和原预览摘要", false)
		return
	}
	p, err := pm.GetHistoryPreview(r.Context(), currentAPIKeyUserID(r), mux.Vars(r)["id"])
	if err != nil || p.ProfileVersion != stages.PreparationProfileVersion {
		v2Fail(w, r, 409, "preview_changed", "预览不存在或准备规则已变化", false)
		return
	}
	p, err = pm.ConfirmHistory(r.Context(), currentAPIKeyUserID(r), p.ID, body.Digest)
	if err != nil {
		v2Fail(w, r, 409, "preview_changed", err.Error(), false)
		return
	}
	v2Write(w, r, 200, p, nil)
}
func controlHistoryPreview(w http.ResponseWriter, r *http.Request) {
	pm := historyManager(w, r)
	if pm == nil {
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if v2Decode(w, r, &body) != nil || (body.Action != "pause" && body.Action != "resume") {
		v2Fail(w, r, 400, "invalid_request", "操作必须为pause或resume", false)
		return
	}
	owner, id := currentAPIKeyUserID(r), mux.Vars(r)["id"]
	if err := pm.SetHistoryPaused(r.Context(), owner, id, body.Action == "pause"); err != nil {
		v2Fail(w, r, 409, "preview_unavailable", err.Error(), false)
		return
	}
	p, err := pm.GetHistoryPreview(r.Context(), owner, id)
	if err != nil {
		v2Fail(w, r, 503, "preview_unavailable", "读取控制结果失败", true)
		return
	}
	v2Write(w, r, 200, p, nil)
}
