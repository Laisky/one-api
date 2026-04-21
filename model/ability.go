package model

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/utils"
	"github.com/songquanpeng/one-api/dto"
)

type Ability struct {
	Group        string     `json:"group" gorm:"type:varchar(32);primaryKey;autoIncrement:false"`
	Model        string     `json:"model" gorm:"primaryKey;autoIncrement:false"`
	ChannelId    int        `json:"channel_id" gorm:"primaryKey;autoIncrement:false;index"`
	Enabled      bool       `json:"enabled"`
	Priority     *int64     `json:"priority" gorm:"bigint;default:0;index"`
	SuspendUntil *time.Time `json:"suspend_until,omitempty" gorm:"index"`
	CreatedAt    int64      `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt    int64      `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
}

func GetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}

	ability := Ability{}
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		trueVal = "true"
	}
	now := time.Now()

	var err error = nil
	var channelQuery *gorm.DB
	if ignoreFirstPriority {
		channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, model, now)
	} else {
		maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, model, now)
		channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND priority = (?) AND (suspend_until IS NULL OR suspend_until < ?)", group, model, maxPrioritySubQuery, now)
	}
	if common.UsingSQLite.Load() || common.UsingPostgreSQL.Load() {
		err = channelQuery.Order("RANDOM()").First(&ability).Error
	} else {
		err = channelQuery.Order("RAND()").First(&ability).Error
	}
	if err != nil {
		return nil, errors.Wrap(err, "get random satisfied channel")
	}

	channel := Channel{}
	channel.Id = ability.ChannelId
	err = DB.First(&channel, "id = ?", ability.ChannelId).Error
	if err != nil {
		return nil, errors.Wrapf(err, "load channel %d for ability", ability.ChannelId)
	}
	if !channel.SupportsModel(model) {
		return nil, errors.Errorf("channel #%d does not list support for model %s", channel.Id, model)
	}
	return &channel, nil
}

func (channel *Channel) AddAbilities() error {
	models := utils.DeDuplication(channel.GetSupportedModelNames())
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
	return DB.Create(&abilities).Error
}

func (channel *Channel) DeleteAbilities() error {
	return DB.Where("channel_id = ?", channel.Id).Delete(&Ability{}).Error
}

// UpdateAbilities updates abilities of this channel.
// Make sure the channel is completed before calling this function.
func (channel *Channel) UpdateAbilities() error {
	// A quick and dirty way to update abilities
	// First delete all abilities of this channel
	err := channel.DeleteAbilities()
	if err != nil {
		return errors.Wrapf(err, "delete abilities for channel %d", channel.Id)
	}
	// Then add new abilities
	err = channel.AddAbilities()
	if err != nil {
		return errors.Wrapf(err, "add abilities for channel %d", channel.Id)
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
	return models, err
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
	if group == "" || modelName == "" || channelId == 0 {
		return errors.New("group, modelName, and channelId must be specified for suspending ability")
	}
	suspendTime := time.Now().Add(duration)

	// Handle database-specific identifier quoting like other functions
	groupCol := "`group`"
	modelCol := "`model`"
	channelCol := "`channel_id`"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		modelCol = `"model"`
		channelCol = `"channel_id"`
	}

	return DB.Model(&Ability{}).
		Where(groupCol+" = ? AND "+modelCol+" = ? AND "+channelCol+" = ?",
			group, modelName, channelId).
		Update("suspend_until", suspendTime).Error
}

func GetRandomSatisfiedChannelExcluding(group string, model string, ignoreFirstPriority bool, excludeChannelIds map[int]bool) (*Channel, error) {
	if DB == nil {
		return nil, errors.New("database not initialized")
	}
	ability := Ability{}
	groupCol := "`group`"
	trueVal := "1"
	if common.UsingPostgreSQL.Load() {
		groupCol = `"group"`
		trueVal = "true"
	}
	now := time.Now()

	var err error = nil
	var channelQuery *gorm.DB

	// Build base query with exclusions
	baseCondition := groupCol + " = ? AND model = ? AND enabled = " + trueVal + " AND (suspend_until IS NULL OR suspend_until < ?)"
	if len(excludeChannelIds) > 0 {
		var excludeIds []int
		for channelId := range excludeChannelIds {
			excludeIds = append(excludeIds, channelId)
		}
		baseCondition += " AND channel_id NOT IN (?)"
	}

	if ignoreFirstPriority {
		// For ignoreFirstPriority=true, we want to select from lower priority channels
		// First, find the maximum priority among available channels (excluding failed ones)
		maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, model, now)
		if len(excludeChannelIds) > 0 {
			var excludeIds []int
			for channelId := range excludeChannelIds {
				excludeIds = append(excludeIds, channelId)
			}
			maxPrioritySubQuery = maxPrioritySubQuery.Where("channel_id NOT IN (?)", excludeIds)
		}

		// Then select channels with priority less than the maximum priority
		if len(excludeChannelIds) > 0 {
			var excludeIds []int
			for channelId := range excludeChannelIds {
				excludeIds = append(excludeIds, channelId)
			}
			channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND priority < (?) AND (suspend_until IS NULL OR suspend_until < ?) AND channel_id NOT IN (?)", group, model, maxPrioritySubQuery, now, excludeIds)
		} else {
			channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND priority < (?) AND (suspend_until IS NULL OR suspend_until < ?)", group, model, maxPrioritySubQuery, now)
		}
	} else {
		// For ignoreFirstPriority=false, select from highest priority channels
		// First check if there are any available channels after exclusions
		var availableCount int64
		countQuery := DB.Model(&Ability{}).Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, model, now)
		if len(excludeChannelIds) > 0 {
			var excludeIds []int
			for channelId := range excludeChannelIds {
				excludeIds = append(excludeIds, channelId)
			}
			countQuery = countQuery.Where("channel_id NOT IN (?)", excludeIds)
		}
		countQuery.Count(&availableCount)

		if availableCount == 0 {
			// Simple error message to avoid performance overhead
			// Detailed diagnostics can be enabled via debug mode if needed
			var excludeIds []int
			for channelId := range excludeChannelIds {
				excludeIds = append(excludeIds, channelId)
			}

			errorMsg := fmt.Sprintf("no channels available for model %s in group %s after excluding %d channels",
				model, group, len(excludeIds))
			return nil, errors.New(errorMsg)
		}

		// Now find the maximum priority among available channels
		maxPrioritySubQuery := DB.Model(&Ability{}).Select("MAX(priority)").Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND (suspend_until IS NULL OR suspend_until < ?)", group, model, now)
		if len(excludeChannelIds) > 0 {
			var excludeIds []int
			for channelId := range excludeChannelIds {
				excludeIds = append(excludeIds, channelId)
			}
			// Add exclusion to the subquery as well
			maxPrioritySubQuery = maxPrioritySubQuery.Where("channel_id NOT IN (?)", excludeIds)
			channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND priority = (?) AND (suspend_until IS NULL OR suspend_until < ?) AND channel_id NOT IN (?)", group, model, maxPrioritySubQuery, now, excludeIds)
		} else {
			channelQuery = DB.Where(groupCol+" = ? AND model = ? AND enabled = "+trueVal+" AND priority = (?) AND (suspend_until IS NULL OR suspend_until < ?)", group, model, maxPrioritySubQuery, now)
		}
	}

	if common.UsingSQLite.Load() || common.UsingPostgreSQL.Load() {
		err = channelQuery.Order("RANDOM()").First(&ability).Error
	} else {
		err = channelQuery.Order("RAND()").First(&ability).Error
	}
	if err != nil {
		return nil, errors.Wrap(err, "get random satisfied channel excluding failed ones")
	}

	channel := Channel{}
	channel.Id = ability.ChannelId
	err = DB.First(&channel, "id = ?", ability.ChannelId).Error
	if err != nil {
		return nil, errors.Wrapf(err, "load channel %d for ability exclusion check", ability.ChannelId)
	}
	if !channel.SupportsModel(model) {
		return nil, errors.Errorf("channel #%d does not list support for model %s", channel.Id, model)
	}
	return &channel, nil
}
