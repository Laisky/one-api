package model

import (
	"context"
	"maps"
	"slices"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
)

// ChannelPricingUpdate describes the change a channel pricing save makes. Only the
// parts marked as provided are applied; everything else on the channel row is left
// exactly as stored.
type ChannelPricingUpdate struct {
	// ModelConfigs replaces the unified per-model pricing table (model_configs) when
	// ModelConfigsProvided is true. An empty map clears the table.
	ModelConfigs map[string]ModelConfigLocal
	// ModelConfigsProvided reports whether ModelConfigs should be applied.
	ModelConfigsProvided bool
	// Tooling replaces the tooling entry inside the config blob when ToolingProvided is
	// true. A nil value removes the entry; the other config entries are kept.
	Tooling *ChannelToolingConfig
	// ToolingProvided reports whether Tooling should be applied.
	ToolingProvided bool
}

// UpdateChannelPricingWithContext applies a pricing change to one channel and writes
// only the pricing columns update provides: model_configs and/or config (plus
// updated_at). An update that provides neither writes nothing.
//
// The read and the write run in one transaction that first takes the row lock, so
// the change is applied to the latest stored config and model_configs, and a
// concurrent edit of the channel's other columns (models, priority, base URL, groups,
// testing model, and so on) is never reverted with stale values. Pricing does not
// feed abilities, so they are not rebuilt; the in-memory channel cache is refreshed
// because routing and billing read pricing from it.
//
// Parameters:
//   - ctx: carries the request-scoped logger; cancellation does not abort the write.
//   - id: the internal id of the channel to update.
//   - update: the pricing and tooling change to apply.
//
// Return values:
//   - error: a wrapped error marked errkind.NotFound when the channel does not
//     exist, errkind.InvalidRequest when the pricing or tooling is invalid, and
//     unmarked for persistence failures. Nothing is written when it is non-nil.
func UpdateChannelPricingWithContext(ctx context.Context, id int, update ChannelPricingUpdate) error {
	var (
		current Channel
		columns []string
	)
	// WithoutCancel keeps request values (logging, tracing) while preserving the
	// established run-to-completion behaviour if the client disconnects mid-update.
	err := DB.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		if err := lockChannelRow(tx, id); err != nil {
			return errors.Wrapf(err, "lock channel %d for pricing update", id)
		}
		// type and base_url are read because config decoding validates Jina URLs;
		// uuid and name only label diagnostics.
		if err := tx.Select("id", "uuid", "name", "type", "base_url", "model_configs", "config").
			First(&current, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errkind.NotFoundErr(errors.Wrapf(err, "channel %d not found", id))
			}
			return errors.Wrapf(err, "read pricing columns of channel %d", id)
		}
		values, err := current.applyPricingUpdate(update)
		if err != nil {
			return errors.Wrapf(err, "apply pricing update to channel %d", id)
		}
		if len(values) == 0 {
			return nil
		}
		if err := tx.Model(&Channel{}).Where("id = ?", id).Updates(values).Error; err != nil {
			return errors.Wrapf(err, "write pricing columns of channel %d", id)
		}
		columns = slices.Sorted(maps.Keys(values))
		return nil
	})
	if err != nil {
		ref := current.Ref()
		if current.Id == 0 {
			ref = LookupChannelRef(ctx, id)
		}
		return identity.Tag(errors.Wrapf(err, "persist channel %d pricing update", id), ref)
	}

	lg := logger.FromContext(ctx)
	if len(columns) == 0 {
		lg.Debug("channel pricing update changed nothing", current.Ref().Zap()...)
		return nil
	}
	lg.Debug("channel pricing updated", append(current.Ref().Zap(), zap.Strings("columns", columns))...)
	// Group model lists derive from abilities, which pricing does not touch, so only
	// the channel snapshot that carries model_configs and config is rebuilt.
	if err := InitChannelCacheContext(context.WithoutCancel(ctx)); err != nil {
		lg.Error("failed to refresh channel cache after pricing update",
			append(current.Ref().Zap(), zap.Error(err))...)
	}
	return nil
}

// applyPricingUpdate applies update to the pricing fields of channel, which must hold
// the freshly read model_configs and config columns.
//
// Parameters:
//   - update: the pricing and tooling change to apply.
//
// Return values:
//   - map[string]any: the column values to persist, keyed by column name; empty when
//     update provides nothing.
//   - error: a wrapped error, marked errkind.InvalidRequest when update itself is invalid.
func (channel *Channel) applyPricingUpdate(update ChannelPricingUpdate) (map[string]any, error) {
	values := make(map[string]any, 2)
	if update.ModelConfigsProvided {
		if err := channel.SetModelPriceConfigs(update.ModelConfigs); err != nil {
			return nil, errkind.InvalidRequestErr(errors.Wrap(err, "set model configs"))
		}
		// A nil pointer is written as SQL NULL, which is how an empty table is stored.
		values["model_configs"] = channel.ModelConfigs
	}
	if update.ToolingProvided {
		// Decode the stored blob first so a broken stored config is reported as a
		// server-side fault rather than blamed on the request.
		if _, err := channel.LoadConfig(); err != nil {
			return nil, errors.Wrap(err, "load stored channel config")
		}
		if err := channel.SetToolingConfig(update.Tooling); err != nil {
			return nil, errkind.InvalidRequestErr(errors.Wrap(err, "set tooling config"))
		}
		// An empty string is written as-is, so removing the last entry clears the blob.
		values["config"] = channel.Config
	}
	return values, nil
}
