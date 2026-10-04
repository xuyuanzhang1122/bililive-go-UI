package servers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bililive-go/bililive-go/src/pkg/playbackdecision"
	uuid "github.com/satori/go.uuid"
)

type v2Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	RetryAfter int    `json:"retry_after_seconds,omitempty"`
}
type v2Envelope struct {
	Data      any      `json:"data"`
	Error     *v2Error `json:"error"`
	RequestID string   `json:"request_id"`
}
type v2CreateSession struct {
	RecordingID   string                        `json:"recording_id"`
	SourceVersion string                        `json:"source_version"`
	Capabilities  playbackdecision.Capabilities `json:"capabilities"`
}
type v2SessionDTO struct {
	ID             string   `json:"id"`
	RecordingID    string   `json:"recording_id"`
	SourceVersion  string   `json:"source_version"`
	AssetVersion   string   `json:"asset_version"`
	Status         string   `json:"status"`
	Decision       string   `json:"decision"`
	Protocol       string   `json:"protocol,omitempty"`
	URL            string   `json:"url,omitempty"`
	ExpiresAt      int64    `json:"expires_at"`
	TokenExpiresAt int64    `json:"token_expires_at,omitempty"`
	RefreshAfter   int      `json:"refresh_after_seconds,omitempty"`
	RetryAfter     int      `json:"retry_after_seconds"`
	Error          *v2Error `json:"error"`
}

func v2Write(w http.ResponseWriter, r *http.Request, status int, data any, e *v2Error) {
	id, err := uuid.NewV4()
	rid := id.String()
	if err != nil {
		rid = fmt.Sprint(time.Now().UnixNano())
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Request-ID", rid)
	if d, ok := data.(v2SessionDTO); ok && d.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(d.RetryAfter))
	}
	if e != nil && e.Retryable {
		w.Header().Set("Retry-After", "2")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v2Envelope{data, e, rid})
}
func v2Fail(w http.ResponseWriter, r *http.Request, status int, code, msg string, retry bool) {
	v2Write(w, r, status, nil, v2Problem(code, msg, retry))
}
func v2Decode(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("请求必须仅含一个 JSON 对象")
	}
	return nil
}

func v2Problem(code, message string, retry bool) *v2Error {
	interval := 0
	if retry {
		interval = 2
	}
	return &v2Error{Code: code, Message: message, Retryable: retry, RetryAfter: interval}
}
