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
	Group        string     `json:"group" gorm:"type:varchar(32);primaryKey;autoIncrement:false"`
	Model        string     `json:"model" gorm:"primaryKey;autoIncrement:false"`
	ChannelId    int        `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled      bool       `json:"enabled"`
	Priority     *int64     `json:"priority" gorm:"bigint;default:0;index"`
	Weight       *uint      `json:"weight" gorm:"default:0"`
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

// GetRandomSatisfiedChannel selects a random enabled channel for an exact group
// and model match, optionally considering all priority tiers.
func GetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	now := time.Now().UTC()

	var channelQuery *gorm.DB
	if ignoreFirstPriority {
		channelQuery = availableAbilitiesQuery(DB, group, model, now, nil)
	} else {
		maxPrioritySubQuery := availableAbilitiesQuery(DB, group, model, now, nil).Select("MAX(priority)")
		channelQuery = availableAbilitiesQuery(DB, group, model, now, nil).Where("priority = (?)", maxPrioritySubQuery)
	}
	var abilities []Ability
	if err := channelQuery.Find(&abilities).Error; err != nil {
		return nil, errors.Wrap(err, "get random satisfied channel")
	}
	if len(abilities) == 0 {
		return nil, errkind.ConfigErr(errors.New("no channels available for this group/model pair"))
	}

	selectedAbility, err := selectAbilityByWeight(abilities)
	if err != nil {
		return nil, errkind.ConfigErr(errors.Wrap(err, "select ability by weight"))
	}

	channel := Channel{}
	channel.Id = selectedAbility.ChannelId
	if err := DB.First(&channel, "id = ?", selectedAbility.ChannelId).Error; err != nil {
		return nil, identity.Tag(
			errors.Wrapf(err, "load channel %d for ability", selectedAbility.ChannelId),
			LookupChannelRef(context.Background(), selectedAbility.ChannelId))
	}
	if !channel.SupportsModel(model) {
		return nil, identity.Tag(
			errors.Errorf("channel #%d does not list support for model %s", channel.Id, model),
			channel.Ref())
	}
	return &channel, nil
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
				Weight:       channel.Weight,
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
	InitChannelCache()
	affectedGroups := collectChannelGroups(groupCSVs...)
	ctx := context.Background()
	redisReady := common.IsRedisEnabled() && common.RDB != nil
	if len(affectedGroups) == 0 {
		if redisReady {
			if err := deleteRedisKeysByPattern(ctx, "group_models:*"); err != nil {
				logger.Logger.Warn("failed to clear redis group_models cache by pattern", zap.Error(err))
			}
			if err := deleteRedisKeysByPattern(ctx, "group_models_v2:*"); err != nil {
				logger.Logger.Warn("failed to clear redis group_models_v2 cache by pattern", zap.Error(err))
			}
		}
		return
	}

	for group := range affectedGroups {
		getGroupModelsV2Cache.Delete(group)
		if !redisReady {
			continue
		}
		if err := common.RedisDel(ctx, fmt.Sprintf("group_models:%s", group)); err != nil {
			logger.Logger.Warn("failed to clear redis group_models cache", zap.String("group", group), zap.Error(err))
		}
		if err := common.RedisDel(ctx, fmt.Sprintf("group_models_v2:%s", group)); err != nil {
			logger.Logger.Warn("failed to clear redis group_models_v2 cache", zap.String("group", group), zap.Error(err))
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

// GetRandomSatisfiedChannelExcluding selects a random enabled channel for an
// exact group and model match while omitting the supplied channel IDs.
func GetRandomSatisfiedChannelExcluding(group string, model string, ignoreFirstPriority bool, excludeChannelIds map[int]bool) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	now := time.Now().UTC()
	excludeIDs := excludedChannelIDs(excludeChannelIds)

	var channelQuery *gorm.DB
	if ignoreFirstPriority {
		maxPrioritySubQuery := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Select("MAX(priority)")
		channelQuery = availableAbilitiesQuery(DB, group, model, now, excludeIDs).
			Where("priority < (?)", maxPrioritySubQuery)
	} else {
		var availableCount int64
		if err := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Count(&availableCount).Error; err != nil {
			return nil, errors.Wrap(err, "count available channels after exclusions")
		}
		if availableCount == 0 {
			// Every candidate channel has been excluded by earlier retries: a
			// configuration/capacity state, not a server-side code fault.
			return nil, errkind.ConfigErr(errors.Errorf("no channels available for model %s in group %s after excluding %d channels",
				model, group, len(excludeIDs)))
		}

		maxPrioritySubQuery := availableAbilitiesQuery(DB, group, model, now, excludeIDs).Select("MAX(priority)")
		channelQuery = availableAbilitiesQuery(DB, group, model, now, excludeIDs).
			Where("priority = (?)", maxPrioritySubQuery)
	}

	var abilities []Ability
	if err := channelQuery.Find(&abilities).Error; err != nil {
		return nil, errors.Wrap(err, "get random satisfied channel excluding failed ones")
	}
	if len(abilities) == 0 {
		return nil, errkind.ConfigErr(errors.Errorf("no channels available for model %s in group %s after excluding %d channels",
			model, group, len(excludeIDs)))
	}

	selectedAbility, err := selectAbilityByWeight(abilities)
	if err != nil {
		return nil, errkind.ConfigErr(errors.Wrap(err, "select ability by weight"))
	}

	channel := Channel{}
	channel.Id = selectedAbility.ChannelId
	if err := DB.First(&channel, "id = ?", selectedAbility.ChannelId).Error; err != nil {
		return nil, identity.Tag(
			errors.Wrapf(err, "load channel %d for ability exclusion check", selectedAbility.ChannelId),
			LookupChannelRef(context.Background(), selectedAbility.ChannelId))
	}
	if !channel.SupportsModel(model) {
		return nil, identity.Tag(
			errors.Errorf("channel #%d does not list support for model %s", channel.Id, model),
			channel.Ref())
	}
	return &channel, nil
}

// selectAbilityByWeight picks one ability using weighted random selection.
// When all weights are zero it falls back to uniform random.
func selectAbilityByWeight(abilities []Ability) (Ability, error) {
	if len(abilities) == 0 {
		return Ability{}, errors.New("no abilities to select from")
	}

	// Calculate total weight
	var totalWeight uint
	for _, a := range abilities {
		if a.Weight != nil {
			totalWeight += *a.Weight
		}
	}

	// If all weights are zero, fall back to uniform random
	if totalWeight == 0 {
		return abilities[rand.Intn(len(abilities))], nil
	}

	// Weighted random selection
	r := rand.Intn(int(totalWeight))
	var cumulative uint
	for _, a := range abilities {
		w := uint(0)
		if a.Weight != nil {
			w = *a.Weight
		}
		cumulative += w
		if uint(r) < cumulative {
			return a, nil
		}
	}

	// Fallback (shouldn't reach here)
	return abilities[len(abilities)-1], nil
}
