package model

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// asyncLostCommitPool delegates real SQL transactions but can lose one COMMIT
// acknowledgement AFTER the underlying database has committed its writes.
type asyncLostCommitPool struct {
	gorm.ConnPool
	lose atomic.Bool
}

// BeginTx returns a real transaction wrapped only at the commit boundary.
func (p *asyncLostCommitPool) BeginTx(ctx context.Context, opts *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.ConnPool.(gorm.TxBeginner).BeginTx(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "begin lost-ack fixture transaction")
	}
	return &asyncLostCommitTx{Tx: tx, parent: p}, nil
}

// asyncLostCommitTx injects an acknowledgement loss without rolling back money.
type asyncLostCommitTx struct {
	*sql.Tx
	parent *asyncLostCommitPool
}

// Commit commits durable state before returning the injected transport error.
func (tx *asyncLostCommitTx) Commit() error {
	if err := tx.Tx.Commit(); err != nil {
		return errors.Wrap(err, "commit lost-ack fixture transaction")
	}
	if tx.parent.lose.CompareAndSwap(true, false) {
		return errors.New("injected lost commit acknowledgement")
	}
	return nil
}

// injectAsyncLostCommit installs a reversible SQL wrapper on the selected handle.
func injectAsyncLostCommit(t *testing.T, db *gorm.DB) *asyncLostCommitPool {
	t.Helper()
	pool := &asyncLostCommitPool{ConnPool: db.ConnPool}
	oldPool, oldStatement := db.ConnPool, db.Statement.ConnPool
	db.ConnPool, db.Statement.ConnPool = pool, pool
	t.Cleanup(func() { db.ConnPool, db.Statement.ConnPool = oldPool, oldStatement })
	return pool
}

// TestAsyncBillingLostCommitAcknowledgements checks real committed reservation,
// settlement, refund and split-log delivery after a lost database acknowledgement.
func TestAsyncBillingLostCommitAcknowledgements(t *testing.T) {
	for _, stage := range []string{"reservation", "settlement", "refund", "log"} {
		t.Run(stage, func(t *testing.T) {
			_, input := durableJobTestStore(t)
			target := DB
			if stage == "log" {
				target = LOG_DB
			}
			pool := injectAsyncLostCommit(t, target)
			if stage == "reservation" {
				pool.lose.Store(true)
			}
			task, _, err := ReserveAsyncTask(context.Background(), input)
			require.NoError(t, err)
			require.NotNil(t, task)
			if stage == "settlement" || stage == "refund" {
				lease, err := ClaimAsyncTask(context.Background(), time.Now())
				require.NoError(t, err)
				pool.lose.Store(true)
				update := AsyncTaskUpdate{State: AsyncTaskCompleted, ResultJSON: `{"videos":[{"url":"https://media.example/v.mp4"}]}`}
				if stage == "refund" {
					update = AsyncTaskUpdate{State: AsyncTaskFailed, Refund: true}
				}
				require.Error(t, ApplyAsyncTaskUpdate(context.Background(), lease, update))
				require.ErrorIs(t, ApplyAsyncTaskUpdate(context.Background(), lease, update), ErrAsyncLeaseLost)
			}
			if stage == "log" {
				completeDurableJob(t, task)
				pool.lose.Store(true)
			}
			for range 3 {
				require.NoError(t, FlushAsyncTaskLogs(context.Background()))
			}
			user, token, _ := asyncBillingBalances(t, task)
			debit := int64(200000)
			if stage == "refund" {
				debit = 0
			}
			require.EqualValues(t, 1000000-debit, user.Quota)
			require.EqualValues(t, 1000000-debit, token.RemainQuota)
			var rows int64
			require.NoError(t, DB.Model(&AsyncTask{}).Count(&rows).Error)
			require.EqualValues(t, 1, rows)
			require.NoError(t, LOG_DB.Model(&Log{}).Count(&rows).Error)
			require.EqualValues(t, 1, rows)
		})
	}
}
