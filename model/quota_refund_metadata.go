package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/one-api/common/config"
	"gorm.io/gorm"
)

// PrepareQuotaRefundMetadata captures the original cost-row generation before credit.
// Missing cost rows are established with the outstanding hold, never a false zero.
func PrepareQuotaRefundMetadata(ctx context.Context, intent *QuotaRefund, logID int) (err error) {
	defer func() {
		if err != nil {
			intent.CostSnapshotUnavailable = true
			intent.CostRecordID = 0
			intent.CostRecoveryEligible = false
		}
	}()
	intent.MetadataRequested = true
	intent.CostRecoveryEligible = config.RefundCostRecoveryEnabled
	intent.ProvisionalLogID = logID
	if intent.RequestID == "" {
		return nil
	}
	var cost UserRequestCost
	lookup := DB.WithContext(ctx).Where("request_id = ? AND user_id = ?", intent.RequestID, intent.UserID).Take(&cost).Error
	if errors.Is(lookup, gorm.ErrRecordNotFound) {
		cost = *NewUserRequestCost(intent.UserID, intent.RequestID, intent.Amount)
		if err := DB.WithContext(ctx).Create(&cost).Error; err != nil {
			if reread := DB.WithContext(ctx).Where("request_id = ? AND user_id = ?", intent.RequestID, intent.UserID).Take(&cost).Error; reread != nil {
				return errors.Wrap(err, "establish refund cost target")
			}
		}
	} else if lookup != nil {
		return errors.Wrap(lookup, "capture refund cost target")
	}
	intent.CostRecordID, intent.CostRevision = cost.Id, cost.Revision
	return nil
}

// reconcileQuotaRefundMetadata completes accounting after durable credit without re-crediting.
// Log IDs belong to one attempt; cost updates compare the captured generation so a
// later attempt, including an equal-valued charge, can never be zeroed by recovery.
func reconcileQuotaRefundMetadata(ctx context.Context, intent QuotaRefund, currentAttempt bool) error {
	if !intent.MetadataRequested || intent.MetadataCompleted {
		return nil
	}
	var failures []error
	if !intent.LogMetadataCompleted {
		var logErr error
		if intent.ProvisionalLogID > 0 {
			logErr = ReconcileConsumeLog(ctx, intent.ProvisionalLogID, 0, "refunded: "+intent.Reason, 0, 0, 0, nil)
		}
		if logErr == nil {
			logErr = DB.WithContext(ctx).Model(&QuotaRefund{}).Where("id = ? AND status = ?", intent.ID, QuotaRefundCompleted).Update("log_metadata_completed", true).Error
		}
		if logErr != nil {
			failures = append(failures, errors.Wrap(logErr, "recover refund log metadata"))
		}
	}
	if !intent.CostMetadataCompleted {
		if err := reconcileRefundCost(ctx, intent, currentAttempt); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return errors.Wrap(errors.Join(failures...), "refund metadata remains pending")
	}
	if err := DB.WithContext(ctx).Model(&QuotaRefund{}).Where("id = ? AND status = ?", intent.ID, QuotaRefundCompleted).
		Update("metadata_completed", true).Error; err != nil {
		return errors.Wrap(err, "complete refund metadata recovery")
	}
	return nil
}

// reconcileRefundCost commits shared-cost reconciliation with its durable receipt.
// Only the original blocked attempt or an explicitly qualified delayed snapshot
// may write; upgrading later cannot make mixed-version snapshots trustworthy.
func reconcileRefundCost(ctx context.Context, intent QuotaRefund, currentAttempt bool) error {
	if intent.CostSnapshotUnavailable {
		return errors.New("refund credited but original cost snapshot unavailable; manual reconciliation required")
	}
	if intent.CostRecordID > 0 && !currentAttempt && (!config.RefundCostRecoveryEnabled || !intent.CostRecoveryEligible) {
		return errors.New("refund cost metadata requires manual reconciliation: snapshot not eligible for delayed recovery")
	}
	return errors.Wrap(DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if intent.CostRecordID > 0 {
			if err := tx.Model(&UserRequestCost{}).
				Where("id = ? AND user_id = ? AND request_id = ? AND revision = ?", intent.CostRecordID, intent.UserID, intent.RequestID, intent.CostRevision).
				Updates(map[string]any{"quota": int64(0), "revision": gorm.Expr("revision + 1")}).Error; err != nil {
				return errors.Wrap(err, "recover refund cost metadata")
			}
		}
		return errors.Wrap(tx.Model(&QuotaRefund{}).Where("id = ? AND status = ?", intent.ID, QuotaRefundCompleted).
			Update("cost_metadata_completed", true).Error, "complete refund cost receipt")
	}), "commit refund cost metadata")
}
