package servers

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bililive-go/bililive-go/src/configs"
	"github.com/bililive-go/bililive-go/src/pkg/playbackdecision"
	"github.com/bililive-go/bililive-go/src/pkg/recordingcatalog"
	securitypkg "github.com/bililive-go/bililive-go/src/pkg/security"
	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

type v2Session struct {
	DTO                     v2SessionDTO
	Owner, Credential, Root string
	Capabilities            playbackdecision.Capabilities
	File                    *recordingcatalog.PlaybackFile
	HLSProfile              string
	Invalidated             bool
	Revision                uint64
	MimeType                string
}
type v2Service struct {
	mu       sync.Mutex
	sessions map[string]v2Session
	secret   string
	now      func() time.Time
}
type v2ContextKey struct{}

var errV2SessionExpired = errors.New("播放会话已过期，请重新创建")

func v2FromRequest(r *http.Request) *v2Service {
	s, _ := r.Context().Value(v2ContextKey{}).(*v2Service)
	return s
}
func (s *v2Service) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), v2ContextKey{}, s))
		if s.secret == "" {
			v2Fail(w, r, http.StatusServiceUnavailable, "service_unavailable", "会话授权服务未就绪", true)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/v2/media/") {
			next.ServeHTTP(w, r)
			return
		}

		authenticated, ok := v2Authenticate(r)
		if !ok {
			v2Fail(w, r, http.StatusUnauthorized, "unauthorized", "缺少有效 Bearer Key 或授权已撤销", false)
			return
		}
		next.ServeHTTP(w, authenticated)

	})
}
func v2ValidIdentity(id, version string) bool {
	_, err := uuid.FromString(id)
	v, e := hex.DecodeString(version)
	return err == nil && e == nil && len(v) == 32
}

func v2Authenticate(r *http.Request) (*http.Request, bool) {
	auth := r.Header.Get("Authorization")
	if len(auth) <= 7 || !strings.EqualFold(auth[:7], "Bearer ") || r.URL.Query().Get("_key") != "" || r.Header.Get("X-API-Key") != "" {
		return r, false
	}
	user, key, ok, _ := authenticateAPIKeyRequest(r, configs.GetCurrentConfig())
	if !ok {
		return r, false
	}
	return withAPIKeyUser(r, user, key), true
}
func (s *v2Service) ownedSession(r *http.Request) (v2Session, int, *v2Error) {
	id := mux.Vars(r)["id"]
	s.mu.Lock()
	session, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok {
		return session, http.StatusNotFound, v2Problem("session_not_found", "播放会话不存在或服务已重启", false)
	}
	if session.Owner != currentAPIKeyUserID(r) || session.Credential != securitypkg.HashAPIKey(currentRawAPIKey(r)) {
		return session, http.StatusForbidden, v2Problem("forbidden", "会话属于其他授权用户", false)
	}
	if session.DTO.ExpiresAt <= s.now().Unix() {
		return session, http.StatusGone, v2Problem("session_expired", "播放会话已过期，请重新创建", false)
	}
	return session, http.StatusOK, nil
}
func (s *v2Service) getOwned(w http.ResponseWriter, r *http.Request) (v2Session, bool) {
	session, status, err := s.ownedSession(r)
	if err != nil {
		v2Write(w, r, status, nil, err)
		return session, false
	}
	return session, true
}
func (s *v2Service) create(w http.ResponseWriter, r *http.Request) {
	var body v2CreateSession
	if v2Decode(w, r, &body) != nil || !v2ValidIdentity(body.RecordingID, body.SourceVersion) || playbackdecision.Validate(body.Capabilities) != nil {
		v2Fail(w, r, http.StatusBadRequest, "invalid_request", "录播身份或能力请求无效", false)
		return
	}
	cfg := configs.GetCurrentConfig()
	if cfg == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "配置或目录未就绪", true)
		return
	}
	cat := requestCatalog(r, cfg)
	if cat == nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "录播目录未就绪", true)
		return
	}
	status, err := cat.Status(r.Context())
	if err != nil || status != "verified" {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "录播目录尚未核验或不可用", true)
		return
	}
	record, err := cat.PlaybackRecording(r.Context(), body.RecordingID)
	if errors.Is(err, sql.ErrNoRows) {
		v2Fail(w, r, http.StatusNotFound, "recording_not_found", "录播不存在", false)
		return
	}
	if err != nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "catalog_unavailable", "录播查询失败", true)
		return
	}
	if record.SourceVersion != body.SourceVersion {
		v2Fail(w, r, http.StatusConflict, "source_changed", "录播来源版本已变化", false)
		return
	}
	id, err := uuid.NewV4()
	if err != nil {
		v2Fail(w, r, http.StatusServiceUnavailable, "service_unavailable", "无法生成播放身份", true)
		return
	}
	session := v2Session{DTO: v2SessionDTO{ID: id.String(), RecordingID: body.RecordingID, SourceVersion: body.SourceVersion, Status: "processing", Decision: "direct", ExpiresAt: s.now().Add(15 * time.Minute).Unix()}, Owner: currentAPIKeyUserID(r), Credential: securitypkg.HashAPIKey(currentRawAPIKey(r)), Root: cfg.OutPutPath, Capabilities: body.Capabilities}
	session = s.resolve(r, session)
	session.Revision = 1
	if session.DTO.Error != nil && session.DTO.Error.Code == "recording_active" {
		v2Fail(w, r, http.StatusConflict, "recording_active", "文件仍在录制", true)
		return
	}
	s.mu.Lock()
	count := 0
	active := 0
	oldestCancelled := ""
	now := s.now().Unix()
	for key, v := range s.sessions {
		if v.DTO.ExpiresAt <= now {
			delete(s.sessions, key)
		} else if v.DTO.Status == "cancelled" {
			if oldestCancelled == "" || v.DTO.ExpiresAt < s.sessions[oldestCancelled].DTO.ExpiresAt {
				oldestCancelled = key
			}
		} else {
			active++
			if v.Owner == session.Owner {
				count++
			}
		}
	}
	if active >= 1024 || count >= 32 {
		s.mu.Unlock()
		v2Fail(w, r, http.StatusTooManyRequests, "session_limit", "播放会话数量已达到上限", true)
		return
	}
	// 保留有界取消记录，拒绝旧 token；取消记录不占用播放器活跃配额。
	if len(s.sessions) >= 2048 && oldestCancelled != "" {
		delete(s.sessions, oldestCancelled)
	}
	s.sessions[session.DTO.ID] = session
	s.mu.Unlock()
	s.respond(w, r, http.StatusCreated, session)
}
func (s *v2Service) get(w http.ResponseWriter, r *http.Request) {
	session, ok := s.getOwned(w, r)
	if !ok {
		return
	}
	if session.DTO.Status != "cancelled" && !session.Invalidated {
		session = s.resolve(r, session)
	}
	session, exists := s.commitResolved(r, session)
	if !exists {
		v2Fail(w, r, http.StatusGone, "session_expired", "会话已失效", false)
		return
	}
	s.respond(w, r, http.StatusOK, session)
}

// commitResolved 以修订号提交，两个入口都不能写回过期快照或已取消的请求结果。
func (s *v2Service) commitResolved(r *http.Request, session v2Session) (v2Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.sessions[session.DTO.ID]
	if !exists {
		return session, false
	}
	if current.Revision != session.Revision || r.Context().Err() != nil || current.DTO.Status == "cancelled" || current.Invalidated {
		return current, true
	}
	session.Revision++
	s.sessions[session.DTO.ID] = session
	return session, true
}
func (s *v2Service) cancel(w http.ResponseWriter, r *http.Request) {
	session, ok := s.getOwned(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	current, exists := s.sessions[session.DTO.ID]
	if !exists {
		s.mu.Unlock()
		v2Fail(w, r, http.StatusGone, "session_expired", "会话已失效", false)
		return
	}
	session = current
	session.DTO.Status = "cancelled"
	session.DTO.Error = nil
	session.DTO.RetryAfter = 0
	session.Revision++
	s.sessions[session.DTO.ID] = session
	s.mu.Unlock()
	s.respond(w, r, http.StatusOK, session)
}
func (s *v2Service) respond(w http.ResponseWriter, r *http.Request, status int, session v2Session) {
	dto, err := s.publicSession(session)
	if err != nil {
		if errors.Is(err, errV2SessionExpired) {
			v2Fail(w, r, http.StatusGone, "session_expired", err.Error(), false)
			return
		}
		v2Fail(w, r, http.StatusServiceUnavailable, "service_unavailable", "媒体授权生成失败", true)
		return
	}
	v2Write(w, r, status, dto, nil)
}

func (s *v2Service) publicSession(session v2Session) (v2SessionDTO, error) {
	dto := session.DTO
	now := s.now().Unix()
	if now >= dto.ExpiresAt {
		return dto, errV2SessionExpired
	}
	if dto.Status == "ready" {
		exp := now + int64((5 * time.Minute).Seconds())
		if exp > dto.ExpiresAt {
			exp = dto.ExpiresAt
		}
		token, err := s.signToken(session, exp)
		if err != nil {
			return dto, err
		}
		name := "file"
		if dto.Protocol == "hls" {
			name = "index.m3u8"
		}
		dto.URL = v2MediaURL(dto.ID, name, token)
		dto.TokenExpiresAt = exp
		dto.RefreshAfter = int(exp-now) - 60
		if dto.RefreshAfter < 0 {
			dto.RefreshAfter = 0
		}
	}

	return dto, nil
}

func (s *v2Service) contextMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), v2ContextKey{}, s)))
	})
}
