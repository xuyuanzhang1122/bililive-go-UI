package urlresolver

import (
	"context"
	"errors"
)

var (
	// ErrNoURL 表示输入中没有可解析的 URL。
	ErrNoURL = errors.New("未找到可解析的 URL")
	// ErrUnsupportedURL 表示当前阶段仅支持抖音直播间链接。
	ErrUnsupportedURL = errors.New("当前仅支持抖音直播间链接")
	// ErrUnresolved 表示请求成功但没有得到稳定的 live.douyin.com 房间地址。
	ErrUnresolved = errors.New("无法解析为稳定的抖音直播间地址")
)

// Resolver 将分享文案、短链或跳转链接转换为标准直播间地址。
type Resolver interface {
	Resolve(ctx context.Context, raw string) (string, error)
}

// Resolve 使用当前阶段的默认 resolver。
func Resolve(ctx context.Context, raw string) (string, error) {
	return NewDouyinResolver().Resolve(ctx, raw)
}
