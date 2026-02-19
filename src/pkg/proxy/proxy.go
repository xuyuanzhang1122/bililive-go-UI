// Package proxy 提供代理配置和管理功能
// 支持通用代理、信息获取代理和下载代理三个层级
// 优先级：专用代理（如果设置）> 通用代理 > 系统环境变量
package proxy

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/bililive-go/bililive-go/src/configs"
	"golang.org/x/net/proxy"
)

// proxyScope 代理用途
type proxyScope int

const (
	// scopeGeneral 通用代理
	scopeGeneral proxyScope = iota
	// scopeInfo 信息获取代理（获取直播间信息、平台 API 请求等）
	scopeInfo
	// scopeDownload 下载代理（下载直播流数据）
	scopeDownload
)

// getProxyURLForScope 获取指定用途的代理 URL
// 优先级：专用代理（如果设置）> 通用代理 > 环境变量
func getProxyURLForScope(scope proxyScope) string {
	cfg := configs.GetCurrentConfig()
	if cfg != nil && configs.EnableProxyConfig {
		// 先检查专用代理
		switch scope {
		case scopeInfo:
			if cfg.Proxy.InfoProxy != nil && cfg.Proxy.InfoProxy.Enable && cfg.Proxy.InfoProxy.URL != "" {
				return cfg.Proxy.InfoProxy.URL
			}
		case scopeDownload:
			if cfg.Proxy.DownloadProxy != nil && cfg.Proxy.DownloadProxy.Enable && cfg.Proxy.DownloadProxy.URL != "" {
				return cfg.Proxy.DownloadProxy.URL
			}
		}

		// 回退到通用代理
		if cfg.Proxy.Enable && cfg.Proxy.URL != "" {
			return cfg.Proxy.URL
		}
	}

	// 按优先级检查环境变量
	for _, envVar := range []string{"ALL_PROXY", "all_proxy", "HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy"} {
		if proxyURL := os.Getenv(envVar); proxyURL != "" {
			return proxyURL
		}
	}

	return ""
}

// GetProxyURL 获取通用代理 URL
// 优先级：配置文件 > 环境变量 (ALL_PROXY > HTTPS_PROXY > HTTP_PROXY)
func GetProxyURL() string {
	return getProxyURLForScope(scopeGeneral)
}

// GetInfoProxyURL 获取信息获取代理 URL
// 用于获取直播间信息、平台 API 请求等
// 优先级：InfoProxy > 通用 Proxy > 环境变量
func GetInfoProxyURL() string {
	return getProxyURLForScope(scopeInfo)
}

// GetDownloadProxyURL 获取下载代理 URL
// 用于下载直播流数据
// 优先级：DownloadProxy > 通用 Proxy > 环境变量
func GetDownloadProxyURL() string {
	return getProxyURLForScope(scopeDownload)
}

// GetProxyFunc 返回 HTTP 代理函数（通用代理）
// 用于 http.Transport.Proxy 字段
func GetProxyFunc() func(*http.Request) (*url.URL, error) {
	return getProxyFuncForURL(GetProxyURL())
}

// IsSocks5Proxy 检查通用代理是否为 SOCKS5 代理
func IsSocks5Proxy() bool {
	return isSocks5(GetProxyURL())
}

// CreateSocks5DialContext 为 SOCKS5 代理创建 DialContext 函数
// 用于 http.Transport.DialContext 字段
func CreateSocks5DialContext(proxyURL string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		// 解析代理 URL
		parsedURL, err := url.Parse(proxyURL)
		if err != nil {
			return nil, err
		}

		// 构建 SOCKS5 认证信息
		var auth *proxy.Auth
		if parsedURL.User != nil {
			auth = &proxy.Auth{
				User: parsedURL.User.Username(),
			}
			if password, ok := parsedURL.User.Password(); ok {
				auth.Password = password
			}
		}

		// 创建 SOCKS5 dialer
		dialer, err := proxy.SOCKS5("tcp", parsedURL.Host, auth, proxy.Direct)
		if err != nil {
			return nil, err
		}

		// 连接目标
		return dialer.Dial(network, addr)
	}
}

// ApplyProxyToTransport 将通用代理设置应用到 http.Transport
func ApplyProxyToTransport(transport *http.Transport) {
	applyProxyURLToTransport(transport, GetProxyURL())
}

// ApplyInfoProxyToTransport 将信息获取代理设置应用到 http.Transport
// 用于获取直播间信息、平台 API 请求等
func ApplyInfoProxyToTransport(transport *http.Transport) {
	applyProxyURLToTransport(transport, GetInfoProxyURL())
}

// ApplyDownloadProxyToTransport 将下载代理设置应用到 http.Transport
// 用于下载直播流数据
func ApplyDownloadProxyToTransport(transport *http.Transport) {
	applyProxyURLToTransport(transport, GetDownloadProxyURL())
}

// GetProxyEnvVars 返回用于子进程的通用代理环境变量
// 可用于传递给外部工具（如 ffmpeg、dotnet 等）
func GetProxyEnvVars() []string {
	return getProxyEnvVarsForURL(GetProxyURL())
}

// GetDownloadProxyEnvVars 返回用于子进程的下载代理环境变量
// 可用于传递给下载相关的外部工具
func GetDownloadProxyEnvVars() []string {
	return getProxyEnvVarsForURL(GetDownloadProxyURL())
}

// ==================== 内部辅助函数 ====================

// isSocks5 检查代理 URL 是否为 SOCKS5 代理
func isSocks5(proxyURL string) bool {
	return strings.HasPrefix(proxyURL, "socks5://") || strings.HasPrefix(proxyURL, "socks5h://")
}

// getProxyFuncForURL 为指定的代理 URL 返回 HTTP 代理函数
func getProxyFuncForURL(proxyURL string) func(*http.Request) (*url.URL, error) {
	if proxyURL == "" {
		return nil
	}

	// SOCKS5 代理不能用于 HTTP Proxy 字段，需要通过 DialContext 处理
	if isSocks5(proxyURL) {
		return nil
	}

	// 解析代理 URL
	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil
	}

	return http.ProxyURL(parsedURL)
}

// applyProxyURLToTransport 将指定的代理 URL 应用到 http.Transport
func applyProxyURLToTransport(transport *http.Transport, proxyURL string) {
	if proxyURL == "" {
		// 使用系统默认代理（从环境变量）
		transport.Proxy = http.ProxyFromEnvironment
		return
	}

	if isSocks5(proxyURL) {
		// SOCKS5 代理需要通过 DialContext 处理
		transport.DialContext = CreateSocks5DialContext(proxyURL)
	} else {
		// HTTP/HTTPS 代理
		transport.Proxy = getProxyFuncForURL(proxyURL)
	}
}

// getProxyEnvVarsForURL 为指定的代理 URL 返回环境变量列表
func getProxyEnvVarsForURL(proxyURL string) []string {
	if proxyURL == "" {
		return nil
	}

	return []string{
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
	}
}
