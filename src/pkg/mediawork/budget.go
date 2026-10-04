// Package mediawork 为录后 MP4 和按需 HLS 提供共同的重媒体任务预算。
package mediawork

import (
	"context"
	"sync"
)

var Shared Budget

type Budget struct {
	mu      sync.Mutex
	active  int
	changed chan struct{}
}

// 外层持久化队列和 HLS 队列负责有界排队，本层限制两者合计并发。
func (b *Budget) Acquire(ctx context.Context, limit int) (func(), error) {
	if limit < 1 {
		limit = 3
	}
	for {
		b.mu.Lock()
		if err := ctx.Err(); err != nil {
			b.mu.Unlock()
			return nil, err
		}
		if b.active < limit {
			b.active++
			b.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					b.mu.Lock()
					defer b.mu.Unlock()
					b.active--
					if b.changed != nil {
						close(b.changed)
						b.changed = nil
					}
				})
			}, nil
		}
		if b.changed == nil {
			b.changed = make(chan struct{})
		}
		changed := b.changed
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}
