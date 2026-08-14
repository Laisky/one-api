package model

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/errkind"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/dto"
)

type Ability struct {
	Group     string `json:"group" gorm:"type:varchar(32);primaryKey;autoIncrement:false"`
	Model     string `json:"model" gorm:"primaryKey;autoIncrement:false"`
	ChannelId int    `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled   bool   `json:"enabled"`
	Priority  *int64 `json:"priority" gorm:"bigint;default:0;index"`
	// Weight is intentionally NOT stored on the ability. The abilities row is a
	// routing projection whose columns are all query FILTERS (group, model, enabled,
	// priority, suspend_until); weight is a post-filter SELECTION input, so it lives
	// solely on the parent channel (channels.weight, the single source of truth) and
	// is read there by both routing paths. See getRandomSatisfiedChannel and
	// pickWeightedChannel.
	SuspendUntil *time.Time `json:"suspend_until,omitempty" gorm:"index"`
	CreatedAt    int64      `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt    int64      `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

// exactModelPredicate returns a parameterized equality predicate for a trusted,
// optionally-qualified model column. MySQL casts the bound value during rollout
// so exact routing remains correct before the model-column collation migration
// completes; other supported backends use their schema comparison directly.
func exactModelPredicate(column string) string {
	if common.UsingMySQL.Load() {
		return column + " = CAST(? AS BINARY)"
	}
	return column + " = ?"
}

// excludedChannelIDs returns a stable list of channel IDs excluded by routing.
func excludedChannelIDs(excluded map[int]bool) []int {
	ids := make([]int, 0, len(excluded))
	for channelID := range excluded {
		ids = append(ids, channelID)
	}
	sort.Ints(ids)
	return ids
}

// availableAbilitiesQuery builds the shared enabled, unsuspended, exact-model
// ability query and optionally excludes channel IDs. The returned query is not executed.
func availableAbilitiesQuery(db *gorm.DB, group string, model string, now time.Time, excludeIDs []int) *gorm.DB {
	groupCol := "`group`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
	}

	query := db.Model(&Ability{}).Where(
		groupCol+" = ? AND "+exactModelPredicate("model")+" AND enabled = ? AND (suspend_until IS NULL OR suspend_until < ?)",
		group, model, true, now,
	)
	if len(excludeIDs) > 0 {
		query = query.Where("channel_id NOT IN ?", excludeIDs)
	}
	return query
}

// priorityPolicy selects which priority tier participates in routing.
type priorityPolicy uint8

const (
	// tierHighest keeps only the highest-priority tier (ignoreFirstPriority=false).
	tierHighest priorityPolicy = iota
	// tierSkipHighestLenient keeps tiers strictly below the highest, but falls back
	// to the highest tier when none exists. Used by the non-excluding first-pass
	// selection so "ignore first priority" still routes in a single-tier group.
	tierSkipHighestLenient
	// tierSkipHighestStrict keeps only tiers strictly below the highest and treats
	// "no lower tier" as a configuration state (no candidates). Used by the retry
	// (excluding) path so the caller can decide whether to fall back to the top tier.
	tierSkipHighestStrict
)

// GetRandomSatisfiedChannel selects a random enabled channel for an exact group and
// model match. With ignoreFirstPriority=false only the highest-priority tier is
// eligible; with ignoreFirstPriority=true the highest tier is skipped in favour of
// lower tiers, falling back to the highest tier only when no lower tier exists.
// Within the eligible tier, selection is weighted by the channel's weight.
func GetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	policy := tierHighest
	if ignoreFirstPriority {
		policy = tierSkipHighestLenient
	}
	return getRandomSatisfiedChannel(group, model, nil, policy)
}

// getRandomSatisfiedChannel is the shared implementation behind both
// GetRandomSatisfiedChannel and GetRandomSatisfiedChannelExcluding. It resolves the
// candidate channels serving (group, model) at the tier chosen by policy, then
// weighted-selects one by the channel's weight (channels.weight is the single
// source of truth; CacheGetRandomSatisfiedChannel uses the same pickWeightedChannel
// helper). When every candidate weight is zero it falls back to uniform random.
func getRandomSatisfiedChannel(group string, model string, excludeChannelIds map[int]bool, policy priorityPolicy) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	now := time.Now().UTC()
	excludeIDs := excludedChannelIDs(excludeChannelIds)

	channelIDs, err := candidateChannelIDs(group, model, now, excludeIDs, policy)
	if err != nil {
		return nil, err
	}
	if len(channelIDs) == 0 {
		return nil, errkind.ConfigErr(noSatisfiedChannelError(group, model, excludeIDs))
	}

	var candidates []*Channel
	if err := DB.Where("id IN ?", channelIDs).Find(&candidates).Error; err != nil {
		return nil, errors.Wrap(err, "load candidate channels for routing")
	}
	if len(candidates) == 0 {
		// The abilities projection referenced channels that no longer exist.
		return nil, errkind.ConfigErr(noSatisfiedChannelError(group, model, excludeIDs))
	}

	channel := pickWeightedChannel(candidates)
	if channel == nil {
		// Every candidate weight is zero: fall back to uniform random.
		channel = candidates[rand.Intn(len(candidates))]
	}
	if !channel.SupportsModel(model) {
		return nil, identity.Tag(
			errors.Errorf("channel #%d does not list support for model %s", channel.Id, model),
			channel.Ref())
	}
	return channel, nil
}

// candidateChannelIDs returns the channel IDs whose abilities satisfy (group,
// model) at the priority tier selected by policy. The maximum priority is computed
// AFTER exclusions, so excluding a whole tier promotes the next one.
func candidateChannelIDs(group string, model string, now time.Time, excludeIDs []int, policy priorityPolicy) ([]int, error) {
	tierQuery := availableAbilitiesQuery(DB, group, model, now, excludeIDs)
	switch policy {
	case tierHighest:
		maxSub := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Select("MAX(priority)")
		tierQuery = tierQuery.Where("priority = (?)", maxSub)
	case tierSkipHighestLenient, tierSkipHighestStrict:
		maxSub := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Select("MAX(priority)")
		tierQuery = tierQuery.Where("priority < (?)", maxSub)
	}

	var channelIDs []int
	if err := tierQuery.Pluck("channel_id", &channelIDs).Error; err != nil {
		return nil, errors.Wrap(err, "load candidate channel ids")
	}

	if len(channelIDs) == 0 && policy == tierSkipHighestLenient {
		// No lower tier exists: fall back to the highest (only) tier so a
		// single-tier group still routes when ignoreFirstPriority is requested.
		maxSub := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Select("MAX(priority)")
		if err := availableAbilitiesQuery(DB, group, model, now, excludeIDs).
			Where("priority = (?)", maxSub).
			Pluck("channel_id", &channelIDs).Error; err != nil {
			return nil, errors.Wrap(err, "load highest-tier candidate channel ids")
		}
	}
	return channelIDs, nil
}

// noSatisfiedChannelError builds the "no channels available" configuration error,
// noting the exclusion count when retries have narrowed the candidate set.
func noSatisfiedChannelError(group string, model string, excludeIDs []int) error {
	if len(excludeIDs) > 0 {
		return errors.Errorf("no channels available for model %s in group %s after excluding %d channels",
			model, group, len(excludeIDs))
	}
	return errors.Errorf("no channels available for model %s in group %s", model, group)
}

// AddAbilities persists every visible ability advertised by channel.
func (channel *Channel) AddAbilities() error {
	return addAbilitiesWithDB(DB, channel)
}

// addAbilitiesWithDB persists channel abilities through db so callers can include
// the writes in a wider transaction. It returns a wrapped persistence error.
func addAbilitiesWithDB(db *gorm.DB, channel *Channel) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	if channel == nil {
		return errors.New("channel must be specified when adding abilities")
	}

	models := channel.GetSupportedModelNames()
	groups := channel.GetGroupNames()
	hiddenModels := channel.GetHiddenModels()
	abilities := make([]Ability, 0, len(models)*len(groups))
	for _, model := range models {
		if _, hidden := hiddenModels[strings.ToLower(model)]; hidden {
			continue
		}
		for _, group := range groups {
			ability := Ability{
				Group:        group,
				Model:        model,
				ChannelId:    channel.Id,
				Enabled:      channel.Status == ChannelStatusEnabled,
				Priority:     channel.Priority,
				SuspendUntil: nil, // Explicitly nil on new creation
			}
			abilities = append(abilities, ability)
		}
	}
	if len(abilities) == 0 {
		return nil
	}
	if err := db.Create(&abilities).Error; err != nil {
		return identity.Tag(
			errors.Wrapf(err, "create abilities for channel %d", channel.Id),
			channel.Ref())
	}
	return nil
}

// DeleteAbilities removes every ability persisted for channel.
func (channel *Channel) DeleteAbilities() error {
	return deleteAbilitiesWithDB(DB, channel.Id)
}

// deleteAbilitiesWithDB removes a channel's abilities through db so callers can
// include the delete in a wider transaction. It returns a wrapped persistence error.
func deleteAbilitiesWithDB(db *gorm.DB, channelID int) error {
	if db == nil {
		return errors.New("database not initialized")
	}
	if err := db.Where("channel_id = ?", channelID).Delete(&Ability{}).Error; err != nil {
		return identity.Tag(
			errors.Wrapf(err, "delete abilities for channel %d", channelID),
			LookupChannelRef(context.Background(), channelID))
	}
	return nil
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities() error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := deleteAbilitiesWithDB(tx, channel.Id); err != nil {
			return err
		}
		return addAbilitiesWithDB(tx, channel)
	}); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "update abilities for channel %d", channel.Id),
			channel.Ref())
	}
	return nil
}

func UpdateAbilityStatus(channelId int, status bool) error {
	return DB.Model(&Ability{}).Where("channel_id = ?", channelId).Select("enabled").Update("enabled", status).Error
}

func GetGroupModels(ctx context.Context, group string) ([]string, error) {
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		trueVal = "true"
	}
	var models []string
	err := DB.Model(&Ability{}).Distinct("model").Where(groupCol+" = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, time.Now()).Pluck("model", &models).Error
	if err != nil {
		return nil, errors.Wrap(err, "get group models")
	}
	sort.Strings(models)
	return models, nil
}

var getGroupModelsV2Cache = gutils.NewExpCache[[]dto.EnabledAbility](context.Background(), time.Second*10)

// collectChannelGroups expands comma-separated group lists into a unique set of cache keys.
func collectChannelGroups(groupCSVs ...string) map[string]struct{} {
	groups := make(map[string]struct{})
	for _, groupCSV := range groupCSVs {
		for _, rawGroup := range strings.Split(groupCSV, ",") {
			group := strings.TrimSpace(rawGroup)
			if group == "" {
				continue
			}
			groups[group] = struct{}{}
		}
	}
	return groups
}

// deleteRedisKeysByPattern removes Redis keys matching the provided pattern.
func deleteRedisKeysByPattern(ctx context.Context, pattern string) error {
	if common.RDB == nil {
		return nil
	}
	var cursor uint64
	for {
		keys, nextCursor, err := common.RDB.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return errors.Wrapf(err, "scan redis keys with pattern %s", pattern)
		}
		if len(keys) > 0 {
			if err := common.RDB.Del(ctx, keys...).Err(); err != nil {
				return errors.Wrapf(err, "delete redis keys with pattern %s", pattern)
			}
		}
		if nextCursor == 0 {
			return nil
		}
		cursor = nextCursor
	}
}

// InvalidateChannelModelCaches refreshes the in-memory routing cache and clears group-model list caches.
func InvalidateChannelModelCaches(groupCSVs ...string) {
	InvalidateChannelModelCachesWithContext(context.Background(), groupCSVs...)
}

const channelModelCacheInvalidationTimeout = 5 * time.Second

// newChannelModelCacheInvalidationContext detaches cache invalidation from
// request cancellation while retaining context values, including the
// request-scoped logger, and returns a bounded context plus its cancel function.
func newChannelModelCacheInvalidationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), channelModelCacheInvalidationTimeout)
}

// InvalidateChannelModelCachesWithContext refreshes routing caches and binds Redis diagnostics to ctx.
// Parameters: ctx carries request logging and groupCSVs lists affected channel groups.
// Returns: none; cache invalidation failures are logged.
func InvalidateChannelModelCachesWithContext(ctx context.Context, groupCSVs ...string) {
	invalidationCtx, cancel := newChannelModelCacheInvalidationContext(ctx)
	defer cancel()
	lg := logger.FromContext(invalidationCtx)

	InitChannelCache()
	affectedGroups := collectChannelGroups(groupCSVs...)
	redisReady := common.IsRedisEnabled() && common.RDB != nil
	if len(affectedGroups) == 0 {
		if redisReady {
			if err := deleteRedisKeysByPattern(invalidationCtx, "group_models:*"); err != nil {
				lg.Warn("failed to clear redis group_models cache by pattern", zap.Error(err))
			}
			if err := deleteRedisKeysByPattern(invalidationCtx, "group_models_v2:*"); err != nil {
				lg.Warn("failed to clear redis group_models_v2 cache by pattern", zap.Error(err))
			}
		}
		return
	}

	for group := range affectedGroups {
		getGroupModelsV2Cache.Delete(group)
		if !redisReady {
			continue
		}
		if err := common.RedisDel(invalidationCtx, fmt.Sprintf("group_models:%s", group)); err != nil {
			lg.Warn("failed to clear redis group_models cache", zap.String("group", group), zap.Error(err))
		}
		if err := common.RedisDel(invalidationCtx, fmt.Sprintf("group_models_v2:%s", group)); err != nil {
			lg.Warn("failed to clear redis group_models_v2 cache", zap.String("group", group), zap.Error(err))
		}
	}
}

// GetGroupModelsV2 returns all enabled models for this group with their channel names.
func GetGroupModelsV2(ctx context.Context, group string) ([]dto.EnabledAbility, error) {
	// get from cache first
	if models, ok := getGroupModelsV2Cache.Load(group); ok {
		return models, nil
	}

	// prepare query based on database type
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		trueVal = "true"
	}
	now := time.Now()

	// query with JOIN to get model, channel type, and channel ID in a single query
	var models []dto.EnabledAbility
	query := DB.Model(&Ability{}).
		Select("DISTINCT abilities.model AS model, channels.type AS channel_type, abilities.channel_id AS channel_id").
		Joins("JOIN channels ON abilities.channel_id = channels.id").
		Where("abilities."+groupCol+" = ? AND abilities.enabled = "+trueVal+" AND (abilities.suspend_until IS NULL OR abilities.suspend_until < ?)", group, now).
		Order("abilities.model")

	err := query.Find(&models).Error
	if err != nil {
		return nil, errors.Wrap(err, "get group models")
	}

	// store in cache
	getGroupModelsV2Cache.Store(group, models)

	return models, nil
}

// SuspendAbility sets the SuspendUntil timestamp for a given ability.
func SuspendAbility(ctx context.Context, group string, modelName string, channelId int, duration time.Duration) error {
	if DB == nil {
		return errors.New("database not initialized")
	}
	if group == "" || modelName == "" || channelId == 0 {
		return errors.New("group, modelName, and channelId must be specified for suspending ability")
	}
	suspendTime := time.Now().UTC().Add(duration)

	// Handle database-specific identifier quoting like other functions
	groupCol := "`group`"
	modelCol := "`model`"
	channelCol := "`channel_id`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		modelCol = `"model"`
		channelCol = `"channel_id"`
	}
	result := DB.WithContext(ctx).Model(&Ability{}).
		Where(groupCol+" = ? AND "+exactModelPredicate(modelCol)+" AND "+channelCol+" = ?",
			group, modelName, channelId).
		Update("suspend_until", suspendTime)
	if result.Error != nil {
		return identity.Tag(
			errors.Wrapf(result.Error, "suspend ability for group %q, model %q, channel %d", group, modelName, channelId),
			LookupChannelRef(ctx, channelId))
	}
	if result.RowsAffected != 1 {
		return identity.Tag(
			errors.Errorf("suspend ability for group %q, model %q, channel %d affected %d rows instead of 1",
				group, modelName, channelId, result.RowsAffected),
			LookupChannelRef(ctx, channelId))
	}
	return nil
}

// GetRandomSatisfiedChannelExcluding selects a random enabled channel for an exact
// group and model match while omitting the supplied channel IDs. This is the retry
// path: with ignoreFirstPriority=true only tiers strictly below the highest are
// eligible and "no lower tier" is a configuration state (error), letting the caller
// decide whether to fall back to the highest tier.
func GetRandomSatisfiedChannelExcluding(group string, model string, ignoreFirstPriority bool, excludeChannelIds map[int]bool) (*Channel, error) {
	policy := tierHighest
	if ignoreFirstPriority {
		policy = tierSkipHighestStrict
	}
	return getRandomSatisfiedChannel(group, model, excludeChannelIds, policy)
}
