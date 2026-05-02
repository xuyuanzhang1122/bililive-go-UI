package servers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSameOriginRequest_SecFetchSite(t *testing.T) {
	// 浏览器同源请求
	r, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	assert.True(t, isSameOriginRequest(r), "Sec-Fetch-Site: same-origin 应被判定为同源")

	// 跨站请求
	r2, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r2.Header.Set("Sec-Fetch-Site", "cross-site")
	assert.False(t, isSameOriginRequest(r2), "Sec-Fetch-Site: cross-site 不应被判定为同源")

	// none（用户直接输入 URL）
	r3, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r3.Header.Set("Sec-Fetch-Site", "none")
	assert.False(t, isSameOriginRequest(r3), "Sec-Fetch-Site: none 不应被判定为同源")
}

func TestIsSameOriginRequest_Referer(t *testing.T) {
	// 无 Sec-Fetch-Site，回退到 Referer 检查
	r, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r.Host = "localhost:8080"
	r.Header.Set("Referer", "http://localhost:8080/")
	assert.True(t, isSameOriginRequest(r), "Referer host 匹配应被判定为同源")

	// Referer 不匹配
	r2, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r2.Host = "localhost:8080"
	r2.Header.Set("Referer", "http://evil.com/")
	assert.False(t, isSameOriginRequest(r2), "Referer host 不匹配不应被判定为同源")
}

func TestIsSameOriginRequest_NoHeaders(t *testing.T) {
	// 无浏览器 header（iOS App 场景）
	r, _ := http.NewRequest("GET", "http://localhost:8080/api/lives", nil)
	r.Host = "localhost:8080"
	assert.False(t, isSameOriginRequest(r), "无浏览器 header 不应被判定为同源")
}

func TestStripDefaultPort(t *testing.T) {
	assert.Equal(t, "localhost", stripDefaultPort("localhost:80"))
	assert.Equal(t, "localhost", stripDefaultPort("localhost:443"))
	assert.Equal(t, "localhost:8080", stripDefaultPort("localhost:8080"))
	assert.Equal(t, "localhost", stripDefaultPort("localhost"))
}
