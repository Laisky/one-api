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
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/common"
	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/common/random"
	"github.com/Laisky/one-api/dto"
)

var (
	TokenCacheSeconds           = config.SyncFrequency
	UserId2GroupCacheSeconds    = config.SyncFrequency
	UserId2QuotaCacheSeconds    = config.SyncFrequency
	UserId2StatusCacheSeconds   = config.SyncFrequency
	UserId2UsernameCacheSeconds = config.SyncFrequency
	GroupModelsCacheSeconds     = config.SyncFrequency
)

func CacheGetTokenByKey(ctx context.Context, key string) (*Token, error) {
	lg := logger.FromContext(ctx)
	keyCol := "`key`"
	if common.UsingPostgreSQL.Load() {
		keyCol = `"key"`
	}
	var token Token
	if !common.IsRedisEnabled() {
		if DB == nil {
			return nil, errors.New("database not initialized")
		}
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		if err != nil {
			return nil, errors.Wrapf(err, "get token by key %s", key)
		}
		return &token, nil
	}
	tokenObjectString, err := common.RedisGet(ctx, fmt.Sprintf("token:%s", key))
	if err != nil {
		if DB == nil {
			return nil, errors.Wrap(err, "database not initialized")
		}
		err := DB.Where(keyCol+" = ?", key).First(&token).Error
		if err != nil {
			return nil, errors.Wrapf(err, "get token by key %s", key)
		}
		// Cache the raw token row. With Token.MarshalJSON retired, the default
		// serialization already keeps the raw stored key (the response-time
		// prefix now lives in Token.ToResponse, not in json.Marshal) and carries
		// the internal id, which is exactly what the cache needs. (This file is
		// allowlisted in the noentityresponse analyzer: marshaling the raw entity
		// for the internal cache is intentional here.)
		jsonBytes, err := json.Marshal(token)
		if err != nil {
			return nil, identity.Tag(
				errors.Wrapf(err, "marshal token %d for cache", token.Id),
				token.Ref(), token.OwnerRef())
		}
		err = common.RedisSet(ctx, fmt.Sprintf("token:%s", key), string(jsonBytes), time.Duration(TokenCacheSeconds)*time.Second)
		if err != nil {
			// The raw token key is a credential and must never reach the log; the
			// token reference (id + uuid + name) is what an operator can act on.
			lg.Warn("Redis set token failed, continuing without cache",
				append(token.Ref().Zap(), zap.Error(err))...)
		}
		return &token, nil
	}

	err = json.Unmarshal([]byte(tokenObjectString), &token)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal cached token for key %s", key)
	}
	return &token, nil
}

// UserId2UserCacheSeconds controls the TTL for the full user object cache.
var UserId2UserCacheSeconds = config.SyncFrequency

// CacheGetUserById retrieves a full User (minus password/access_token) by ID,
// using Redis when available. On cache miss it falls back to GetUserById and populates the cache.
func CacheGetUserById(ctx context.Context, id int) (*User, error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetUserById(id, false)
	}
	cacheKey := fmt.Sprintf("user_obj:%d", id)
	cached, err := common.RedisGet(ctx, cacheKey)
	if err == nil {
		var user User
		if jsonErr := json.Unmarshal([]byte(cached), &user); jsonErr != nil {
			lg.Warn("Redis cached user object corrupted, falling back to database", zap.Int("user_id", id), zap.Error(jsonErr))
		} else {
			return &user, nil
		}
	}
	user, err := GetUserById(id, false)
	if err != nil {
		return nil, errors.Wrapf(err, "get user %d from database", id)
	}
	// The Redis object cache must persist the internal Id — otherwise a later
	// cache hit reconstructs a User with Id == 0 (issue #353), which propagates
	// into ctxkey.Id and surfaces as a 500 "user id is empty". With User.MarshalJSON
	// retired, json.Marshal(user) is now honest by default: it carries the Id (the
	// #353 fix) and, because Password/AccessToken/TotpSecret/VerificationCode are
	// json:"-", it cannot emit secrets — so the old plainUser alias and the manual
	// scrub are no longer needed. (This file is allowlisted in the noentityresponse
	// analyzer: marshaling the raw entity for the internal cache is intentional.)
	payload, err := json.Marshal(user)
	if err != nil {
		lg.Warn("failed to marshal user for cache", append(user.Ref().Zap(), zap.Error(err))...)
		return user, nil
	}
	if setErr := common.RedisSet(ctx, cacheKey, string(payload), time.Duration(UserId2UserCacheSeconds)*time.Second); setErr != nil {
		lg.Warn("Redis set user object failed, continuing without cache",
			append(user.Ref().Zap(), zap.Error(setErr))...)
	}
	return user, nil
}

func CacheGetUserGroup(ctx context.Context, id int) (group string, err error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetUserGroup(id)
	}
	group, err = common.RedisGet(ctx, fmt.Sprintf("user_group:%d", id))
	if err != nil {
		group, err = GetUserGroup(id)
		if err != nil {
			return "", errors.Wrapf(err, "get user group for user %d", id)
		}
		err = common.RedisSet(ctx, fmt.Sprintf("user_group:%d", id), group, time.Duration(UserId2GroupCacheSeconds)*time.Second)
		if err != nil {
			lg.Warn("Redis set user group failed, continuing without cache", zap.Int("user_id", id), zap.Error(err))
		}
	}
	if err != nil {
		return group, errors.Wrapf(err, "cache user group for user %d", id)
	}
	return group, nil
}

// CacheGetUsername retrieves a username by user ID, using Redis cache when available.
// On cache miss it falls back to GetUsernameById and populates the cache.
// Empty usernames (non-existent users) are not cached to allow retry on the next request.
func CacheGetUsername(ctx context.Context, id int) string {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetUsernameById(id)
	}
	username, err := common.RedisGet(ctx, fmt.Sprintf("user_username:%d", id))
	if err != nil {
		username = GetUsernameById(id)
		if username == "" {
			return username
		}
		if setErr := common.RedisSet(ctx, fmt.Sprintf("user_username:%d", id), username, time.Duration(UserId2UsernameCacheSeconds)*time.Second); setErr != nil {
			lg.Warn("Redis set username failed, continuing without cache",
				append(identity.NewUserRef(id, "", username).Zap(), zap.Error(setErr))...)
		}
	}
	return username
}

func fetchAndUpdateUserQuota(ctx context.Context, id int) (quota int64, err error) {
	lg := logger.FromContext(ctx)
	quota, err = GetUserQuota(id)
	if err != nil {
		return 0, errors.Wrap(err, "get user quota")
	}
	err = common.RedisSet(ctx, fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	if err != nil {
		lg.Warn("Redis set user quota failed, continuing without cache", zap.Int("user_id", id), zap.Error(err))
	}
	return
}

func CacheGetUserQuota(ctx context.Context, id int) (quota int64, err error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetUserQuota(id)
	}
	quotaString, err := common.RedisGet(ctx, fmt.Sprintf("user_quota:%d", id))
	if err != nil {
		return fetchAndUpdateUserQuota(ctx, id)
	}
	quota, err = strconv.ParseInt(quotaString, 10, 64)
	if err != nil {
		return 0, nil
	}
	if quota <= config.PreConsumedQuota { // when user's quota is less than pre-consumed quota, we need to fetch from db
		lg.Info("user's cached quota is too low, refreshing from db", zap.Int64("quota", quota), zap.Int("user_id", id))
		return fetchAndUpdateUserQuota(ctx, id)
	}
	return quota, nil
}

func CacheUpdateUserQuota(ctx context.Context, id int) error {
	if !common.IsRedisEnabled() {
		return nil
	}
	quota, err := GetUserQuota(id)
	if err != nil {
		return errors.Wrapf(err, "get database quota for user %d", id)
	}
	err = common.RedisSet(ctx, fmt.Sprintf("user_quota:%d", id), fmt.Sprintf("%d", quota), time.Duration(UserId2QuotaCacheSeconds)*time.Second)
	if err != nil {
		return errors.Wrapf(err, "set cached quota for user %d", id)
	}
	return nil
}

func CacheDecreaseUserQuota(ctx context.Context, id int, quota int64) error {
	if !common.IsRedisEnabled() {
		return nil
	}
	err := common.RedisDecrease(ctx, fmt.Sprintf("user_quota:%d", id), int64(quota))
	if err != nil {
		return errors.Wrapf(err, "decrease cached quota for user %d", id)
	}
	return nil
}

func CacheIsUserEnabled(ctx context.Context, userId int) (bool, error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return IsUserEnabled(userId)
	}
	enabled, err := common.RedisGet(ctx, fmt.Sprintf("user_enabled:%d", userId))
	if err == nil {
		return enabled == "1", nil
	}

	userEnabled, err := IsUserEnabled(userId)
	if err != nil {
		return false, errors.Wrapf(err, "check user %d enabled", userId)
	}
	enabled = "0"
	if userEnabled {
		enabled = "1"
	}
	err = common.RedisSet(ctx, fmt.Sprintf("user_enabled:%d", userId), enabled, time.Duration(UserId2StatusCacheSeconds)*time.Second)
	if err != nil {
		lg.Warn("Redis set user enabled failed, continuing without cache", zap.Int("user_id", userId), zap.Error(err))
	}
	if err != nil {
		return userEnabled, errors.Wrapf(err, "cache enabled status for user %d", userId)
	}
	return userEnabled, nil
}

// CacheGetGroupModels returns models of a group
//
// Deprecated: use CacheGetGroupModelsV2 instead
func CacheGetGroupModels(ctx context.Context, group string) (models []string, err error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetGroupModels(ctx, group)
	}
	modelsStr, err := common.RedisGet(ctx, fmt.Sprintf("group_models:%s", group))
	if err == nil {
		return strings.Split(modelsStr, ","), nil
	}
	models, err = GetGroupModels(ctx, group)
	if err != nil {
		return nil, errors.Wrap(err, "get group models")
	}
	err = common.RedisSet(ctx, fmt.Sprintf("group_models:%s", group), strings.Join(models, ","), time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		lg.Warn("Redis set group models failed, continuing without cache", zap.String("group", group), zap.Error(err))
	}
	return models, nil
}

// CacheGetGroupModelsV2 is a version of CacheGetGroupModels that returns EnabledAbility instead of string
func CacheGetGroupModelsV2(ctx context.Context, group string) (models []dto.EnabledAbility, err error) {
	lg := logger.FromContext(ctx)
	if !common.IsRedisEnabled() {
		return GetGroupModelsV2(ctx, group)
	}
	modelsStr, err := common.RedisGet(ctx, fmt.Sprintf("group_models_v2:%s", group))
	if err != nil {
		lg.Debug("Redis cache miss for group models, falling back to database", zap.String("group", group), zap.Error(err))
	} else {
		if err = json.Unmarshal([]byte(modelsStr), &models); err != nil {
			lg.Warn("Redis cached group models data corrupted, falling back to database", zap.String("group", group), zap.Error(err))
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
		lg.Warn("failed to marshal group models for cache, continuing without cache", zap.String("group", group), zap.Error(err))
		return models, nil
	}

	err = common.RedisSet(ctx, fmt.Sprintf("group_models_v2:%s", group), string(cachePayload),
		time.Duration(GroupModelsCacheSeconds)*time.Second)
	if err != nil {
		lg.Warn("Redis set group models failed, continuing without cache", zap.String("group", group), zap.Error(err))
	}

	return models, nil
}

var group2model2channels map[string]map[string][]*Channel

// channelId2channel is the id -> channel snapshot rebuilt by InitChannelCache.
// Guarded by channelSyncLock. It makes channel log enrichment free for code that
// holds only an integer channel id. It contains ENABLED channels only.
var channelId2channel map[int]*Channel
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
		channelGroups := channel.GetGroupNames()
		channelModels := channel.GetSupportedModelNames()

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
	channelId2channel = newChannelId2channel
	channelSyncLock.Unlock()
	logger.Logger.Info("channels synced from database, considering suspensions")
}

// cachedChannelById returns the cached channel row, or nil when the in-memory
// snapshot has not been built yet or the channel is disabled/unknown. It never
// queries the database, so it is safe to call from a logging path.
//
// Parameters:
//   - id: channel primary key.
//
// Return values:
//   - *Channel: the cached row, or nil when unavailable.
func cachedChannelById(id int) *Channel {
	if id <= 0 {
		return nil
	}
	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()
	if channelId2channel == nil {
		return nil
	}
	return channelId2channel[id]
}

func SyncChannelCache(frequency int) {
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		logger.Logger.Info("syncing channels from database")
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
	// It is important to make a copy if we are going to modify or iterate outside lock,
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

	if len(candidateChannels) == 0 {
		return nil, errors.Errorf("no channels in cache support model %s", model)
	}

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

	var channel *Channel
	if ignoreFirstPriority && endIdx < len(candidateChannels) {
		channel = pickWeightedChannel(candidateChannels[endIdx:])
		if channel == nil {
			channel = candidateChannels[random.RandRange(endIdx, len(candidateChannels))]
		}
	} else {
		channel = pickWeightedChannel(candidateChannels[:endIdx])
		if channel == nil {
			channel = candidateChannels[rand.Intn(endIdx)]
		}
	}
	logger.Logger.Info("select channel in cache", channel.Ref().Zap()...)
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
				if modelConfig != nil {
					smallerMaxTokensSizes[modelConfig.MaxTokens] = true
				}
			}
		}

		var LargerMaxTokensSizeChannels []*Channel
		// Work on already-filtered candidateChannels, not the original channelsFromCache
		for _, channel := range candidateChannels {
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
		return nil, errors.Errorf("no available channels support model %s after exclusions", model)
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
			channel := pickWeightedChannel(candidateChannels[endIdx:])
			if channel == nil {
				channel = candidateChannels[random.RandRange(endIdx, len(candidateChannels))]
			}
			logger.Logger.Info("select channel in cache", channel.Ref().Zap()...)
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

		channel := pickWeightedChannel(maxPriorityChannels)
		if channel == nil {
			channel = maxPriorityChannels[rand.Intn(len(maxPriorityChannels))]
		}
		logger.Logger.Info("select channel in cache", channel.Ref().Zap()...)
		return channel, nil
	}
}

// pickWeightedChannel selects a channel using weighted random over the channels'
// weights. When all weights are zero (or the slice is empty) it returns nil so the
// caller can fall back to uniform random. It shares its core algorithm with the DB
// path's selectAbilityByWeight via weightedIndex so both routing paths behave
// identically.
func pickWeightedChannel(channels []*Channel) *Channel {
	if len(channels) == 0 {
		return nil
	}

	weights := make([]uint, len(channels))
	for i, ch := range channels {
		weights[i] = ch.GetWeight()
	}

	idx := weightedIndex(weights, rand.Int63n)
	if idx < 0 {
		return nil // signal caller to use uniform random
	}
	return channels[idx]
}
