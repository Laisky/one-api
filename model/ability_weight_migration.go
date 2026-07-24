package model

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common/logger"
)

// MigrateAbilityWeightBackfill reconciles abilities.weight with the parent
// channel's weight. abilities.weight is a pure projection of channels.weight
// (always written to match by addAbilitiesWithDB), so any row that diverges is
// historical drift that must be healed.
//
// It reconciles on DATA state, not migration history, deliberately. Two upgrade
// shapes reach here with different physical states:
//   - Pre-weight-column installs: AutoMigrate adds the nullable column as NULL, so
//     historical rows need filling.
//   - Installs that already booted an intermediate build declaring the column with
//     a DB default (default:0): historical rows physically store 0 (never NULL),
//     because a column default never rewrites existing values and AutoMigrate does
//     not drop an existing default. A NULL-only backfill would silently skip these
//     and leave the DB routing path (reads abilities.weight) disagreeing with the
//     cache path (reads channels.weight) forever.
//
// Matching every out-of-sync row (NULL or a stale value) heals both shapes. It runs
// at startup before traffic, so there are no concurrent ability writes to race, and
// it only writes divergent rows — once converged it writes nothing, so it stays
// cheap on every subsequent boot (idempotent).
func MigrateAbilityWeightBackfill() error {
	if !DB.Migrator().HasTable(&Ability{}) {
		return nil
	}

	// Check if weight column exists
	if !DB.Migrator().HasColumn(&Ability{}, "Weight") {
		return nil
	}

	// Reconcile every row whose weight differs from its channel's weight. The
	// correlated subquery targets channels (a different table from the UPDATE
	// target), which is portable across SQLite, MySQL and PostgreSQL. A NULL
	// abilities.weight is caught by the explicit IS NULL disjunct because `NULL <> x`
	// evaluates to unknown rather than true.
	result := DB.Exec(`
		UPDATE abilities
		SET weight = COALESCE((SELECT weight FROM channels WHERE channels.id = abilities.channel_id), 0)
		WHERE weight IS NULL
		   OR weight <> COALESCE((SELECT weight FROM channels WHERE channels.id = abilities.channel_id), 0)
	`)
	if result.Error != nil {
		return errors.Wrap(result.Error, "reconcile ability weights from channels")
	}

	if result.RowsAffected > 0 {
		logger.Logger.Info("reconciled ability weights from channels",
			zap.Int64("rows_affected", result.RowsAffected))
	}

	return nil
}
