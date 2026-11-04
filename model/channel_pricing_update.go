package model

import (
	"context"

	"github.com/Laisky/errors/v2"
	"gorm.io/gorm"
)

// UpdatePricingWithContext atomically updates only pricing and tooling fields.
// Parameters: ctx carries the request context, pricingProvided selects the prepared
// model configs, tooling is the requested tooling, and toolingProvided selects it.
// Returns: a wrapped database or validation error.
func (channel *Channel) UpdatePricingWithContext(ctx context.Context, pricingProvided bool, tooling *ChannelToolingConfig, toolingProvided bool) error {
	writeCtx, cancel := detachedChannelWriteContext(ctx)
	defer cancel()
	var current Channel
	err := DB.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		if err := lockChannelRow(tx, channel.Id); err != nil {
			return errors.Wrap(err, "lock channel for pricing update")
		}
		if err := tx.First(&current, "id = ?", channel.Id).Error; err != nil {
			return errors.Wrap(err, "load channel for pricing update")
		}
		updates := map[string]any{}
		if pricingProvided {
			updates["model_configs"] = channel.ModelConfigs
		}
		if toolingProvided {
			if err := current.SetToolingConfig(tooling); err != nil {
				return errors.Wrap(err, "set channel tooling")
			}
			updates["config"] = current.Config
		}
		if len(updates) > 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Updates(updates).Error; err != nil {
				return errors.Wrap(err, "update channel pricing")
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "persist channel pricing")
	}
	InvalidateChannelModelCachesWithContext(ctx, current.Group)
	return nil
}
