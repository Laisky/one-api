package model

import (
	"context"
	"time"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
)

// RecoverQuotaRefunds attempts at most 32 due intents. Pending records are the
// authority, independent of Gin contexts and process-local ownership markers.
// Each failed record is deferred for 30 seconds so one poison row does not starve
// later refunds. Concurrent recovery workers are safe because credits are atomic
// with the unique intent's completion. It returns the joined processing errors.
func RecoverQuotaRefunds(ctx context.Context) error {
	var pending []QuotaRefund
	if err := DB.WithContext(ctx).Where("status = ? AND retry_at <= ?", QuotaRefundPending, time.Now().UTC().Unix()).
		Order("retry_at, created_at, id").Limit(32).Find(&pending).Error; err != nil {
		return errors.Wrap(err, "list pending quota refunds")
	}
	var failures []error
	for _, intent := range pending {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(errors.Join(append(failures, err)...), "quota refund recovery interrupted")
		}
		if err := ApplyQuotaRefund(ctx, intent.ID); err != nil {
			failures = append(failures, errors.Wrapf(err, "recover quota refund %s", intent.ID))
			// A lost commit acknowledgement may already have completed the row.
			// This update cannot reopen it or undo a concurrent worker's success.
			if updateErr := DB.WithContext(ctx).Model(&QuotaRefund{}).
				Where("id = ? AND status = ?", intent.ID, QuotaRefundPending).
				Updates(map[string]any{"retry_at": time.Now().UTC().Add(30 * time.Second).Unix(),
					"attempts": gorm.Expr("attempts + 1")}).Error; updateErr != nil {
				failures = append(failures, errors.Wrap(updateErr, "defer failed quota refund"))
			}
		}
	}
	if len(failures) > 0 {
		return errors.Wrap(errors.Join(failures...), "recover pending quota refunds")
	}
	return nil
}

// RunQuotaRefundRecovery runs an immediate bounded sweep at startup and then
// every ten seconds until ctx is cancelled. The caller starts it through
// StartBackgroundWorker, so shutdown joins it before closing database handles.
func RunQuotaRefundRecovery(ctx context.Context) {
	lg := logger.FromContext(ctx)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		passCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := RecoverQuotaRefunds(passCtx)
		cancel()
		if err != nil && ctx.Err() == nil {
			lg.Error("pending quota refund recovery incomplete", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
