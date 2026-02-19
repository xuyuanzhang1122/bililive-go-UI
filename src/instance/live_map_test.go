package instance

import (
	"testing"

	"github.com/bililive-go/bililive-go/src/live"
	"github.com/bililive-go/bililive-go/src/types"
)

// TestLiveMap_NilReceiver 验证所有方法在 nil receiver 上不会 panic。
// 这是为了防止 inst.Lives 尚未初始化时（启动早期），
// HTTP handler 和各类 manager 调用方法导致 nil 指针 panic。
func TestLiveMap_NilReceiver(t *testing.T) {
	var lm *LiveMap

	t.Run("Get", func(t *testing.T) {
		v, ok := lm.Get("test")
		if ok || v != nil {
			t.Errorf("Get on nil LiveMap should return (nil, false), got (%v, %v)", v, ok)
		}
	})

	t.Run("Has", func(t *testing.T) {
		if lm.Has("test") {
			t.Error("Has on nil LiveMap should return false")
		}
	})

	t.Run("Len", func(t *testing.T) {
		if lm.Len() != 0 {
			t.Errorf("Len on nil LiveMap should return 0, got %d", lm.Len())
		}
	})

	t.Run("Range", func(t *testing.T) {
		called := false
		lm.Range(func(id types.LiveID, l live.Live) bool {
			called = true
			return true
		})
		if called {
			t.Error("Range on nil LiveMap should not call the callback")
		}
	})

	t.Run("Snapshot", func(t *testing.T) {
		snap := lm.Snapshot()
		if snap == nil {
			t.Error("Snapshot on nil LiveMap should return non-nil empty map")
		}
		if len(snap) != 0 {
			t.Errorf("Snapshot on nil LiveMap should return empty map, got %d entries", len(snap))
		}
	})

	t.Run("SetIfAbsent", func(t *testing.T) {
		if lm.SetIfAbsent("test", nil) {
			t.Error("SetIfAbsent on nil LiveMap should return false")
		}
	})

	t.Run("Set", func(t *testing.T) {
		lm.Set("test", nil) // 不应 panic
	})

	t.Run("Delete", func(t *testing.T) {
		lm.Delete("test") // 不应 panic
	})

	t.Run("ReplaceKey", func(t *testing.T) {
		lm.ReplaceKey("old", "new", nil) // 不应 panic
	})
}

// TestLiveMap_BasicOperations 验证正常初始化后的基本操作。
func TestLiveMap_BasicOperations(t *testing.T) {
	lm := NewLiveMap()

	if lm.Len() != 0 {
		t.Fatalf("new LiveMap should be empty, got Len=%d", lm.Len())
	}

	if lm.Has("id1") {
		t.Error("Has should return false for non-existent key")
	}

	if _, ok := lm.Get("id1"); ok {
		t.Error("Get should return false for non-existent key")
	}

	// Snapshot 应该返回空 map
	snap := lm.Snapshot()
	if len(snap) != 0 {
		t.Errorf("Snapshot of empty LiveMap should be empty, got %d", len(snap))
	}
}
