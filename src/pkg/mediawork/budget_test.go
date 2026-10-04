package mediawork

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestBudgetSharesSlotsAndCancelsWait(t *testing.T) {
	b := &Budget{}
	release, err := b.Acquire(context.Background(), 1)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = b.Acquire(ctx, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	release()
	release() // 释放必须幂等。
	release, err = b.Acquire(context.Background(), 1)
	require.NoError(t, err)
	release()
}
