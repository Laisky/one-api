package model

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/songquanpeng/one-api/common"
)

func TestRunWithSQLiteBusyRetryEventualSuccess(t *testing.T) {
	prev := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		common.UsingSQLite.Store(prev)
	})

	attempts := 0
	err := runWithSQLiteBusyRetry(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked")
		}
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 3, attempts)
}

func TestRunWithSQLiteBusyRetryContextCanceled(t *testing.T) {
	prev := common.UsingSQLite.Load()
	common.UsingSQLite.Store(true)
	t.Cleanup(func() {
		common.UsingSQLite.Store(prev)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	attempts := 0
	go func() {
		time.Sleep(sqliteBusyRetryBaseDelay / 2)
		cancel()
	}()

	err := runWithSQLiteBusyRetry(ctx, func() error {
		attempts++
		return errors.New("database is locked")
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
	require.GreaterOrEqual(t, attempts, 1)
}

func TestRunWithSQLiteBusyRetrySkipsWhenNotSQLite(t *testing.T) {
	prev := common.UsingSQLite.Load()
	common.UsingSQLite.Store(false)
	t.Cleanup(func() {
		common.UsingSQLite.Store(prev)
	})

	attempts := 0
	err := runWithSQLiteBusyRetry(context.Background(), func() error {
		attempts++
		return errors.New("database is locked")
	})

	require.Error(t, err)
	require.Equal(t, 1, attempts)
}

func TestShouldRetrySQLiteBusy(t *testing.T) {
	t.Parallel()
	require.True(t, shouldRetrySQLiteBusy(errors.New("database is locked")))
	require.True(t, shouldRetrySQLiteBusy(errors.New("database table is locked")))
	require.True(t, shouldRetrySQLiteBusy(errors.New("database is busy")))
	require.False(t, shouldRetrySQLiteBusy(errors.New("constraint failed")))
	require.False(t, shouldRetrySQLiteBusy(nil))
}
