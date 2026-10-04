package recordingcatalog

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Manager 串行后台核验，事件只唤醒队列，不在 HTTP 请求中扫描媒体。
type Manager struct {
	store    *Store
	interval time.Duration
	wake     chan struct{}
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

func NewManager(store *Store, interval time.Duration) *Manager {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Manager{store: store, interval: interval, wake: make(chan struct{}, 1)}
}
func (m *Manager) Start(ctx context.Context) {
	ctx, m.cancel = context.WithCancel(ctx)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()
		for {
			if err := m.store.Reconcile(ctx); err != nil && ctx.Err() == nil {
				logrus.WithError(err).Warn("录播目录后台核验失败")
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-m.wake:
			}
		}
	}()
}
func (m *Manager) Notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) MatchesRoot(root string) bool               { return m.store.MatchesRoot(root) }
func (m *Manager) Rooms(ctx context.Context) ([]Room, error)  { return m.store.Rooms(ctx) }
func (m *Manager) Status(ctx context.Context) (string, error) { return m.store.Status(ctx) }
func (m *Manager) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	m.wg.Wait()
	return m.store.Close()
}

func (s *Store) Status(ctx context.Context) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM catalog_meta WHERE key='status'").Scan(&status)
	return status, err
}
