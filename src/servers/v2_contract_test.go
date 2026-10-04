package servers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/require"
)

// 将真实 HTTP DTO 与共享契约比较，避免只验证各端各自复制的结构。
func v2AssertSchema(t *testing.T, name string, value any) {
	t.Helper()
	data, err := os.ReadFile("../../docs/contracts/openapi-v2.yaml")
	require.NoError(t, err)
	var contract map[string]any
	require.NoError(t, json.Unmarshal(data, &contract))
	schemas := contract["components"].(map[string]any)["schemas"].(map[string]any)
	var check func(map[string]any, any, string)
	check = func(schema map[string]any, v any, path string) {
		t.Helper()
		if v == nil && schema["nullable"] == true {
			return
		}
		if ref, ok := schema["$ref"].(string); ok {
			check(schemas[strings.TrimPrefix(ref, "#/components/schemas/")].(map[string]any), v, path)
			return
		}
		if all, ok := schema["allOf"].([]any); ok {
			for _, part := range all {
				check(part.(map[string]any), v, path)
			}
		}
		if allowed, ok := schema["enum"].([]any); ok {
			found := false
			for _, item := range allowed {
				found = found || reflect.DeepEqual(item, v)
			}
			require.True(t, found, path+" enum")
		}
		switch schema["type"] {
		case "object":
			obj, ok := v.(map[string]any)
			require.True(t, ok, path+" object")
			props, _ := schema["properties"].(map[string]any)
			if required, ok := schema["required"].([]any); ok {
				for _, key := range required {
					_, ok := obj[key.(string)]
					require.True(t, ok, path+" 缺少 "+key.(string))
				}
			}
			for key, item := range obj {
				p, exists := props[key]
				if !exists {
					require.NotEqual(t, false, schema["additionalProperties"], path+" 未知字段 "+key)
					continue
				}
				check(p.(map[string]any), item, path+"."+key)
			}
		case "array":
			arr, ok := v.([]any)
			require.True(t, ok, path+" array")
			if n, ok := schema["minItems"].(float64); ok {
				require.GreaterOrEqual(t, len(arr), int(n), path)
			}
			if n, ok := schema["maxItems"].(float64); ok {
				require.LessOrEqual(t, len(arr), int(n), path)
			}
			for i, item := range arr {
				check(schema["items"].(map[string]any), item, fmt.Sprintf("%s[%d]", path, i))
			}
		case "string":
			s, ok := v.(string)
			require.True(t, ok, path+" string")
			if pattern, ok := schema["pattern"].(string); ok {
				require.Regexp(t, regexp.MustCompile(pattern), s, path)
			}
			if schema["format"] == "uuid" {
				_, err := uuid.FromString(s)
				require.NoError(t, err, path)
			}
			if n, ok := schema["minLength"].(float64); ok {
				require.GreaterOrEqual(t, len(s), int(n), path)
			}
		case "boolean":
			_, ok := v.(bool)
			require.True(t, ok, path+" boolean")
		case "integer", "number":
			n, ok := v.(float64)
			require.True(t, ok, path+" number")
			if schema["type"] == "integer" {
				require.Equal(t, math.Trunc(n), n, path)
			}
			if min, ok := schema["minimum"].(float64); ok {
				require.GreaterOrEqual(t, n, min, path)
			}
			if max, ok := schema["maximum"].(float64); ok {
				require.LessOrEqual(t, n, max, path)
			}
		}
	}
	check(schemas[name].(map[string]any), value, name)
}
func v2AssertResponse(t *testing.T, res *httptest.ResponseRecorder, schema string) {
	t.Helper()
	var value any
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &value), res.Body.String())
	v2AssertSchema(t, schema, value)
	require.Equal(t, "application/json", res.Header().Get("Content-Type"))
	require.NotEmpty(t, res.Header().Get("X-Request-ID"))
}
func TestV2ContractFixturesAndRealHTTPResponses(t *testing.T) {
	files, err := filepath.Glob("../../docs/contracts/examples/v2-*.json")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		var sample struct {
			Schema string `json:"schema"`
			Value  any    `json:"value"`
		}
		require.NoError(t, json.Unmarshal(data, &sample))
		v2AssertSchema(t, sample.Schema, sample.Value)
	}
	h, _, cfg, _, entry, _, _ := v2Fixture(t, "mp4")
	res := v2Request(t, h, "GET", "/api/v2/recordings", cfg.Security.APIKey, nil)
	require.Equal(t, 200, res.Code, res.Body.String())
	v2AssertResponse(t, res, "RecordingsEnvelope")
	unauthorized := v2Request(t, h, "GET", "/api/v2/recordings", "", nil)
	require.Equal(t, http.StatusUnauthorized, unauthorized.Code)
	v2AssertResponse(t, unauthorized, "ErrorEnvelope")
	res = v2Request(t, h, "POST", "/api/v2/playback-sessions", cfg.Security.APIKey, map[string]any{"recording_id": entry.RecordingID, "source_version": entry.SourceVersion, "capabilities": v2Caps()})
	require.Equal(t, 201, res.Code, res.Body.String())
	v2AssertResponse(t, res, "SessionEnvelope")
	session := v2ReadSession(t, res)
	res = v2Request(t, h, "DELETE", "/api/v2/playback-sessions/"+session.ID, cfg.Security.APIKey, nil)
	v2AssertResponse(t, res, "SessionEnvelope")
	res = v2Request(t, h, "POST", "/api/v2/recordings/batch-delete", cfg.Security.APIKey, map[string]any{"items": []any{map[string]any{"recording_id": entry.RecordingID, "source_version": strings.Repeat("b", 64)}}})
	require.Equal(t, 200, res.Code, res.Body.String())
	v2AssertResponse(t, res, "BatchEnvelope")
}
