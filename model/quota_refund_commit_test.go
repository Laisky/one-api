package model

import (
	"context"
	"database/sql"
	"sync/atomic"
	"testing"

	"github.com/Laisky/errors/v2"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// refundLostAckPool delegates to the real SQLite pool but wraps transactions so
// one successful COMMIT reports a transport error to the caller.
type refundLostAckPool struct {
	*sql.DB
	lost *atomic.Bool
}

// BeginTx opens a real transaction with ctx/options and returns its fault wrapper.
func (p *refundLostAckPool) BeginTx(ctx context.Context, options *sql.TxOptions) (gorm.ConnPool, error) {
	tx, err := p.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, errors.Wrap(err, "begin lost-ack fixture transaction")
	}
	return &refundLostAckTx{Tx: tx, lost: p.lost}, nil
}

// GetDBConn returns the underlying pool for GORM's lifecycle operations.
func (p *refundLostAckPool) GetDBConn() (*sql.DB, error) { return p.DB, nil }

// refundLostAckTx reports an error only after the database has committed, to
// reproduce an ambiguous acknowledgement rather than a rolled-back statement.
type refundLostAckTx struct {
	*sql.Tx
	lost *atomic.Bool
}

// Commit commits the real transaction and loses exactly its first acknowledgement.
func (t *refundLostAckTx) Commit() error {
	if err := t.Tx.Commit(); err != nil {
		return errors.Wrap(err, "commit lost-ack fixture")
	}
	if t.lost.CompareAndSwap(false, true) {
		return errors.New("injected lost acknowledgement after durable commit")
	}
	return nil
}

// TestQuotaRefundLostCommitAcknowledgement proves a financial COMMIT can succeed
// while its caller sees an error; replay must observe the completion marker and
// must not credit either balance twice.
func TestQuotaRefundLostCommitAcknowledgement(t *testing.T) {
	_, intent := quotaRefundFixture(t)
	ctx := context.Background()
	require.NoError(t, EnsureQuotaRefund(ctx, intent))
	pool, err := DB.DB()
	require.NoError(t, err)
	original := DB.Statement.ConnPool
	var lost atomic.Bool
	DB.Statement.ConnPool = &refundLostAckPool{DB: pool, lost: &lost}
	t.Cleanup(func() { DB.Statement.ConnPool = original })
	require.ErrorContains(t, ApplyQuotaRefund(ctx, intent.ID), "lost acknowledgement")
	require.True(t, lost.Load())
	assertQuotaRefundBalance(t, 10000)
	var row QuotaRefund
	require.NoError(t, DB.Where("id = ?", intent.ID).Take(&row).Error)
	require.Equal(t, QuotaRefundCompleted, row.Status)
	require.NoError(t, ApplyQuotaRefund(ctx, intent.ID))
	require.NoError(t, RecoverQuotaRefunds(ctx))
	assertQuotaRefundBalance(t, 10000)
}
