package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	relay "github.com/Laisky/one-api/relay"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
)

// GetModelsDisplay returns models available to the current user grouped by channel/adaptor with pricing information.
// This endpoint is designed for the Models display page in the frontend.
//
// The response data map is keyed by "<channel type name>:<channel name>", which is
// also each entry's channel_name. When several channels in one response share a type
// and name, the lowest channel id keeps that key and each later one, in ascending id
// order, is keyed "<channel type name>:<channel name>#2", "#3", ... skipping any key a
// real channel name already holds (see channelDisplayKeys). No channel is dropped, no
// channel id or UUID is exposed, and existing consumers keep working unchanged.
//
// Parameters: c is the request context; an authenticated user id narrows the listing
// to the user's group, and query parameters select keyword and capability filters.
// Returns: nothing; it writes a ModelsDisplayResponse or an error response.
func GetModelsDisplay(c *gin.Context) {
	// If logged-in, filter by user's allowed models; otherwise, show all supported models grouped by channel type
	userId := c.GetInt(ctxkey.Id)
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
	filters := parseModelDisplayFilters(c)
	lg := gmw.GetLogger(c)
	// One instant prices every model of this response, so all of them quote the
	// same tariff state, and the anonymous cache never mixes two states.
	displayNow := modelsDisplayNow()

	// Helper to build pricing info map for a channel with given model names
	convertRatioToPrice := func(r float64) float64 {
		if r <= 0 {
			return 0
		}
		if r < 0.001 {
			return r * 1_000_000
		}
		return (r * 1_000_000) / ratio.QuotaPerUsd
	}

	buildChannelModels := func(channel *model.Channel, modelNames []string, overrides map[string]model.ModelConfigLocal) map[string]ModelDisplayInfo {
		result := make(map[string]ModelDisplayInfo)
		// Get adaptor for this channel type (fallback to OpenAI for unsupported/custom)
		adaptor := relay.GetAdaptor(channeltype.ToAPIType(channel.Type))
		if adaptor == nil {
			adaptor = relay.GetAdaptor(apitype.OpenAI)
			if adaptor == nil {
				return result
			}
		}
		m := &meta.Meta{ChannelType: channel.Type}
		adaptor.Init(m)

		dc := modelDisplayContext{
			channel:        channel,
			adaptor:        adaptor,
			defaultPricing: adaptor.GetDefaultModelPricing(),
			convert:        convertRatioToPrice,
			now:            displayNow,
			lg:             lg,
		}
		modelMapping := channel.GetModelMappingWithContext(gmw.Ctx(c))

		for _, rawName := range modelNames {
			modelName := strings.TrimSpace(rawName)
			if modelName == "" {
				continue
			}
			if channel.IsModelHidden(modelName) {
				continue
			}
			if !channel.SupportsModel(modelName) {
				continue
			}
			if keyword != "" && !strings.Contains(strings.ToLower(modelName), keyword) {
				continue
			}
			// resolve mapped model for pricing
			actual := modelName
			if modelMapping != nil {
				if mapped, ok := modelMapping[modelName]; ok && mapped != "" {
					actual = mapped
				}
			}

			override := lookupDisplayOverride(overrides, modelName, actual)
			info := dc.buildModelDisplay(actual, override)
			if !filters.matchesModel(info) {
				continue
			}
			result[modelName] = info
			if info.InputPrice == 0 && info.CachedInputPrice == 0 && info.OutputPrice == 0 && info.ImagePrice == 0 && lg != nil {
				lg.Debug("model display missing pricing metadata",
					channel.Ref().AppendZap([]zap.Field{
						zap.String("model", modelName),
						zap.String("resolved_model", actual),
						zap.Bool("override_applied", override != nil),
					})...)
			}
		}
		return result
	}

	// If userId is zero, treat as anonymous: list all channels and their supported models from DB and adaptor
	if userId == 0 {
		buildResult := func() (map[string]ChannelModelsDisplayInfo, error) {
			channels, err := model.GetAllEnabledChannels()
			if err != nil {
				return nil, errors.Wrap(err, "get all enabled channels")
			}
			result := make(map[string]ChannelModelsDisplayInfo)
			keys := channelDisplayKeys(channels)
			for _, ch := range channels {
				if !filters.matchesChannel(ch.Type) {
					continue
				}
				overrides := ch.GetModelPriceConfigsWithContext(gmw.Ctx(c))
				supported := mergeModelNamesWithOverrides(ch.GetSupportedModelNames(), overrides)
				if len(supported) == 0 {
					continue
				}
				modelInfos := buildChannelModels(ch, supported, overrides)
				if len(modelInfos) == 0 {
					continue
				}
				key := keys[ch.Id]
				result[key] = ChannelModelsDisplayInfo{ChannelName: key, ChannelType: ch.Type, Models: modelInfos}
			}
			return result, nil
		}

		// Bypass the singleflight cache when filters are set: filter combinations explode
		// the cache key space and most filtered requests are user-driven.
		if filters.hasContent() {
			data, err := buildResult()
			if err != nil {
				helper.RespondError(c, err)
				return
			}
			c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: data})
			return
		}

		// Anonymous path with cache + singleflight to mitigate DB load and thundering herd.
		// Time windows are minute-granular, so keying by the display minute keeps
		// a cached response from quoting a tariff state that has since ended.
		cacheKey := "kw:" + keyword + ":at:" + strconv.FormatInt(displayNow.Unix()/60, 10)
		if version, err := model.GetEnabledChannelsVersionSignature(); err == nil {
			cacheKey += ":" + version
		}
		if data, ok := anonymousModelsDisplayCache.Load(cacheKey); ok {
			c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: data})
			return
		}

		v, err, _ := anonymousModelsDisplayGroup.Do(cacheKey, func() (any, error) {
			result, err := buildResult()
			if err != nil {
				return nil, errors.WithStack(err)
			}
			anonymousModelsDisplayCache.Store(cacheKey, result)
			return result, nil
		})
		if err != nil {
			helper.RespondError(c, errors.Wrap(err, "Failed to load channels"))
			return
		}
		data := v.(map[string]ChannelModelsDisplayInfo)
		c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: data})
		return
	}

	// Logged-in path: show only models allowed for the user group
	ctx, userGroup, err := getRequestUserGroup(c)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "Failed to get user group"))
		return
	}
	abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		helper.RespondError(c, errors.Wrap(err, "Failed to get available models"))
		return
	}

	result := make(map[string]ChannelModelsDisplayInfo)
	// Group abilities by channel ID and deduplicate models
	ch2models := make(map[int]map[string]struct{})
	for _, ab := range abilities {
		if _, ok := ch2models[ab.ChannelId]; !ok {
			ch2models[ab.ChannelId] = make(map[string]struct{})
		}
		ch2models[ab.ChannelId][ab.Model] = struct{}{}
	}
	channels := make([]*model.Channel, 0, len(ch2models))
	for chID := range ch2models {
		ch, err := model.GetChannelById(chID, true)
		if err != nil {
			// A stale ability can outlive its channel; the listing simply omits it.
			lg.Debug("skip models display channel that failed to load",
				zap.Int("channel_id", chID), zap.Error(err))
			continue
		}
		channels = append(channels, ch)
	}
	keys := channelDisplayKeys(channels)
	for _, ch := range channels {
		modelSet := ch2models[ch.Id]
		if !filters.matchesChannel(ch.Type) {
			continue
		}
		overrides := ch.GetModelPriceConfigsWithContext(gmw.Ctx(c))
		models := make([]string, 0, len(modelSet))
		for m := range modelSet {
			if ch.SupportsModel(m) {
				models = append(models, m)
			}
		}
		if len(models) == 0 {
			continue
		}
		sort.Strings(models)
		infos := buildChannelModels(ch, models, overrides)
		if len(infos) == 0 {
			continue
		}
		key := keys[ch.Id]
		result[key] = ChannelModelsDisplayInfo{ChannelName: key, ChannelType: ch.Type, Models: infos}
	}

	c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: result})
}

// channelDisplayKeys assigns every channel a unique GET /api/models/display response key.
//
// The legacy key is "<channel type name>:<channel name>". Two enabled channels may share
// a type and name (the channel "duplicate" action produces exactly that), and a plain map
// keyed by the legacy key would let the later one overwrite the earlier one, silently
// dropping a channel's models. Channels are therefore ranked by ascending id: the first
// channel claiming a legacy key keeps it, and each later channel with the same key gets
// the next free ordinal suffix "<legacy key>#2", "#3", ... in id order. Every channel's
// own legacy key is reserved before any suffix is assigned, so a suffix never displaces
// a channel whose real name already ends in "#N"; the ordinal simply keeps incrementing.
// The endpoint is public, so neither integer ids nor channel UUIDs appear in the keys;
// every known consumer treats the key as opaque and only strips the "<type name>:"
// prefix for its label.
//
// Parameters: channels is the candidate channel set; the slice itself is not reordered
// and nil entries are ignored.
// Returns: a map from channel id to its unique response key, also used as channel_name.
func channelDisplayKeys(channels []*model.Channel) map[int]string {
	ordered := make([]*model.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch != nil {
			ordered = append(ordered, ch)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Id < ordered[j].Id })

	keys := make(map[int]string, len(ordered))
	taken := make(map[string]struct{}, len(ordered))
	legacyKeys := make([]string, len(ordered))
	collided := make([]bool, len(ordered))
	for i, ch := range ordered {
		legacyKeys[i] = fmt.Sprintf("%s:%s", channeltype.IdToName(ch.Type), ch.Name)
		if _, exists := taken[legacyKeys[i]]; exists {
			collided[i] = true
			continue
		}
		taken[legacyKeys[i]] = struct{}{}
		keys[ch.Id] = legacyKeys[i]
	}
	for i, ch := range ordered {
		if !collided[i] {
			continue
		}
		for ordinal := 2; ; ordinal++ {
			candidate := fmt.Sprintf("%s#%d", legacyKeys[i], ordinal)
			if _, exists := taken[candidate]; exists {
				continue
			}
			taken[candidate] = struct{}{}
			keys[ch.Id] = candidate
			break
		}
	}
	return keys
}
