package pipeline

import (
	"context"
	"database/sql"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestPreparationStatusReadDoesNotCreateOrChangeJobs(t *testing.T) {
	ctx := context.Background()
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tasks.db"))
	require.NoError(t, err)
	defer s.Close()
	key := PreparationKey{"id", "source", "profile"}
	_, err = s.PreparationStatus(ctx, key)
	require.ErrorIs(t, err, sql.ErrNoRows)
	_, err = s.QueuePreparation(ctx, key)
	require.NoError(t, err)
	j, err := s.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationQueued, j.Status)
	require.NoError(t, s.ClaimPreparation(ctx, key))
	require.NoError(t, s.FinishPreparation(ctx, key, PreparationFailed, "", "actual failure"))
	j, err = s.PreparationStatus(ctx, key)
	require.NoError(t, err)
	require.Equal(t, PreparationFailed, j.Status)
	require.Equal(t, "actual failure", j.Reason)
}
