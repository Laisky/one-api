package model

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// MigrateAbilityWeightBackfill sets abilities.weight from the parent channel's weight
// for all ability rows where weight is currently NULL. This migration is idempotent
// and safe to run on every startup.
func MigrateAbilityWeightBackfill() error {
	if !DB.Migrator().HasTable(&Ability{}) {
		return nil
	}

	// Check if weight column exists
	if !DB.Migrator().HasColumn(&Ability{}, "Weight") {
		return nil
	}

	// Backfill: UPDATE abilities JOIN channels SET abilities.weight = channels.weight WHERE abilities.weight IS NULL
	result := DB.Exec(`
		UPDATE abilities 
		SET weight = COALESCE((SELECT weight FROM channels WHERE channels.id = abilities.channel_id), 0)
		WHERE weight IS NULL
	`)
	if result.Error != nil {
		return errors.Wrap(result.Error, "backfill ability weights from channels")
	}

	if result.RowsAffected > 0 {
		logger.Logger.Info("backfilled ability weights from channels",
			zap.Int64("rows_affected", result.RowsAffected))
	}

	return nil
}
