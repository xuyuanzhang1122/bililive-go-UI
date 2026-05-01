package tools

import (
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

var githubDownloadMirrors = []string{
	"https://ghfast.top/",
	"https://gh-proxy.com/",
	"https://github.moeyy.xyz/",
}

func getRuntimeConfigData() ([]byte, error) {
	data, err := getConfigData()
	if err != nil {
		return nil, err
	}
	return addDownloadFallbacks(data)
}

func addDownloadFallbacks(data []byte) ([]byte, error) {
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	augmentDownloadURL(raw)

	out, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func augmentDownloadURL(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == "downloadUrl" {
				typed[key] = appendDownloadMirrors(value)
				continue
			}
			augmentDownloadURL(value)
		}
	case []any:
		for _, item := range typed {
			augmentDownloadURL(item)
		}
	}
}

func appendDownloadMirrors(v any) any {
	switch typed := v.(type) {
	case string:
		return appendMirrorURLs([]any{typed})
	case []any:
		return appendMirrorURLs(typed)
	case map[string]any:
		for key, value := range typed {
			typed[key] = appendDownloadMirrors(value)
		}
		return typed
	default:
		return v
	}
}

func appendMirrorURLs(urls []any) []any {
	seen := make(map[string]struct{}, len(urls)*2)
	out := make([]any, 0, len(urls)*3)
	for _, value := range urls {
		rawURL, ok := value.(string)
		if !ok || rawURL == "" {
			continue
		}
		appendUniqueURL(&out, seen, rawURL)
		for _, mirrorURL := range mirrorURLsFor(rawURL) {
			appendUniqueURL(&out, seen, mirrorURL)
		}
	}
	return out
}

func appendUniqueURL(out *[]any, seen map[string]struct{}, rawURL string) {
	if _, ok := seen[rawURL]; ok {
		return
	}
	seen[rawURL] = struct{}{}
	*out = append(*out, rawURL)
}

func mirrorURLsFor(rawURL string) []string {
	if strings.Contains(rawURL, "bililive-go.com/remotetools/download") {
		if parsed, err := url.Parse(rawURL); err == nil {
			if upstream := parsed.Query().Get("downloadurl"); upstream != "" {
				return append([]string{upstream}, mirrorURLsFor(upstream)...)
			}
		}
		return nil
	}

	if !strings.HasPrefix(rawURL, "https://github.com/") {
		return nil
	}

	mirrors := make([]string, 0, len(githubDownloadMirrors))
	for _, mirror := range githubDownloadMirrors {
		mirrors = append(mirrors, mirror+rawURL)
	}
	return mirrors
}

func applyDownloadProxyEnvForRemoteTools() {
	for _, env := range proxy.GetDownloadProxyEnvVars() {
		key, value, ok := strings.Cut(env, "=")
		if ok {
			_ = os.Setenv(key, value)
		}
	}
}
