package servers

import (
	"errors"
	"github.com/gorilla/mux"
	"net/http"
)

// 旧接口通过显式 session_id 消费 v2 会话，保持 commonResp 包装与状态词。
func resolveV2LegacySession(w http.ResponseWriter, r *http.Request) {
	s := v2FromRequest(r)
	if s == nil {
		writePlaybackResponse(w, playbackResolveResponse{Status: "failed", Error: "播放会话服务未就绪", RetryAfter: 2})
		return
	}
	authenticated, ok := v2Authenticate(r)
	if !ok {
		writeJsonWithStatusCode(w, http.StatusUnauthorized, commonResp{ErrNo: http.StatusUnauthorized, ErrMsg: "缺少有效 Bearer Key"})
		return
	}
	r = mux.SetURLVars(authenticated, map[string]string{"id": r.URL.Query().Get("session_id")})
	session, statusCode, problem := s.ownedSession(r)
	if problem != nil {
		writeJsonWithStatusCode(w, statusCode, commonResp{ErrNo: statusCode, ErrMsg: problem.Message})
		return
	}
	if session.DTO.Status != "cancelled" && !session.Invalidated {
		session = s.resolve(r, session)
	}
	session, exists := s.commitResolved(r, session)
	if !exists {
		writeJsonWithStatusCode(w, http.StatusGone, commonResp{ErrNo: http.StatusGone, ErrMsg: "会话已失效"})
		return
	}
	dto, err := s.publicSession(session)
	if err != nil {
		if errors.Is(err, errV2SessionExpired) {
			writeJsonWithStatusCode(w, http.StatusGone, commonResp{ErrNo: http.StatusGone, ErrMsg: err.Error()})
			return
		}
		writePlaybackResponse(w, playbackResolveResponse{Status: "failed", Error: err.Error(), RetryAfter: 2})
		return
	}
	status := dto.Status
	if status == "paused" {
		status = "processing"
	}
	if status == "cancelled" {
		status = "failed"
	}
	message := ""
	if dto.Error != nil {
		message = dto.Error.Message
	}
	if dto.Status == "cancelled" {
		message = "播放会话已取消"
	}
	mime := session.MimeType
	if dto.Protocol == "hls" {
		mime = "application/vnd.apple.mpegurl"
	}
	writePlaybackResponse(w, playbackResolveResponse{Status: status, Protocol: dto.Protocol, MimeType: mime, URL: dto.URL, ExpiresAt: dto.TokenExpiresAt, Error: message, RetryAfter: dto.RetryAfter})
}
