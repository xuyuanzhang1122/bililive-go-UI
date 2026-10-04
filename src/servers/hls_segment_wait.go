package servers

import (
	"context"
	"fmt"
	"os"
	"time"
)

// 未来分片最多等一个轮询周期；等待读租约保护目录，取消只结束请求。
func waitHLSSegment(ctx context.Context, job *hlsJob, path string) (*os.File, error) {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() || info.Size() == 0 {
				return nil, fmt.Errorf("HLS 分片不是完整普通文件")
			}
			f, err := os.Open(path)
			if err != nil {
				return nil, err
			}
			opened, err := f.Stat()
			if err != nil || !os.SameFile(info, opened) {
				f.Close()
				return nil, fmt.Errorf("HLS 分片打开期间变化")
			}
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		if job == nil {
			return nil, fmt.Errorf("HLS 作业已失效")
		}
		status, message := job.GetStatus()
		if status == hlsJobFailed || status == hlsJobPartial {
			return nil, fmt.Errorf("HLS 转换失败: %s", message)
		}
		if status == hlsJobCompleted {
			return nil, os.ErrNotExist
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
			return nil, fmt.Errorf("HLS 分片尚未生成，请稍后重试")
		case <-ticker.C:
		}
	}
}
