package urlresolver

import (
	"context"
	"errors"
	"testing"
)

func TestExtractCandidateURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "分享文案中的完整短链",
			raw:  "快来看看 https://v.douyin.com/abc123/ 复制此链接",
			want: "https://v.douyin.com/abc123/",
		},
		{
			name: "无协议抖音短链",
			raw:  "v.douyin.com/abc123/",
			want: "https://v.douyin.com/abc123/",
		},
		{
			name: "清理中文标点",
			raw:  "https://live.douyin.com/12345678，",
			want: "https://live.douyin.com/12345678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractCandidateURL(tt.raw)
			if err != nil {
				t.Fatalf("extractCandidateURL() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("extractCandidateURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeDouyinURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "路径房间号",
			raw:  "https://live.douyin.com/12345678?foo=bar",
			want: "https://live.douyin.com/12345678",
			ok:   true,
		},
		{
			name: "query 房间号",
			raw:  "https://live.douyin.com/share?web_rid=87654321",
			want: "https://live.douyin.com/87654321",
			ok:   true,
		},
		{
			name: "webcast 不直接转换",
			raw:  "https://webcast.amemv.com/douyin/webcast/reflow/12345678",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := normalizeDouyinURL(tt.raw)
			if ok != tt.ok {
				t.Fatalf("normalizeDouyinURL() ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("normalizeDouyinURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalFromHTML(t *testing.T) {
	got, ok := canonicalFromHTML(`window.__ROOM__={"web_rid":"12345678"}`)
	if !ok {
		t.Fatal("canonicalFromHTML() ok = false, want true")
	}
	if got != "https://live.douyin.com/12345678" {
		t.Fatalf("canonicalFromHTML() = %q", got)
	}
}

func TestDouyinResolverRejectsUnsupportedURL(t *testing.T) {
	_, err := NewDouyinResolver().Resolve(context.Background(), "https://notdouyin.com/live/123")
	if !errors.Is(err, ErrUnsupportedURL) {
		t.Fatalf("Resolve() error = %v, want ErrUnsupportedURL", err)
	}
}

func TestHeadlessResolverCanBeDisabled(t *testing.T) {
	t.Setenv("BILILIVE_DOUYIN_HEADLESS", "0")
	_, err := resolveWithHeadlessBrowser(context.Background(), "https://v.douyin.com/abc123/")
	if !errors.Is(err, errHeadlessBrowserUnavailable) {
		t.Fatalf("resolveWithHeadlessBrowser() error = %v, want errHeadlessBrowserUnavailable", err)
	}
}
