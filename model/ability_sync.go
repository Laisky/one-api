package model

import (
	"crypto/subtle"
	"slices"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
)

// abilitySyncBatchSize bounds the rows inserted, or the models named, by one statement
// of an abilities sync, keeping every statement well under each dialect's bind limit.
const abilitySyncBatchSize = 500

// abilityKey identifies one ability of a channel.
type abilityKey struct {
	group string
	model string
}

// abilityUpstreamColumns are the channel columns that identify the upstream an ability's
// runtime suspension was measured against: who is called (type, base URL, config and the
// legacy other field), with which credentials (key), and which upstream model a public
// model resolves to (model mapping, inference profile map).
var abilityUpstreamColumns = []string{
	"id", "type", "key", "base_url", "other", "config", "model_mapping", "inference_profile_arn_map",
}

// buildChannelAbilities returns the abilities a channel's configuration calls for: one per
// visible model and group, carrying the channel's routing flags and no suspension.
//
// Parameters:
//   - channel: the channel whose abilities are built.
//
// Return values:
//   - []Ability: the configuration-derived abilities; empty when nothing is routable.
func buildChannelAbilities(channel *Channel) []Ability {
	models := channel.GetSupportedModelNames()
	groups := channel.GetGroupNames()
	hiddenModels := channel.GetHiddenModels()
	abilities := make([]Ability, 0, len(models)*len(groups))
	for _, model := range models {
		if _, hidden := hiddenModels[strings.ToLower(model)]; hidden {
			continue
		}
		for _, group := range groups {
			abilities = append(abilities, Ability{
				Group:     group,
				Model:     model,
				ChannelId: channel.Id,
				Enabled:   channel.Status == ChannelStatusEnabled,
				Priority:  channel.Priority,
			})
		}
	}
	return abilities
}

// syncAbilitiesWithDB reconciles a channel's abilities with its configuration inside the
// caller's transaction, treating the table as a projection that also carries runtime
// state. Abilities the configuration no longer calls for are deleted and new ones are
// inserted; retained ones are updated in place, so their relay-time suspension survives
// an unrelated save (a testing model, name, priority, status, or model-list change).
// Rebuilding them instead used to lift every active suspension on every save.
//
// Parameters:
//   - db: the transaction that holds the channel row lock.
//   - channel: the channel as stored after the write.
//   - liftSuspensions: true when the write changed the upstream every suspension was
//     measured against (see sameAbilityUpstream), so the suspensions no longer apply.
//
// Return values:
//   - error: a wrapped read or write failure; the caller rolls the transaction back.
func syncAbilitiesWithDB(db *gorm.DB, channel *Channel, liftSuspensions bool) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	if channel == nil {
		return errors.New("channel must be specified when syncing abilities")
	}
	desired := buildChannelAbilities(channel)
	wanted := make(map[abilityKey]struct{}, len(desired))
	for _, ability := range desired {
		wanted[abilityKey{group: ability.Group, model: ability.Model}] = struct{}{}
	}

	var existing []Ability
	if err := db.Select("group", "model", "enabled", "priority").
		Where("channel_id = ?", channel.Id).Find(&existing).Error; err != nil {
		return identity.Tag(errors.Wrapf(err, "read abilities of channel %d", channel.Id), channel.Ref())
	}
	enabled, priority := channel.Status == ChannelStatusEnabled, channel.GetPriority()
	present := make(map[abilityKey]struct{}, len(existing))
	staleByGroup := make(map[string][]string)
	removed, flagsDrifted := 0, false
	for _, ability := range existing {
		key := abilityKey{group: ability.Group, model: ability.Model}
		present[key] = struct{}{}
		if _, ok := wanted[key]; !ok {
			staleByGroup[ability.Group] = append(staleByGroup[ability.Group], ability.Model)
			removed++
			continue
		}
		// A NULL priority never matches a routing tier, so it counts as drift and is
		// rewritten, as the old rebuild did.
		if ability.Enabled != enabled || ability.Priority == nil || *ability.Priority != priority {
			flagsDrifted = true
		}
	}

	// Delete before inserting: on a case-insensitive MySQL collation a case-only model
	// rename would otherwise collide with the row it replaces. Two rows of one channel can
	// never be equal under the column's collation, so this deletes exactly the stale rows.
	for group, models := range staleByGroup {
		for batch := range slices.Chunk(models, abilitySyncBatchSize) {
			if err := db.Where("channel_id = ?", channel.Id).
				Where(map[string]any{"group": group, "model": batch}).
				Delete(&Ability{}).Error; err != nil {
				return identity.Tag(errors.Wrapf(err, "delete stale abilities of channel %d", channel.Id), channel.Ref())
			}
		}
	}
	if flagsDrifted {
		if err := db.Model(&Ability{}).Where("channel_id = ?", channel.Id).
			Updates(map[string]any{"enabled": enabled, "priority": priority}).Error; err != nil {
			return identity.Tag(errors.Wrapf(err, "update routing flags of channel %d", channel.Id), channel.Ref())
		}
	}
	lifted := int64(0)
	if liftSuspensions {
		result := db.Model(&Ability{}).Where("channel_id = ? AND suspend_until IS NOT NULL", channel.Id).
			Update("suspend_until", nil)
		if result.Error != nil {
			return identity.Tag(errors.Wrapf(result.Error, "lift suspensions of channel %d", channel.Id), channel.Ref())
		}
		lifted = result.RowsAffected
	}
	missing := make([]Ability, 0, len(desired))
	for _, ability := range desired {
		if _, ok := present[abilityKey{group: ability.Group, model: ability.Model}]; !ok {
			missing = append(missing, ability)
		}
	}
	if len(missing) > 0 {
		if err := db.CreateInBatches(&missing, abilitySyncBatchSize).Error; err != nil {
			return identity.Tag(errors.Wrapf(err, "create abilities for channel %d", channel.Id), channel.Ref())
		}
	}

	if removed > 0 || len(missing) > 0 || flagsDrifted || lifted > 0 {
		logger.FromContext(db.Statement.Context).Debug("synced channel abilities",
			append(channel.Ref().Zap(),
				zap.Int("added", len(missing)),
				zap.Int("removed", removed),
				zap.Bool("routing_flags_updated", flagsDrifted),
				zap.Int64("suspensions_lifted", lifted))...)
	}
	return nil
}

// sameAbilityUpstream reports whether two snapshots of one channel call the same upstream
// (abilityUpstreamColumns), i.e. whether a suspension measured against before still
// describes after. Rotating a revoked key or repointing the endpoint is how an
// administrator remedies the failure a suspension records, so such a save lifts it.
//
// Parameters:
//   - before, after: the channel before and after a write.
//
// Return values:
//   - bool: true when every upstream-identifying column is unchanged.
func sameAbilityUpstream(before, after *Channel) bool {
	return before.Type == after.Type &&
		subtle.ConstantTimeCompare([]byte(before.Key), []byte(after.Key)) == 1 &&
		before.Config == after.Config &&
		sameOptionalString(before.BaseURL, after.BaseURL) &&
		sameOptionalString(before.Other, after.Other) &&
		sameOptionalString(before.ModelMapping, after.ModelMapping) &&
		sameOptionalString(before.InferenceProfileArnMap, after.InferenceProfileArnMap)
}
