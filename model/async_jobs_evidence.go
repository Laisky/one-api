package model

import (
	"context"
	"math"
	"math/big"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// RecordAsyncTaskEvidence commits already-observed identity and cost separately
// from accounting. A token/user update failure cannot roll back an accepted job
// ID or its charge. No result, refund, lease release or completed state is stored
// here: ApplyAsyncTaskUpdate remains the atomic financial/publication boundary.
func RecordAsyncTaskEvidence(ctx context.Context, task *AsyncTask, update AsyncTaskUpdate) error {
	if update.UpstreamID == "" && update.CostUSD == "" {
		return nil
	}
	if DB == nil || task == nil || task.ID == "" || task.LeaseOwner == "" || len(update.UpstreamID) > 191 {
		return errors.New("invalid async task evidence")
	}
	if _, err := maxAsyncObservedCost("", update.CostUSD); err != nil {
		// A malformed amount cannot discard an independently valid receipt.
		// Preserve identity without publishing output, refunding or settling;
		// return the amount error so accounting remains visibly unresolved.
		if update.UpstreamID != "" {
			if identityErr := RecordAsyncTaskEvidence(ctx, task, AsyncTaskUpdate{UpstreamID: update.UpstreamID}); identityErr != nil {
				return errors.Wrap(errors.Join(err, identityErr), "preserve async identity after invalid cost")
			}
		}
		return err
	}
	return runWithSQLiteBusyRetryForDB(ctx, DB, func() error {
		return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// Incrementing a private evidence version obtains the row lock and makes
			// RowsAffected reliable even when MySQL sees otherwise identical values.
			claim := tx.Model(&AsyncTask{}).Where("id = ? AND state = ? AND lease_owner = ? AND lease_until > ? AND billing_state = ? AND evidence_version < ?", task.ID, task.State, task.LeaseOwner, time.Now().UnixMilli(), AsyncBillingHeld, int64(math.MaxInt64)).
				Updates(map[string]any{"evidence_version": gorm.Expr("evidence_version + 1"), "evidence_pending": true})
			if claim.Error != nil {
				return errors.Wrap(claim.Error, "lock async task evidence")
			}
			if claim.RowsAffected != 1 {
				return ErrAsyncLeaseLost
			}
			var saved AsyncTask
			if err := tx.Where("id = ?", task.ID).Take(&saved).Error; err != nil {
				return errors.Wrap(err, "read async task evidence")
			}
			if saved.UpstreamID != "" && update.UpstreamID != "" && saved.UpstreamID != update.UpstreamID {
				return errors.New("async task evidence conflicts with accepted identity")
			}
			cost, err := maxAsyncObservedCost(saved.ObservedCostUSD, update.CostUSD)
			if err != nil {
				return err
			}
			values := map[string]any{"observed_cost_usd": cost}
			if update.UpstreamID != "" {
				values["upstream_id"] = update.UpstreamID
			}
			if err := tx.Model(&AsyncTask{}).Where("id = ?", task.ID).Updates(values).Error; err != nil {
				return errors.Wrap(err, "persist async task evidence")
			}
			return nil
		})
	})
}

// maxAsyncObservedCost retains the greater bounded decimal amount. Validation
// runs before big-number parsing; empty means no observation, never zero cost.
func maxAsyncObservedCost(previous, current string) (string, error) {
	for _, value := range []string{previous, current} {
		if value != "" {
			if _, err := AsyncUpstreamCostQuota("1", value); err != nil {
				return "", errors.Wrap(err, "invalid async cost evidence")
			}
		}
	}
	if previous == "" {
		return current, nil
	}
	if current == "" {
		return previous, nil
	}
	left, _ := new(big.Rat).SetString(previous)
	right, _ := new(big.Rat).SetString(current)
	if left.Cmp(right) >= 0 {
		return previous, nil
	}
	return current, nil
}
