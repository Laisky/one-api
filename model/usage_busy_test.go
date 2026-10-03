package model

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
)

// usageBusyFixture creates independent on-disk usage counters and restores process state after each test.
func usageBusyFixture(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "usage.db")+"?_busy_timeout=1&_journal_mode=WAL"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &Channel{}))
	pool, err := db.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(2)
	oldDB, oldBatch := DB, config.BatchUpdateEnabled
	DB, config.BatchUpdateEnabled = db, false
	t.Cleanup(func() {
		DB, config.BatchUpdateEnabled = oldDB, oldBatch
		require.NoError(t, pool.Close())
	})
	require.NoError(t, db.Create(&User{Id: 301, Username: "busy-usage", Quota: 9000, UsedQuota: 11, RequestCount: 2}).Error)
	require.NoError(t, db.Create(&Channel{Id: 302, Name: "busy-usage", UsedQuota: 13}).Error)
	return db
}

// TestUsageCountersRetrySQLiteLock verifies recovery from a real SQLITE_BUSY write, without doubling counters or changing balances.
func TestUsageCountersRetrySQLiteLock(t *testing.T) {
	for _, target := range []string{"users", "channels"} {
		t.Run(target, func(t *testing.T) {
			db := usageBusyFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			pool, err := db.DB()
			require.NoError(t, err)
			locker, err := pool.Conn(ctx)
			require.NoError(t, err)
			defer func() { require.NoError(t, locker.Close()) }()
			_, err = locker.ExecContext(ctx, "BEGIN IMMEDIATE")
			require.NoError(t, err)
			var attempts, busy atomic.Int32
			var released atomic.Bool
			defer func() {
				if !released.Load() {
					_, rollbackErr := locker.ExecContext(context.Background(), "ROLLBACK")
					require.NoError(t, rollbackErr)
				}
			}()
			require.NoError(t, db.Callback().Update().After("gorm:update").Before("gorm:commit_or_rollback_transaction").Register("usage:unlock_after_busy", func(tx *gorm.DB) {
				if tx.Statement.Table != target {
					return
				}
				attempts.Add(1)
				if shouldRetrySQLiteBusy(tx.Error) && released.CompareAndSwap(false, true) {
					busy.Add(1)
					_, rollbackErr := locker.ExecContext(ctx, "ROLLBACK")
					require.NoError(t, rollbackErr)
				}
			}))
			if target == "users" {
				UpdateUserUsedQuotaAndRequestCountWithContext(ctx, 301, 37)
			} else {
				UpdateChannelUsedQuotaWithContext(ctx, 302, 37)
			}
			require.Equal(t, int32(1), busy.Load(), "must reproduce an actual SQLite lock")
			require.Equal(t, int32(2), attempts.Load(), "one failed write and one successful retry")
			var user User
			var channel Channel
			require.NoError(t, db.First(&user, 301).Error)
			require.NoError(t, db.First(&channel, 302).Error)
			require.Equal(t, int64(9000), user.Quota, "statistics must not debit financial balances")
			if target == "users" {
				require.Equal(t, int64(48), user.UsedQuota)
				require.Equal(t, 3, user.RequestCount)
				require.Equal(t, int64(13), channel.UsedQuota)
			} else {
				require.Equal(t, int64(11), user.UsedQuota)
				require.Equal(t, 2, user.RequestCount)
				require.Equal(t, int64(50), channel.UsedQuota)
			}
		})
	}
}

// TestUsageCountersDoNotReplayPermanentErrors verifies permanent failures are not retried or partially applied.
func TestUsageCountersDoNotReplayPermanentErrors(t *testing.T) {
	for _, target := range []string{"users", "channels"} {
		t.Run(target, func(t *testing.T) {
			db := usageBusyFixture(t)
			var attempts atomic.Int32
			require.NoError(t, db.Callback().Update().Before("gorm:update").Register("usage:permanent_failure", func(tx *gorm.DB) {
				if tx.Statement.Table == target {
					attempts.Add(1)
					tx.AddError(errors.New("injected permanent constraint failure"))
				}
			}))
			if target == "users" {
				UpdateUserUsedQuotaAndRequestCountWithContext(context.Background(), 301, 37)
			} else {
				UpdateChannelUsedQuotaWithContext(context.Background(), 302, 37)
			}
			require.Equal(t, int32(1), attempts.Load())
			var user User
			var channel Channel
			require.NoError(t, db.First(&user, 301).Error)
			require.NoError(t, db.First(&channel, 302).Error)
			require.Equal(t, int64(11), user.UsedQuota)
			require.Equal(t, 2, user.RequestCount)
			require.Equal(t, int64(13), channel.UsedQuota)
		})
	}
}
