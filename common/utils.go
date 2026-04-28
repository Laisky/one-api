package common

import (
	"fmt"

	"github.com/Laisky/one-api/common/config"
)

// LogQuota formats a quota value either as currency or points depending on configuration.
// The returned string is suitable for logging and user-facing messages.
func LogQuota(quota int64) string {
	if config.DisplayInCurrencyEnabled {
		return fmt.Sprintf("＄%.6f quota", float64(quota)/config.QuotaPerUnit)
	} else {
		return fmt.Sprintf("%d point quota", quota)
	}
}
