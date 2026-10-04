package recordingcatalog

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
	"time"
)

func TestManagerReconcilesStartupAndCoalescedEvents(t *testing.T) {
	store, _, source := catalogFixture(t)
	manager := NewManager(store, time.Hour)
	manager.Start(context.Background())
	defer manager.Close()
	require.Eventually(t, func() bool {
		rooms, err := manager.Rooms(context.Background())
		return err == nil && len(rooms) == 1 && rooms[0].TotalSize == int64(len("source recording"))
	}, 3*time.Second, 10*time.Millisecond)
	require.NoError(t, os.WriteFile(source, []byte("changed recording contents"), 0600))
	for i := 0; i < 100; i++ {
		manager.Notify()
	}
	require.Eventually(t, func() bool {
		rooms, err := manager.Rooms(context.Background())
		return err == nil && len(rooms) == 1 && rooms[0].TotalSize == int64(len("changed recording contents"))
	}, 3*time.Second, 10*time.Millisecond)
}
