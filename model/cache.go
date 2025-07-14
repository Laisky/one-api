package model

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/logger"
	"github.com/songquanpeng/one-api/common/random"
)

var (
	TokenCacheSeconds         = config.SyncFrequency
	UserId2GroupCacheSeconds  = config.SyncFrequency
	UserId2QuotaCacheSeconds  = config.SyncFrequency
	UserId2StatusCacheSeconds = config.SyncFrequency
	GroupModelsCacheSeconds   = config.SyncFrequency
)

func CacheGetTokenByKey(key string) (*Token, error) {
	keyCol := "`key`"
	if common.UsingPostgreSQL {
		keyCol = `"key"`
	}
	var token Token
	if !common.RedisEnabled {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		return &token, err
	}
	tokenObjectString, err := common.RedisGet(fmt.Sprintf("token:%s", key))
	if err != nil {
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		if err != nil {
			return nil, err
		}
		jsonBytes, err := json.Marshal(token)
		if err != nil {
			return nil, err
		}
		err = common.RedisSet(fmt.Sprintf("token:%s", key), string(jsonBytes), time.Duration(TokenCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set token error: " + err.Error())
		}
		return &token, nil
	}

	err = json.Unmarshal([]byte(tokenObjectString), &token)
	return &token, err
}

func CacheGetUserGroup(id int) (group string, err error) {
	if !common.RedisEnabled {
		return GetUserGroup(id)
	}
	group, err = common.RedisGet(fmt.Sprintf("user_group:%d", id))
	if err != nil {
		group, err = GetUserGroup(id)
		if err != nil {
			return "", err
		}
		err = common.RedisSet(fmt.Sprintf("user_group:%d", id), group, time.Duration(UserId2GroupCacheSeconds)*time.Second)
		if err != nil {
			logger.SysError("Redis set user group error: " + err.Error())
		}
	}
	return group, err
}

func fetchAndUpdateUserQuota(ctx context.Context, id int) (quota int64, err error) {
	quota, err = GetUserQuota(id)
	if err != nil {
		return 0, err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	if err != nil {
		logger.Error(ctx, "Redis set user quota error: "+err.Error())
	}
	return
}

func CacheGetUserQuota(ctx context.Context, id int) (quota int64, err error) {
	if !common.RedisEnabled {
		return GetUserQuota(id)
	}
	quotaString, err := common.RedisGet(fmt.Sprintf("user_quota:%d", id))
	if err != nil {
		return fetchAndUpdateUserQuota(ctx, id)
	}
	quota, err = strconv.ParseInt(quotaString, 10, 64)
	if err != nil {
		return 0, nil
	}
	if quota <= config.PreConsumedQuota { // when user's quota is less than pre-consumed quota, we need to fetch from db
		logger.Infof(ctx, "user %d's cached quota is too low: %d, refreshing from db", quota, id)
		return fetchAndUpdateUserQuota(ctx, id)
	}
	return quota, nil
}

func CacheUpdateUserQuota(ctx context.Context, id int) error {
	if !common.RedisEnabled {
		return nil
	}
	quota, err := CacheGetUserQuota(ctx, id)
	if err != nil {
		return err
	}
	err = common.RedisSet(fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	return err
}

func CacheDecreaseUserQuota(id int, quota int64) error {
	if !common.RedisEnabled {
		return nil
	}
	err := common.RedisDecrease(fmt.Sprintf("user_quota:%d", id), int64(quota))
	return err
}

func CacheIsUserEnabled(userId int) (bool, error) {
	if !common.RedisEnabled {
		return IsUserEnabled(userId)
	}
	enabled, err := common.RedisGet(fmt.Sprintf("user_enabled:%d", userId))
	if err == nil {
		return enabled == "1", nil
	}

	userEnabled, err := IsUserEnabled(userId)
	if err != nil {
		return false, err
	}
	enabled = "0"
	if userEnabled {
		enabled = "1"
	}
	err = common.RedisSet(fmt.Sprintf("user_enabled:%d", userId), enabled, time.Duration(UserId2StatusCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set user enabled error: " + err.Error())
	}
	return userEnabled, err
}

// CacheGetGroupModels returns models of a group
//
// Deprecated: use CacheGetGroupModelsV2 instead
func CacheGetGroupModels(ctx context.Context, group string) (models []string, err error) {
	if !common.RedisEnabled {
		return GetGroupModels(ctx, group)
	}
	modelsStr, err := common.RedisGet(fmt.Sprintf("group_models:%s", group))
	if err == nil {
		return strings.Split(modelsStr, ","), nil
	}
	models, err = GetGroupModels(ctx, group)
	if err != nil {
		return nil, err
	}
	err = common.RedisSet(fmt.Sprintf("group_models:%s", group), strings.Join(models, ","), time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set group models error: " + err.Error())
	}
	return models, nil
}

// CacheGetGroupModelsV2 is a version of CacheGetGroupModels that returns EnabledAbility instead of string
func CacheGetGroupModelsV2(ctx context.Context, group string) (models []EnabledAbility, err error) {
	if !common.RedisEnabled {
		return GetGroupModelsV2(ctx, group)
	}
	modelsStr, err := common.RedisGet(fmt.Sprintf("group_models_v2:%s", group))
	if err != nil {
		logger.Warnf(ctx, "Redis get group models error: %+v", err)
	} else {
		if err = json.Unmarshal([]byte(modelsStr), &models); err != nil {
			logger.Warnf(ctx, "Redis get group models error: %+v", err)
		} else {
			return models, nil
		}
	}

	models, err = GetGroupModelsV2(ctx, group)
	if err != nil {
		return nil, errors.Wrap(err, "get group models")
	}

	cachePayload, err := json.Marshal(models)
	if err != nil {
		logger.SysError("Redis set group models error: " + err.Error())
		return models, nil
	}

	err = common.RedisSet(fmt.Sprintf("group_models:%s", group), string(cachePayload),
		time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		logger.SysError("Redis set group models error: " + err.Error())
	}

	return models, nil
}

var group2model2channels map[string]map[string][]*Channel
var channelSyncLock sync.RWMutex

func InitChannelCache() {
	newChannelId2channel := make(map[int]*Channel)
	var channels []*Channel
	DB.Where("status = ?", ChannelStatusEnabled).Find(&channels)
	for _, channel := range channels {
		newChannelId2channel[channel.Id] = channel
	}

	var allAbilities []*Ability
	DB.Find(&allAbilities) // Fetch all abilities

	// Filter abilities: must be enabled and not currently suspended
	// And create a quick lookup map for valid abilities
	// key: "group:model:channelId"
	validAbilityMap := make(map[string]bool)
	now := time.Now()
	for _, ability := range allAbilities {
		// Ensure the ability corresponds to an enabled channel (via ability.Enabled flag)
		// and is not currently suspended.
		// The ability.Enabled should have been set correctly based on channel.Status during AddAbilities/UpdateAbilities.
		if ability.Enabled && (ability.SuspendUntil == nil || ability.SuspendUntil.Before(now)) {
			// Check if the channel itself is in our list of enabled channels
			if _, channelExists := newChannelId2channel[ability.ChannelId]; channelExists {
				key := fmt.Sprintf("%s:%s:%d", ability.Group, ability.Model, ability.ChannelId)
				validAbilityMap[key] = true
			}
		}
	}

	newGroup2model2channels := make(map[string]map[string][]*Channel)

	// Iterate over channels that are confirmed to be enabled
	for _, channel := range channels { // channels are already filtered by status = ChannelStatusEnabled
		channelGroups := strings.Split(channel.Group, ",")
		channelModels := strings.Split(channel.Models, ",")

		for _, groupName := range channelGroups {
			if _, ok := newGroup2model2channels[groupName]; !ok {
				newGroup2model2channels[groupName] = make(map[string][]*Channel)
			}
			for _, modelName := range channelModels {
				// Check if this specific ability (group, model, channel.Id) is in our valid map
				abilityKey := fmt.Sprintf("%s:%s:%d", groupName, modelName, channel.Id)
				if _, isValidAbility := validAbilityMap[abilityKey]; isValidAbility {
					if _, ok := newGroup2model2channels[groupName][modelName]; !ok {
						newGroup2model2channels[groupName][modelName] = make([]*Channel, 0)
					}
					// Add the channel to the cache for this group and model
					newGroup2model2channels[groupName][modelName] = append(newGroup2model2channels[groupName][modelName], channel)
				}
			}
		}
	}

	// sort by priority
	for group, model2channels := range newGroup2model2channels {
		for model, channels := range model2channels {
			sort.Slice(channels, func(i, j int) bool {
				return channels[i].GetPriority() > channels[j].GetPriority()
			})
			newGroup2model2channels[group][model] = channels
		}
	}

	channelSyncLock.Lock()
	group2model2channels = newGroup2model2channels
	channelSyncLock.Unlock()
	logger.SysLog("channels synced from database, considering suspensions")
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.SysLog("syncing channels from database")
		InitChannelCache()
	}
}

func GetChannelsFromCache(group string, model string) ([]*Channel, error) {
	if !config.MemoryCacheEnabled {
		return nil, errors.New("MemoryCache is disabled")
	}
	channelSyncLock.RLock()
	channelsFromCache := group2model2channels[group][model]
	if len(channelsFromCache) == 0 {
		channelSyncLock.RUnlock()
		return nil, errors.New("channel not found in memory cache")
	}

	candidateChannels := make([]*Channel, len(channelsFromCache))
	copy(candidateChannels, channelsFromCache)
	channelSyncLock.RUnlock()

	return candidateChannels, nil
}

func CacheGetRandomSatisfiedChannel(group string, model string, ignoreFirstPriority bool) (*Channel, error) {
	if !config.MemoryCacheEnabled {
		return GetRandomSatisfiedChannel(group, model, ignoreFirstPriority)
	}
	channelSyncLock.RLock()
	// It's important to make a copy if we're going to modify or iterate outside lock,
	// or ensure operations are safe. Here, we are just reading.
	channelsFromCache := group2model2channels[group][model]

	// Create a new slice to operate on, to avoid issues if the underlying array is changed by a concurrent Sync.
	// And to filter out channels that might have been suspended since cache was built.
	// However, for simplicity and given SyncChannelCache rebuilds the map,
	// we'll rely on SyncChannelCache to clear out suspended channels periodically.
	// A live check here would add DB calls, negating some cache benefits.
	// The current InitChannelCache already filters by suspension.
	// If a channel is suspended *between* syncs, this cache might serve it.
	// The application's retry logic will then handle it.

	if len(channelsFromCache) == 0 {
		channelSyncLock.RUnlock()
		return nil, errors.New("channel not found in memory cache")
	}

	// Make a copy to safely work with outside the lock for selection logic
	candidateChannels := make([]*Channel, len(channelsFromCache))
	copy(candidateChannels, channelsFromCache)
	channelSyncLock.RUnlock()

	endIdx := len(candidateChannels)
	// choose by priority
	if endIdx == 0 { // Should be caught by earlier check, but as a safeguard
		return nil, errors.New("no channels available after cache check")
	}
	firstChannel := candidateChannels[0]
	if firstChannel.GetPriority() > 0 {
		for i := range candidateChannels {
			if candidateChannels[i].GetPriority() != firstChannel.GetPriority() {
				endIdx = i
				break
			}
		}
	}

	if config.DefaultUseMinMaxTokensModel {
		candidateChannels = candidateChannels[:endIdx]

		sort.Slice(candidateChannels, func(i, j int) bool {
			iModelConfig, jModelConfig := candidateChannels[i].GetModelConfig(model), candidateChannels[j].GetModelConfig(model)
			// Treat 0 as infinity (no limit)
			if iModelConfig == nil || iModelConfig.MaxTokens == 0 {
				return false // i has no limit, so it's not less than j
			}
			if jModelConfig == nil || jModelConfig.MaxTokens == 0 {
				return true // j has no limit, so i is less than j
			}

			return iModelConfig.MaxTokens < jModelConfig.MaxTokens
		})

		minTokensChannel := candidateChannels[0]
		minTokensModelConfig := minTokensChannel.GetModelConfig(model)
		if minTokensModelConfig.MaxTokens > 0 {
			for i := range candidateChannels {
				modelConfig := candidateChannels[i].GetModelConfig(model)
				if modelConfig.MaxTokens != minTokensModelConfig.MaxTokens {
					endIdx = i
					break
				}
			}
		}
	}

	idx := rand.Intn(endIdx)
	if ignoreFirstPriority {
		if endIdx < len(candidateChannels) { // which means there are more than one priority
			idx = random.RandRange(endIdx, len(candidateChannels))
		} else {
			// All channels have the same highest priority, or only one priority level exists.
			// If ignoreFirstPriority is true, and we only have one priority level,
			// it means we cannot satisfy "ignoreFirstPriority".
			// This case might indicate no lower-priority channels exist.
			// Depending on desired behavior, could return error or pick from existing.
			// For now, let's assume it means "pick any if only one priority level".
			// If truly no other channel to pick, the random selection will pick from current set.
			// This part of logic might need refinement based on precise meaning of ignoreFirstPriority
			// when only one priority tier exists.
			// The original code implies if endIdx == len(channels), it picks from 0 to endIdx-1.
			// If endIdx < len(channels), it picks from endIdx to len(channels)-1.
			// So if ignoreFirstPriority is true and all are same priority, it will still pick from them.
			// This seems okay.
		}
	}
	channel := candidateChannels[idx]
	logger.SysLogf("select channel %s#%d in cache", channel.Name, channel.Id)
	return channel, nil
}

// CacheGetRandomSatisfiedChannelExcluding gets a random satisfied channel while excluding specified channel IDs
func CacheGetRandomSatisfiedChannelExcluding(group string, model string, ignoreFirstPriority bool, excludeChannelIds map[int]bool, tryLargerMaxTokens bool) (*Channel, error) {
	if !config.MemoryCacheEnabled {
		return GetRandomSatisfiedChannelExcluding(group, model, ignoreFirstPriority, excludeChannelIds)
	}
	channelSyncLock.RLock()
	channelsFromCache := group2model2channels[group][model]

	if len(channelsFromCache) == 0 {
		channelSyncLock.RUnlock()
		return nil, errors.New("channel not found in memory cache")
	}

	// Filter out excluded channels
	var candidateChannels []*Channel
	for _, channel := range channelsFromCache {
		if !excludeChannelIds[channel.Id] {
			candidateChannels = append(candidateChannels, channel)
		}
	}

	// For HTTP Code 413
	// Filter out small max_tokens channels
	if tryLargerMaxTokens {
		smallerMaxTokensSizes := make(map[int32]bool)
		for _, channel := range channelsFromCache {
			if excludeChannelIds[channel.Id] {
				modelConfig := channel.GetModelConfig(model)
				smallerMaxTokensSizes[modelConfig.MaxTokens] = true
			}
		}

		var LargerMaxTokensSizeChannels []*Channel
		for _, channel := range channelsFromCache {
			modelConfig := channel.GetModelConfig(model)
			if modelConfig != nil && !smallerMaxTokensSizes[modelConfig.MaxTokens] {
				LargerMaxTokensSizeChannels = append(LargerMaxTokensSizeChannels, channel)
			} else if modelConfig == nil {
				LargerMaxTokensSizeChannels = append(LargerMaxTokensSizeChannels, channel)
			}
		}

		candidateChannels = LargerMaxTokensSizeChannels
	}
	channelSyncLock.RUnlock()

	if len(candidateChannels) == 0 {
		return nil, errors.New("no available channels after excluding failed channels")
	}

	// If ignoreFirstPriority is true, we want to select from lower priority channels
	// If ignoreFirstPriority is false, we want to select from highest priority channels
	if ignoreFirstPriority {
		// Find the boundary where highest priority channels end
		endIdx := len(candidateChannels)
		firstChannel := candidateChannels[0]
		if firstChannel.GetPriority() > 0 {
			for i := range candidateChannels {
				if candidateChannels[i].GetPriority() != firstChannel.GetPriority() {
					endIdx = i
					break
				}
			}
		}

		// If there are lower priority channels available, select from them
		if endIdx < len(candidateChannels) {
			idx := random.RandRange(endIdx, len(candidateChannels))
			channel := candidateChannels[idx]
			logger.SysLogf("select channel %s#%d in cache", channel.Name, channel.Id)
			return channel, nil
		} else {
			// No lower priority channels available, return error to indicate we should try a different approach
			return nil, errors.New("no lower priority channels available after excluding failed channels")
		}
	} else {
		// Select from highest priority channels among the available candidates
		// Since candidateChannels maintains the original cache order (sorted by priority desc),
		// we need to find the highest priority among the remaining candidates
		if len(candidateChannels) == 0 {
			return nil, errors.New("no candidate channels available")
		}

		// Find the maximum priority among available candidates
		maxPriority := candidateChannels[0].GetPriority()
		for _, channel := range candidateChannels {
			if channel.GetPriority() > maxPriority {
				maxPriority = channel.GetPriority()
			}
		}

		// Collect channels with the maximum priority
		var maxPriorityChannels []*Channel
		for _, channel := range candidateChannels {
			if channel.GetPriority() == maxPriority {
				maxPriorityChannels = append(maxPriorityChannels, channel)
			}
		}

		if len(maxPriorityChannels) == 0 {
			return nil, errors.New("no channels with maximum priority available")
		}

		idx := rand.Intn(len(maxPriorityChannels))
		channel := maxPriorityChannels[idx]
		logger.SysLogf("select channel %s#%d in cache", channel.Name, channel.Id)
		return channel, nil
	}
}
