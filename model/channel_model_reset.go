package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/logger"
)

// ChannelModelResetTarget is the minimal non-secret channel identity used when
// resetting the entire channel list, independently of frontend pagination.
type ChannelModelResetTarget struct {
	Id   int    `json:"-"`
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// ResetChannelModelsToDefaults atomically validates the current channel, replaces
// its models, and rebuilds routing abilities. The catalog callback is supplied by
// the controller to avoid a model/relay import cycle. It returns the persisted row
// without its key, or a wrapped conflict/database error with no partial changes.
func ResetChannelModelsToDefaults(ctx context.Context, id int, catalog func(int) []string) (*Channel, error) {
	if id <= 0 || catalog == nil {
		return nil, errors.WithStack(errors.New("a channel and model catalog are required"))
	}
	var persisted *Channel
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the row's write lock BEFORE reading and validating it. UpdateColumn
		// skips timestamp hooks; this no-op works on SQLite, MySQL, and PostgreSQL
		// without relying on dialect-specific SELECT FOR UPDATE syntax. A refused
		// reset rolls it back. Do not interpret MySQL's no-op RowsAffected as absence.
		if err := tx.Model(&Channel{}).Where("id = ?", id).UpdateColumn("models", gorm.Expr("models")).Error; err != nil {
			return errors.Wrap(err, "lock channel for model reset")
		}
		var current Channel
		if err := tx.Omit("key").First(&current, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read locked channel for model reset")
		}
		planned, err := planChannelModelReset(&current, catalog(current.Type))
		if err != nil {
			return errors.Wrap(err, "plan channel model reset")
		}
		updates := map[string]any{
			"models":        planned.Models,
			"hidden_models": nil,
			"testing_model": planned.TestingModel,
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return errors.Wrap(err, "persist default channel models")
		}
		if err := deleteAbilitiesWithDB(tx, id); err != nil {
			return errors.Wrap(err, "remove previous channel abilities")
		}
		if err := addAbilitiesWithDB(tx, planned); err != nil {
			return errors.Wrap(err, "rebuild default channel abilities")
		}
		persisted = planned
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "reset channel models transaction")
	}
	InvalidateChannelModelCachesWithContext(ctx, persisted.Group)
	logger.FromContext(ctx).Debug("channel models reset to provider defaults",
		append(persisted.Ref().Zap(), zap.Int("model_count", len(persisted.GetSupportedModelNames())))...)
	return persisted, nil
}
