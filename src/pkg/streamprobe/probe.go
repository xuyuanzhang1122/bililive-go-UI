package streamprobe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bililive-go/bililive-go/src/pkg/livelogger"
	"github.com/bililive-go/bililive-go/src/pkg/proxy"
)

const (
	// maxProbeTags 探测阶段最多读取的 tag 数量
	maxProbeTags = 30
)

// Config StreamProbe 配置
type Config struct {
	// UpstreamURL 上游直播流 URL
	UpstreamURL *url.URL

	// Headers 下载用 HTTP headers（如 Cookie、Referer 等）
	Headers map[string]string

	// OnProbed 探测完成回调
	// 在成功解析到流信息后调用
	OnProbed func(info *StreamHeaderInfo)

	// OnProbeError 探测错误回调（不影响流转发）
	// 仅用于日志记录，不应阻止录制
	OnProbeError func(err error, msg string)

	// Logger 日志记录器
	Logger *livelogger.LiveLogger
}

// StreamProbe 直播流探测代理
// 连接上游直播流，解析头部信息，然后在本地提供代理服务
type StreamProbe struct {
	config Config

	// 本地代理服务器
	listener  net.Listener
	localURL  *url.URL
	server    *http.Server
	serverErr chan error

	// 上游连接
	upstreamResp *http.Response
	upstreamBody io.ReadCloser

	// 缓冲数据：探测阶段已读取但需要转发给下载器的数据
	probedData   []byte
	probedDataMu sync.Mutex

	// 探测结果
	headerInfo atomic.Pointer[StreamHeaderInfo]

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 连接状态
	connected   atomic.Bool
	clientReady chan struct{} // 客户端（下载器）已连接
}

// New 创建一个新的 StreamProbe
func New(cfg Config) *StreamProbe {
	return &StreamProbe{
		config:      cfg,
		serverErr:   make(chan error, 1),
		clientReady: make(chan struct{}),
	}
}

// Start 启动探测代理
// 1. 连接上游流并解析头部信息
// 2. 在本地随机端口启动 HTTP 代理
// 3. 通过回调返回解析到的流信息
func (p *StreamProbe) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// 1. 连接上游流
	if err := p.connectUpstream(); err != nil {
		p.cancel()
		return fmt.Errorf("连接上游流失败: %w", err)
	}

	// 2. 探测流头信息
	p.probeStreamHeader()

	// 3. 启动本地代理
	if err := p.startLocalServer(); err != nil {
		p.cleanup()
		return fmt.Errorf("启动本地代理失败: %w", err)
	}

	return nil
}

// Stop 停止探测代理
func (p *StreamProbe) Stop() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cleanup()
}

// LocalURL 返回本地代理的 URL
// 下载器应使用此 URL 作为输入
func (p *StreamProbe) LocalURL() *url.URL {
	return p.localURL
}

// GetHeaderInfo 返回已解析的流头信息
func (p *StreamProbe) GetHeaderInfo() *StreamHeaderInfo {
	return p.headerInfo.Load()
}

// connectUpstream 连接上游直播流
func (p *StreamProbe) connectUpstream() error {
	// 创建带下载代理的 HTTP 客户端
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	proxy.ApplyDownloadProxyToTransport(transport)

	client := &http.Client{
		Transport: transport,
		Timeout:   0, // 不设超时，直播流是持久连接
	}

	req, err := http.NewRequestWithContext(p.ctx, http.MethodGet, p.config.UpstreamURL.String(), nil)
	if err != nil {
		return err
	}

	// 设置下载 headers
	for k, v := range p.config.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("上游返回 HTTP %d", resp.StatusCode)
	}

	p.upstreamResp = resp
	p.upstreamBody = resp.Body
	return nil
}

// probeStreamHeader 探测 FLV 流头信息
func (p *StreamProbe) probeStreamHeader() {
	// 判断是否为 FLV 流
	if p.isFLVStream() {
		info, bufferedData, err := parseFLVStreamInfo(p.upstreamBody, maxProbeTags)
		if err != nil {
			p.notifyError(err, "FLV 流头解析失败")
			// 如果是 ErrNotFLV，说明不是 FLV 格式，尝试作为原始数据通过
			if errors.Is(err, ErrNotFLV) {
				// 保留已读取的数据
				p.probedData = bufferedData
			}
			return
		}

		p.probedData = bufferedData
		p.headerInfo.Store(info)
		p.notifyProbed(info)
	} else if p.isHLSStream() {
		// HLS 流：探测第一个分段的信息
		info := p.probeHLSStream()
		if info != nil {
			p.headerInfo.Store(info)
			p.notifyProbed(info)
		}
		// HLS 不需要代理转发，但为了架构统一仍然提供代理
		// 代理直接转发上游响应
	} else {
		p.notifyError(nil, "不支持的流格式，跳过头部解析")
	}
}

// probeHLSStream 探测 HLS 流的信息（通过解析第一个 TS 分段头部）
// 目前为简化实现，只设置基本信息
func (p *StreamProbe) probeHLSStream() *StreamHeaderInfo {
	// TODO: 实现 HLS 流探测
	// 1. 下载 m3u8
	// 2. 获取第一个 TS 分段 URL
	// 3. 下载 TS 分段头部（前几个 TS packet）
	// 4. 解析 PAT/PMT -> PES -> SPS
	info := &StreamHeaderInfo{
		Unsupported:    true,
		UnsupportedMsg: "HLS 流探测功能开发中",
	}
	return info
}

// isFLVStream 判断上游是否为 FLV 流
func (p *StreamProbe) isFLVStream() bool {
	urlPath := strings.ToLower(p.config.UpstreamURL.Path)
	if strings.Contains(urlPath, ".flv") {
		return true
	}
	// 检查 Content-Type
	if p.upstreamResp != nil {
		ct := strings.ToLower(p.upstreamResp.Header.Get("Content-Type"))
		if strings.Contains(ct, "flv") || strings.Contains(ct, "x-flv") {
			return true
		}
	}
	return false
}

// isHLSStream 判断上游是否为 HLS 流
func (p *StreamProbe) isHLSStream() bool {
	urlPath := strings.ToLower(p.config.UpstreamURL.Path)
	if strings.Contains(urlPath, ".m3u8") || strings.HasSuffix(urlPath, ".m3u") {
		return true
	}
	if p.upstreamResp != nil {
		ct := strings.ToLower(p.upstreamResp.Header.Get("Content-Type"))
		if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "x-mpegurl") {
			return true
		}
	}
	return false
}

// startLocalServer 在本地随机端口启动 HTTP 代理
func (p *StreamProbe) startLocalServer() error {
	var err error
	p.listener, err = net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}

	addr := p.listener.Addr().(*net.TCPAddr)
	p.localURL, _ = url.Parse(fmt.Sprintf("http://127.0.0.1:%d/stream", addr.Port))

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", p.handleStreamRequest)

	p.server = &http.Server{
		Handler:     mux,
		ReadTimeout: 5 * time.Second,
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		if err := p.server.Serve(p.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			p.serverErr <- err
		}
	}()

	return nil
}

// handleStreamRequest 处理下载器的 HTTP 请求
// 将探测阶段缓冲的数据 + 后续上游数据一起返回
// 注意：只允许一个下载器连接，多个连接会导致 upstreamBody 数据竞争
func (p *StreamProbe) handleStreamRequest(w http.ResponseWriter, r *http.Request) {
	// 使用 CAS 确保只有一个客户端可以消费上游数据
	if !p.connected.CompareAndSwap(false, true) {
		http.Error(w, "此代理已被另一个下载器连接", http.StatusConflict)
		return
	}

	// 通知客户端已连接
	select {
	case <-p.clientReady:
	default:
		close(p.clientReady)
	}

	// 设置响应头
	if p.upstreamResp != nil {
		for k, vs := range p.upstreamResp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	w.Header().Set("Connection", "close")

	// 设置 Flusher 以实时推送数据
	flusher, hasFlusher := w.(http.Flusher)

	// 1. 先发送探测阶段缓冲的数据
	p.probedDataMu.Lock()
	buffered := p.probedData
	p.probedDataMu.Unlock()

	if len(buffered) > 0 {
		if _, err := w.Write(buffered); err != nil {
			return
		}
		if hasFlusher {
			flusher.Flush()
		}
	}

	// 2. 转发上游后续数据
	if p.upstreamBody == nil {
		return
	}

	buf := make([]byte, 32*1024) // 32KB 缓冲区
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-r.Context().Done():
			return
		default:
		}

		n, err := p.upstreamBody.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return
			}
			if hasFlusher {
				flusher.Flush()
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
				p.notifyError(err, "上游流读取中断")
			}
			return
		}
	}
}

// cleanup 清理资源
func (p *StreamProbe) cleanup() {
	if p.server != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		p.server.Shutdown(shutdownCtx)
	}
	if p.upstreamBody != nil {
		p.upstreamBody.Close()
	}
	p.wg.Wait()
}

// notifyProbed 通知探测完成
func (p *StreamProbe) notifyProbed(info *StreamHeaderInfo) {
	if p.config.OnProbed != nil {
		p.config.OnProbed(info)
	}
}

// notifyError 通知探测错误
func (p *StreamProbe) notifyError(err error, msg string) {
	if p.config.OnProbeError != nil {
		p.config.OnProbeError(err, msg)
	}
}

// IsStreamFLV 判断给定的 URL 是否看起来像 FLV 流
// 这是一个工具函数，供外部判断是否应该使用 StreamProbe
func IsStreamFLV(u *url.URL) bool {
	return strings.Contains(strings.ToLower(u.Path), ".flv")
}

// IsStreamHLS 判断给定的 URL 是否看起来像 HLS 流
func IsStreamHLS(u *url.URL) bool {
	path := strings.ToLower(u.Path)
	return strings.Contains(path, ".m3u8") || strings.HasSuffix(path, ".m3u")
}

// WaitForClient 等待下载器连接到代理
// 返回 true 表示客户端已连接，false 表示超时或 context 取消
func (p *StreamProbe) WaitForClient(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-p.clientReady:
		return true
	case <-timer.C:
		return false
	case <-p.ctx.Done():
		return false
	}
}
