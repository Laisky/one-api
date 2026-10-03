package controller

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
)

// finalizationCancelPool wraps a real SQL pool to cancel the request only after
// its financial transaction successfully commits. It does not replace SQL work.
type finalizationCancelPool struct {
	*sql.DB
	cancel  context.CancelFunc
	commits *atomic.Int64
}

// BeginTx starts the real transaction with ctx and opts and returns a wrapper
// that observes successful commit. Database errors retain their original cause.
func (p *finalizationCancelPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "begin observed finalization transaction")
	}
	return &finalizationCancelTx{Tx: tx, cancel: p.cancel, commits: p.commits}, nil
}

// finalizationCancelTx delegates SQL work and rollback to its real transaction.
type finalizationCancelTx struct {
	*sql.Tx
	cancel  context.CancelFunc
	commits *atomic.Int64
}

// Commit performs the real commit before canceling the originating request.
// It returns the underlying error and never cancels an unsuccessful commit.
func (tx *finalizationCancelTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return errors.Wrap(err, "commit observed finalization transaction")
	}
	tx.commits.Add(1)
	tx.cancel()
	return nil
}

// finalizationCacheContextKey identifies a request value retained by cleanup.
type finalizationCacheContextKey struct{}

// finalizationRedisObserver observes cleanup context without changing Redis
// behavior; all cache operations still reach a real local Redis protocol server.
type finalizationRedisObserver struct {
	redis.Cmdable
	deletes   int
	ctxErr    error
	value     any
	remaining time.Duration
	bounded   bool
}

// Del captures the actual invalidation context and delegates keys to Redis.
// Its return value is the real Redis command result, including cancellation.
func (r *finalizationRedisObserver) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	r.deletes++
	r.ctxErr = ctx.Err()
	r.value = ctx.Value(finalizationCacheContextKey{})
	deadline, bounded := ctx.Deadline()
	r.bounded = bounded
	if bounded {
		r.remaining = time.Until(deadline)
	}
	return r.Cmdable.Del(ctx, keys...)
}

// TestExternalFinalizationCacheSurvivesRequestCancellation cancels exactly after
// the durable commit, then checks the real cache reader sees committed balances.
// t supplies isolated SQLite and Redis fixtures; no financial write is mocked.
func TestExternalFinalizationCacheSurvivesRequestCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		charge int64
	}{
		{"post_refund", model.TokenTransactionStatusConfirmed, 40},
		{"post_charge", model.TokenTransactionStatusConfirmed, 120},
		{"cancel", model.TokenTransactionStatusCanceled, 0},
		{"unchanged_charge", model.TokenTransactionStatusConfirmed, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanup, user, token := setupConsumeTokenTest(t)
			defer cleanup()
			txn := seedFinalizationReservation(t, user, token)
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
			defer func() { require.NoError(t, client.Close()) }()
			observer := &finalizationRedisObserver{Cmdable: client}
			oldRedis, oldEnabled := common.RDB, common.IsRedisEnabled()
			common.RDB = observer
			common.SetRedisEnabled(true)
			defer func() { common.RDB = oldRedis; common.SetRedisEnabled(oldEnabled) }()
			cached, err := model.CacheGetTokenByKey(context.Background(), token.Key)
			require.NoError(t, err)
			require.Equal(t, int64(900), cached.RemainQuota)
			require.True(t, server.Exists("token:"+token.Key), "positive control must warm the real cache")

			ctx, cancel := context.WithCancel(context.WithValue(context.Background(), finalizationCacheContextKey{}, "request-correlation"))
			defer cancel()
			baseDB := model.DB
			sqlDB, err := baseDB.DB()
			require.NoError(t, err)
			var commits atomic.Int64
			pool := &finalizationCancelPool{DB: sqlDB, cancel: cancel, commits: &commits}
			observedDB := baseDB.Session(&gorm.Session{NewDB: true, Context: context.Background()})
			observedDB.Config.ConnPool = pool
			observedDB.Statement.ConnPool = pool
			model.DB = observedDB
			defer func() { model.DB = baseDB }()
			result, err := model.FinalizePendingTokenTransaction(ctx, token.Id, txn.Id, model.TokenTransactionFinalization{
				Status: tc.status, FinalQuota: tc.charge, At: helper.GetTimestamp(),
			})
			require.NoError(t, err)
			require.Equal(t, int64(1), commits.Load(), "cancel must follow a real successful commit")
			require.ErrorIs(t, ctx.Err(), context.Canceled)
			require.Equal(t, tc.status, result.Status)
			require.Zero(t, result.ExpiresAt)
			assertFinalizationBalances(t, user.Id, token.Id, tc.charge)
			refreshed, err := model.CacheGetTokenByKey(context.Background(), token.Key)
			require.NoError(t, err)
			require.Equal(t, int64(1000)-tc.charge, refreshed.RemainQuota, "the public cache reader must observe the committed balance")
			require.Equal(t, tc.charge, refreshed.UsedQuota)
			if tc.charge == 100 {
				require.Zero(t, observer.deletes, "zero delta leaves the already-correct token cache intact")
				return
			}
			require.Equal(t, 1, observer.deletes)
			require.NoError(t, observer.ctxErr, "request cancellation must not abort committed cleanup")
			require.Equal(t, "request-correlation", observer.value)
			require.True(t, observer.bounded, "detached cleanup still needs an operational deadline")
			require.Greater(t, observer.remaining, time.Duration(0))
			require.LessOrEqual(t, observer.remaining, 5*time.Second)
		})
	}
}
