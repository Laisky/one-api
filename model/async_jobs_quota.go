package model

import (
	"math"
	"time"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// adjustAsyncTaskQuota moves a signed quota amount in the task transaction.
// Both UPDATEs fence immutable identity, overflow and (during admission) current
// permissions. The original finite-token reservation is refunded even after an
// administrator changes the token's unlimited flag. No cache is used as a wallet.
func adjustAsyncTaskQuota(tx *gorm.DB, task *AsyncTask, amount int64, admission bool) error {
	user := tx.Model(&User{}).Where("id = ? AND uuid = ?", task.UserID, task.UserUUID)
	if admission {
		user = user.Where("status = ? AND quota >= ?", UserStatusEnabled, amount)
	} else {
		user = user.Where("quota <= ?", int64(math.MaxInt64)+amount)
	}
	result := user.Update("quota", gorm.Expr("quota - ?", amount))
	if result.Error != nil {
		return errors.Wrap(result.Error, "update async task user quota")
	}
	if result.RowsAffected != 1 {
		return errors.Wrap(ErrAsyncQuota, "async task owner changed or balance out of bounds")
	}
	token := tx.Model(&Token{}).Where("id = ? AND user_id = ? AND uuid = ?", task.TokenID, task.UserID, task.TokenUUID)
	if admission {
		token = token.Where("status = ? AND unlimited_quota = ? AND (expired_time = -1 OR expired_time >= ?)", TokenStatusEnabled, task.TokenUnlimited, time.Now().Unix())
		if task.TokenUnlimited {
			// A write fences a concurrent permission change even for unlimited tokens.
			result = token.Update("accessed_time", time.Now().Unix())
			if result.Error != nil {
				return errors.Wrap(result.Error, "validate unlimited async token")
			}
			if result.RowsAffected != 1 {
				// MySQL may report zero changed rows when accessed_time already equals
				// this second. The UPDATE still locks its matching row until commit.
				var count int64
				if err := token.Count(&count).Error; err != nil {
					return errors.Wrap(err, "check unchanged unlimited async token")
				}
				if count != 1 {
					return errors.New("async task token changed during admission")
				}
			}
			return nil
		}
		token = token.Where("remain_quota >= ? AND used_quota <= ?", amount, int64(math.MaxInt64)-amount)
	} else {
		if task.TokenUnlimited {
			return nil
		}
		token = token.Where("remain_quota <= ? AND used_quota >= ?", int64(math.MaxInt64)+amount, -amount)
	}
	values := map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", amount), "used_quota": gorm.Expr("used_quota + ?", amount), "accessed_time": time.Now().Unix()}
	if !admission {
		// Do not re-enable explicitly disabled/expired tokens. Exhaustion alone ends
		// when the original reservation is refunded.
		values["status"] = gorm.Expr("CASE WHEN status = ? THEN ? ELSE status END", TokenStatusExhausted, TokenStatusEnabled)
	}
	result = token.Updates(values)
	if result.Error != nil {
		return errors.Wrap(result.Error, "update async task token quota")
	}
	if result.RowsAffected != 1 {
		if !admission {
			var count int64
			if err := tx.Model(&Token{}).Where("id = ? AND user_id = ? AND uuid = ?", task.TokenID, task.UserID, task.TokenUUID).Count(&count).Error; err != nil {
				return errors.Wrap(err, "check removed async task token")
			}
			if count == 0 {
				// Only the original owner's wallet remains refundable after token
				// deletion. The immutable UUID prevents crediting a reused token ID.
				return nil
			}
		}
		return errors.Wrap(ErrAsyncQuota, "async token changed or balance out of bounds")
	}
	return nil
}
