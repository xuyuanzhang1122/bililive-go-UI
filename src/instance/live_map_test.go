package instance

import (
	"testing"

	"github.com/bililive-go/bililive-go/src/live"
	"github.com/bililive-go/bililive-go/src/types"
)

// TestLiveMap_ZeroValue 验证 LiveMap 的零值可以安全使用。
// 这保证了 Instance 通过 new() 创建后，无需显式初始化 Lives 字段，
// 启动早期 HTTP handler 和各类 manager 就可以安全调用方法。
func TestLiveMap_ZeroValue(t *testing.T) {
	var lm LiveMap // 零值，未经任何初始化

	t.Run("Len", func(t *testing.T) {
		if lm.Len() != 0 {
			t.Errorf("Len on zero LiveMap should return 0, got %d", lm.Len())
		}
	})

	t.Run("Has", func(t *testing.T) {
		if lm.Has("test") {
			t.Error("Has on zero LiveMap should return false")
		}
	})

	t.Run("Get", func(t *testing.T) {
		v, ok := lm.Get("test")
		if ok || v != nil {
			t.Errorf("Get on zero LiveMap should return (nil, false), got (%v, %v)", v, ok)
		}
	})

	t.Run("Range", func(t *testing.T) {
		called := false
		lm.Range(func(id types.LiveID, l live.Live) bool {
			called = true
			return true
		})
		if called {
			t.Error("Range on zero LiveMap should not call the callback")
		}
	})

	t.Run("Snapshot", func(t *testing.T) {
		snap := lm.Snapshot()
		if snap == nil {
			t.Error("Snapshot on zero LiveMap should return non-nil empty map")
		}
		if len(snap) != 0 {
			t.Errorf("Snapshot on zero LiveMap should return empty map, got %d entries", len(snap))
		}
	})

	t.Run("Delete", func(t *testing.T) {
		lm.Delete("test") // 不应 panic（delete on nil map 在 Go 中是安全的）
	})

	t.Run("SetIfAbsent", func(t *testing.T) {
		if lm.SetIfAbsent("test", nil) != true {
			t.Error("SetIfAbsent on zero LiveMap should succeed (lazy init)")
		}
		// 清理
		lm.Delete("test")
	})
}

// TestLiveMap_BasicOperations 验证基本的 CRUD 操作。
func TestLiveMap_BasicOperations(t *testing.T) {
	var lm LiveMap

	if lm.Len() != 0 {
		t.Fatalf("new LiveMap should be empty, got Len=%d", lm.Len())
	}

	// Set 触发懒初始化
	lm.Set("id1", nil)
	if lm.Len() != 1 {
		t.Fatalf("after Set, Len should be 1, got %d", lm.Len())
	}

	if !lm.Has("id1") {
		t.Error("Has should return true for existing key")
	}

	if lm.Has("id2") {
		t.Error("Has should return false for non-existent key")
	}

	// Delete
	lm.Delete("id1")
	if lm.Len() != 0 {
		t.Fatalf("after Delete, Len should be 0, got %d", lm.Len())
	}

	// SetIfAbsent
	if !lm.SetIfAbsent("id2", nil) {
		t.Error("SetIfAbsent should return true for new key")
	}
	if lm.SetIfAbsent("id2", nil) {
		t.Error("SetIfAbsent should return false for existing key")
	}

	// Snapshot
	snap := lm.Snapshot()
	if len(snap) != 1 {
		t.Errorf("Snapshot should contain 1 entry, got %d", len(snap))
	}
}

// TestLiveMap_ReplaceKey 验证原子替换操作。
func TestLiveMap_ReplaceKey(t *testing.T) {
	var lm LiveMap

	lm.Set("old", nil)
	if !lm.Has("old") {
		t.Fatal("old key should exist")
	}

	lm.ReplaceKey("old", "new", nil)

	if lm.Has("old") {
		t.Error("old key should be removed after ReplaceKey")
	}
	if !lm.Has("new") {
		t.Error("new key should exist after ReplaceKey")
	}
	if lm.Len() != 1 {
		t.Errorf("Len should be 1 after ReplaceKey, got %d", lm.Len())
	}
}
