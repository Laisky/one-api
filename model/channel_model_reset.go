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
// its models with the provider defaults plus every non-default catalog model the
// channel explicitly lists or configures and can serve, and rebuilds routing
// abilities. No override is rewritten, and a default alias whose mapped target
// admission would refuse is reported as a conflict instead of written. The catalog callback is supplied by the
// controller to avoid a model/relay import cycle; it maps a channel type to its
// ChannelResetCatalog. Parameters: ctx bounds the transaction, id is the channel
// ID, and catalog resolves the provider knowledge. It returns the persisted row
// without its key, or a wrapped conflict/database error with no partial changes.
func ResetChannelModelsToDefaults(ctx context.Context, id int, catalog func(int) ChannelResetCatalog) (*Channel, error) {
	if id <= 0 || catalog == nil {
		return nil, errors.WithStack(errors.New("a channel and model catalog are required"))
	}
	var (
		persisted *Channel
		retained  []string
	)
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Acquire the row's write lock BEFORE reading and validating it; a refused
		// reset rolls it back.
		if err := lockChannelRow(tx, id); err != nil {
			return errors.Wrap(err, "lock channel for model reset")
		}
		var current Channel
		if err := tx.Omit("key").First(&current, "id = ?", id).Error; err != nil {
			return errors.Wrap(err, "read locked channel for model reset")
		}
		planned, kept, err := planChannelModelReset(ctx, &current, catalog(current.Type))
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
		// A reset changes only the model list, so retained models keep their suspensions.
		if err := syncAbilitiesWithDB(tx, planned, false); err != nil {
			return errors.Wrap(err, "sync default channel abilities")
		}
		persisted, retained = planned, kept
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "reset channel models transaction")
	}
	InvalidateChannelModelCachesWithContext(ctx, persisted.Group)
	logger.FromContext(ctx).Debug("channel models reset to provider defaults",
		append(persisted.Ref().Zap(),
			zap.Int("model_count", len(persisted.GetSupportedModelNames())),
			zap.Strings("retained_models", retained))...)
	return persisted, nil
}
