package instance

import (
	"sync"

	"github.com/bililive-go/bililive-go/src/live"
	"github.com/bililive-go/bililive-go/src/types"
)

// LiveMap 是一个并发安全的 Live 对象 map。
// 解决了 inst.Lives 在 HTTP handler goroutine 中迭代、
// 同时在事件处理器 goroutine 中写入导致的 concurrent map read/write panic。
//
// 所有公开方法对 nil receiver 安全：当 LiveMap 指针为 nil 时，
// 读方法返回零值（0/false/nil），写方法为 no-op。
// 这保证了即使 inst.Lives 尚未初始化（例如启动早期），
// HTTP handler 和各类 manager 调用时也不会触发 nil 指针 panic。
type LiveMap struct {
	mu sync.RWMutex
	m  map[types.LiveID]live.Live
}

// NewLiveMap 创建一个新的 LiveMap。
func NewLiveMap() *LiveMap {
	return &LiveMap{
		m: make(map[types.LiveID]live.Live),
	}
}

// Get 根据 LiveID 获取 Live 对象。
// nil receiver 安全：返回 (nil, false)。
func (lm *LiveMap) Get(id types.LiveID) (live.Live, bool) {
	if lm == nil {
		return nil, false
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	v, ok := lm.m[id]
	return v, ok
}

// Set 设置一个 Live 对象。
// nil receiver 安全：no-op。
func (lm *LiveMap) Set(id types.LiveID, l live.Live) {
	if lm == nil {
		return
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.m[id] = l
}

// Delete 删除一个 Live 对象。
// nil receiver 安全：no-op。
func (lm *LiveMap) Delete(id types.LiveID) {
	if lm == nil {
		return
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	delete(lm.m, id)
}

// Has 检查是否存在指定 LiveID。
// nil receiver 安全：返回 false。
func (lm *LiveMap) Has(id types.LiveID) bool {
	if lm == nil {
		return false
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	_, ok := lm.m[id]
	return ok
}

// Len 返回 map 中元素的数量。
// nil receiver 安全：返回 0。
func (lm *LiveMap) Len() int {
	if lm == nil {
		return 0
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return len(lm.m)
}

// Range 遍历 map 中的所有元素。
// 回调函数返回 false 时停止遍历。
// nil receiver 安全：直接返回，不调用回调。
//
// 警告：在回调函数执行期间持有读锁。回调中不得调用 Set、Delete、
// ReplaceKey 等写操作，否则会导致死锁（sync.RWMutex 不可重入）。
// 如果需要在遍历过程中修改 map，请使用 Snapshot() 获取快照后再操作。
func (lm *LiveMap) Range(fn func(id types.LiveID, l live.Live) bool) {
	if lm == nil {
		return
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	for id, l := range lm.m {
		if !fn(id, l) {
			break
		}
	}
}

// Snapshot 返回 map 的一个浅拷贝快照。
// 适用于需要长时间处理的场景（避免长时间持有读锁），
// 或者需要在遍历过程中修改 LiveMap 的场景。
// nil receiver 安全：返回空 map（非 nil）。
func (lm *LiveMap) Snapshot() map[types.LiveID]live.Live {
	if lm == nil {
		return make(map[types.LiveID]live.Live)
	}
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	snapshot := make(map[types.LiveID]live.Live, len(lm.m))
	for id, l := range lm.m {
		snapshot[id] = l
	}
	return snapshot
}

// SetIfAbsent 如果 key 不存在则设置，返回是否设置成功。
// nil receiver 安全：返回 false。
func (lm *LiveMap) SetIfAbsent(id types.LiveID, l live.Live) bool {
	if lm == nil {
		return false
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if _, ok := lm.m[id]; ok {
		return false
	}
	lm.m[id] = l
	return true
}

// ReplaceKey 原子地删除旧 key 并设置新 key。
// 用于 InitializingLive 完成初始化后替换 LiveID 的场景。
// nil receiver 安全：no-op。
func (lm *LiveMap) ReplaceKey(oldID types.LiveID, newID types.LiveID, l live.Live) {
	if lm == nil {
		return
	}
	lm.mu.Lock()
	defer lm.mu.Unlock()
	delete(lm.m, oldID)
	lm.m[newID] = l
}
