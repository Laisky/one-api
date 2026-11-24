package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/model"
	relay "github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaypricing "github.com/Laisky/one-api/relay/pricing"
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

		defaultPricing := adaptor.GetDefaultModelPricing()
		modelMapping := channel.GetModelMappingWithContext(gmw.Ctx(c))
		displayNow := time.Now()
		getOverride := func(key string) (*model.ModelConfigLocal, bool) {
			if overrides == nil {
				return nil, false
			}
			cfg, ok := overrides[key]
			if !ok {
				return nil, false
			}
			copied := cfg
			return &copied, true
		}

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

			var inputPrice, cachedInputPrice, cacheWrite5mPrice, cacheWrite1hPrice, outputPrice float64
			var maxTokens int32
			var imagePrice float64
			var tiers []ModelDisplayTier
			var contextLength int32
			var maxOutputTokens int32
			var maxReasoningTokens int32
			var inputModalities []string
			var outputModalities []string
			var supportedFeatures []string
			var supportedSampling []string
			var supportedReasoningEfforts []string
			var defaultReasoningEffort string
			var quantization string
			var huggingFaceID string
			var description string
			var videoPricing *VideoDisplayPricing
			var perCallPricing *PerCallDisplayPricing
			var perPagePricing *PerPageDisplayPricing
			var audioPricing *AudioDisplayPricing
			var imagePricing *ImageDisplayPricing
			var embeddingPricing *EmbeddingDisplayPricing
			var timeWindows []TimeWindowDisplay
			var activeTimeWindow string
			baseCompletionRatio := 0.0
			// tierBase is the ratio-space token tariff billing prices tiers and
			// windows from: the provider default, or the channel config wholesale.
			var tierBase adaptorpkg.ModelConfig
			// providerSchedule is the provider's unwindowed token schedule (base
			// scalars, tiers and windows) a channel entry without ratio inherits.
			var providerSchedule adaptorpkg.ModelConfig
			overrideApplied := false

			// buildImageDisplayPricing converts an adaptor ImagePricingConfig to display format
			buildImageDisplayPricing := func(img interface{ HasData() bool }, raw interface{}) *ImageDisplayPricing {
				// Use type switch to handle both adaptor and local types
				switch v := raw.(type) {
				case *adaptorpkg.ImagePricingConfig:
					if v == nil || !v.HasData() {
						return nil
					}
					dp := &ImageDisplayPricing{
						PricePerImageUsd: v.PricePerImageUsd,
						DefaultSize:      v.DefaultSize,
						DefaultQuality:   v.DefaultQuality,
						MinImages:        v.MinImages,
						MaxImages:        v.MaxImages,
					}
					if len(v.SizeMultipliers) > 0 {
						dp.SizeMultipliers = v.SizeMultipliers
					}
					if len(v.QualityMultipliers) > 0 {
						dp.QualityMultipliers = v.QualityMultipliers
					}
					if len(v.QualitySizeMultipliers) > 0 {
						dp.QualitySizeMultipliers = v.QualitySizeMultipliers
					}
					return dp
				case *model.ImagePricingLocal:
					if v == nil {
						return nil
					}
					dp := &ImageDisplayPricing{
						PricePerImageUsd: v.PricePerImageUsd,
						DefaultSize:      v.DefaultSize,
						DefaultQuality:   v.DefaultQuality,
						MinImages:        v.MinImages,
						MaxImages:        v.MaxImages,
					}
					if len(v.SizeMultipliers) > 0 {
						dp.SizeMultipliers = v.SizeMultipliers
					}
					if len(v.QualityMultipliers) > 0 {
						dp.QualityMultipliers = v.QualityMultipliers
					}
					if len(v.QualitySizeMultipliers) > 0 {
						dp.QualitySizeMultipliers = v.QualitySizeMultipliers
					}
					return dp
				}
				return nil
			}
			_ = buildImageDisplayPricing // used below

			if cfg, ok := defaultPricing[actual]; ok {
				tierBase = displayTierBase(cfg)
				providerSchedule = tierBase
				providerSchedule.TimeWindows = cfg.TimeWindows
				timeWindows, activeTimeWindow = buildTimeWindowDisplays(cfg.TimeWindows, tierBase, convertRatioToPrice(cfg.Ratio), displayNow, convertRatioToPrice)
				perCallPricing = buildPerCallDisplayPricing(cfg.PerCall)
				perPagePricing = buildPerPageDisplayPricing(cfg.PerPage)
				inputPrice = convertRatioToPrice(cfg.Ratio)
				cachedInputPrice = inputPrice
				if cfg.CachedInputRatio != 0 {
					cachedInputPrice = convertRatioToPrice(cfg.CachedInputRatio)
					if inputPrice == 0 && cfg.CachedInputRatio > 0 {
						if lg != nil {
							lg.Debug("model display fell back to cached input ratio",
								channel.Ref().AppendZap([]zap.Field{
									zap.String("resolved_model", actual),
									zap.Float64("cached_ratio", cfg.CachedInputRatio),
								})...)
						}
						inputPrice = cachedInputPrice
					}
				}
				cacheWrite5mPrice = convertRatioToPrice(cfg.CacheWrite5mRatio)
				cacheWrite1hPrice = convertRatioToPrice(cfg.CacheWrite1hRatio)
				baseCompletionRatio = cfg.CompletionRatio
				outputPrice = inputPrice * cfg.CompletionRatio
				maxTokens = cfg.MaxTokens
				contextLength = cfg.ContextLength
				maxOutputTokens = cfg.MaxOutputTokens
				maxReasoningTokens = cfg.MaxReasoningTokens
				inputModalities = append([]string(nil), cfg.InputModalities...)
				outputModalities = append([]string(nil), cfg.OutputModalities...)
				supportedFeatures = append([]string(nil), cfg.SupportedFeatures...)
				supportedSampling = append([]string(nil), cfg.SupportedSamplingParameters...)
				supportedReasoningEfforts = append([]string(nil), cfg.SupportedReasoningEfforts...)
				defaultReasoningEffort = cfg.DefaultReasoningEffort
				quantization = cfg.Quantization
				huggingFaceID = cfg.HuggingFaceID
				description = cfg.Description
				if cfg.Image != nil {
					imagePrice = cfg.Image.PricePerImageUsd
					imagePricing = buildImageDisplayPricing(cfg.Image, cfg.Image)
				}
				// Tiered pricing, with billing's cumulative tier inheritance.
				tiers = buildDisplayTiers(tierBase, convertRatioToPrice)
				// Video pricing
				if cfg.Video != nil && cfg.Video.HasData() {
					videoPricing = &VideoDisplayPricing{
						PerSecondUsd:          cfg.Video.PerSecondUsd,
						InputImageUsd:         cfg.Video.InputImageUsd,
						BaseResolution:        cfg.Video.BaseResolution,
						ResolutionMultipliers: cfg.Video.ResolutionMultipliers,
					}
				}
				// Audio pricing
				if cfg.Audio != nil && cfg.Audio.HasData() {
					audioPricing = &AudioDisplayPricing{
						PromptTokenRatio:          cfg.Audio.PromptRatio,
						CompletionTokenRatio:      cfg.Audio.CompletionRatio,
						PromptTokensPerSecond:     cfg.Audio.PromptTokensPerSecond,
						CompletionTokensPerSecond: cfg.Audio.CompletionTokensPerSecond,
						UsdPerSecond:              cfg.Audio.UsdPerSecond,
						InputUnit:                 cfg.Audio.InputUnit,
						InputPriceUsd:             cfg.Audio.InputPriceUsd,
						InputPriceQuantity:        cfg.Audio.InputPriceQuantity,
						MinimumBillableSeconds:    cfg.Audio.MinimumBillableSeconds,
						BillingIncrementSeconds:   cfg.Audio.BillingIncrementSeconds,
					}
				}
				// Embedding pricing
				if cfg.Embedding != nil && cfg.Embedding.HasData() {
					embeddingPricing = &EmbeddingDisplayPricing{
						TextTokenPrice:     convertRatioToPrice(cfg.Embedding.TextTokenRatio),
						ImageTokenPrice:    convertRatioToPrice(cfg.Embedding.ImageTokenRatio),
						AudioTokenPrice:    convertRatioToPrice(cfg.Embedding.AudioTokenRatio),
						VideoTokenPrice:    convertRatioToPrice(cfg.Embedding.VideoTokenRatio),
						DocumentTokenPrice: convertRatioToPrice(cfg.Embedding.DocumentTokenRatio),
						UsdPerImage:        cfg.Embedding.UsdPerImage,
						UsdPerAudioSecond:  cfg.Embedding.UsdPerAudioSecond,
						UsdPerVideoFrame:   cfg.Embedding.UsdPerVideoFrame,
						UsdPerDocumentPage: cfg.Embedding.UsdPerDocumentPage,
					}
				}
			} else {
				inRatio := adaptor.GetModelRatio(actual)
				compRatio := adaptor.GetCompletionRatio(actual)
				inputPrice = convertRatioToPrice(inRatio)
				cachedInputPrice = inputPrice
				outputPrice = inputPrice * compRatio
				baseCompletionRatio = compRatio
				tierBase = adaptorpkg.ModelConfig{Ratio: inRatio, CompletionRatio: compRatio}
				maxTokens = 0
				imagePrice = 0
			}

			applyOverride := func(cfg *model.ModelConfigLocal) {
				if cfg.MaxTokens != 0 {
					maxTokens = cfg.MaxTokens
				}
				// Runtime admission prefers the channel context window when one is
				// configured (pricing.ResolveModelConfig carries it); zero keeps the
				// adaptor value. Apply it before the min_context_length filter.
				if cfg.ContextLength > 0 {
					contextLength = cfg.ContextLength
				}
				if cfg.Ratio != 0 {
					prevInputPrice := inputPrice
					inputPrice = convertRatioToPrice(cfg.Ratio)
					if cfg.CachedInputRatio != 0 {
						cachedInputPrice = convertRatioToPrice(cfg.CachedInputRatio)
					} else if cachedInputPrice == prevInputPrice {
						// No adaptor-level cache pricing existed, follow the new input price
						cachedInputPrice = inputPrice
					}
					// else: preserve the adaptor-level cachedInputPrice
					if cfg.CompletionRatio != 0 {
						outputPrice = inputPrice * cfg.CompletionRatio
					} else if baseCompletionRatio != 0 {
						outputPrice = inputPrice * baseCompletionRatio
					} else if outputPrice == 0 {
						outputPrice = inputPrice
					}
				} else if cfg.CompletionRatio != 0 && inputPrice > 0 {
					outputPrice = inputPrice * cfg.CompletionRatio
				}
				if cfg.CompletionRatio != 0 {
					baseCompletionRatio = cfg.CompletionRatio
				}
				if cfg.CacheWrite5mRatio != 0 {
					cacheWrite5mPrice = convertRatioToPrice(cfg.CacheWrite5mRatio)
				}
				if cfg.CacheWrite1hRatio != 0 {
					cacheWrite1hPrice = convertRatioToPrice(cfg.CacheWrite1hRatio)
				}
				if cfg.Image != nil && cfg.Image.PricePerImageUsd > 0 {
					imagePrice = cfg.Image.PricePerImageUsd
					imagePricing = buildImageDisplayPricing(nil, cfg.Image)
				}
				// Native tariffs have the same presence semantics as the billing
				// resolvers: a present zero rate is free, not missing configuration.
				converted := convertLocalDisplayConfig(*cfg)
				// Token billing selects the channel config wholesale: its own tiers,
				// already in billing order, replace the provider's and its cache
				// prices are not inherited. An omitted ratio inherits the provider's
				// schedule as a unit, its tiers and, without windows of its own, its
				// windows (relaypricing.InheritProviderSchedule); an omitted
				// completion_ratio falls back to the provider default, as relay/quota does.
				localSchedule := displayTierBase(converted)
				localSchedule.TimeWindows = convertLocalDisplayTimeWindows(cfg.TimeWindows)
				if !adaptorpkg.RequiresOperatorTariff(adaptor, actual) {
					localSchedule = relaypricing.InheritProviderSchedule(localSchedule, providerSchedule)
				}
				localTierBase := displayTierBase(localSchedule)
				if localTierBase.Ratio == 0 {
					localTierBase.Ratio = tierBase.Ratio
				}
				if localTierBase.CompletionRatio == 0 {
					localTierBase.CompletionRatio = tierBase.CompletionRatio
				}
				tierBase = localTierBase
				tiers = buildDisplayTiers(tierBase, convertRatioToPrice)
				// Billing resolves a channel config wholesale (pricing.ResolveModelConfig),
				// so a channel video or embedding block replaces the adaptor block
				// rather than merging field by field. Without one, billing falls back
				// to the provider default already rendered above.
				if converted.Video != nil && converted.Video.HasData() {
					videoPricing = buildVideoDisplayPricing(converted.Video)
				}
				if converted.Embedding != nil && converted.Embedding.HasData() {
					embeddingPricing = buildEmbeddingDisplayPricing(converted.Embedding, convertRatioToPrice)
				}
				if converted.PerCall != nil && converted.PerCall.HasData() {
					perCallPricing = buildPerCallDisplayPricing(converted.PerCall)
				} else if perCallPricing != nil && cfg.Ratio != 0 && cfg.Video == nil {
					// The video relay interprets a legacy scalar override as
					// quota/call, not quota/token. Keep its native display unit.
					perCallPricing = buildPerCallDisplayPricing(&adaptorpkg.PerCallPricingConfig{
						UsdPerThousandCalls: cfg.Ratio * 1000 / ratio.QuotaPerUsd,
					})
				}
				var dropInheritedSchedule bool
				perCallPricing, perPagePricing, dropInheritedSchedule = applyUnitTariffDisplayOverride(converted, perCallPricing, perPagePricing)
				if dropInheritedSchedule {
					// The local override replaces an inherited page tariff wholesale,
					// so the provider's page-price schedule no longer applies.
					timeWindows, activeTimeWindow = nil, ""
				}
				if converted.Audio != nil && converted.Audio.HasData() {
					audioPricing = buildAudioDisplayPricing(converted.Audio)
				} else if cfg.Audio == nil && cfg.Ratio != 0 {
					audioPricing = buildLegacyAudioTariffDisplay(audioPricing, cfg.Ratio)
				}
				hasUnit := perCallPricing != nil || perPagePricing != nil || audioPricing != nil
				hasMedia := videoPricing != nil || imagePricing != nil || embeddingPricing != nil
				dropInheritedSchedule, cachedInputPrice = applyLocalTokenDisplayOverride(
					*cfg, converted, inputPrice, cachedInputPrice, hasUnit, hasMedia, convertRatioToPrice)
				if dropInheritedSchedule {
					timeWindows, activeTimeWindow = nil, ""
				}
				// The entry's own windows, or the provider windows a token entry
				// inherits with token-only overlays.
				if len(cfg.TimeWindows) > 0 || (!hasUnit && !hasMedia && len(localSchedule.TimeWindows) > 0) {
					timeWindows, activeTimeWindow = buildTimeWindowDisplays(localSchedule.TimeWindows, tierBase, inputPrice, displayNow, convertRatioToPrice)
				}
			}

			if cfg, ok := getOverride(modelName); ok {
				overrideApplied = true
				applyOverride(cfg)
			}
			if !overrideApplied && actual != modelName {
				if cfg, ok := getOverride(actual); ok {
					overrideApplied = true
					applyOverride(cfg)
				}
			}

			if perCallPricing != nil || perPagePricing != nil {
				inputPrice, outputPrice, cachedInputPrice = 0, 0, 0
				cacheWrite5mPrice, cacheWrite1hPrice = 0, 0
				tiers = nil
			}

			info := ModelDisplayInfo{
				InputPrice:                inputPrice,
				CachedInputPrice:          cachedInputPrice,
				CacheWrite5mPrice:         cacheWrite5mPrice,
				CacheWrite1hPrice:         cacheWrite1hPrice,
				OutputPrice:               outputPrice,
				MaxTokens:                 maxTokens,
				ContextLength:             contextLength,
				MaxOutputTokens:           maxOutputTokens,
				MaxReasoningTokens:        maxReasoningTokens,
				InputModalities:           inputModalities,
				OutputModalities:          outputModalities,
				SupportedFeatures:         supportedFeatures,
				SupportedSampling:         supportedSampling,
				SupportedReasoningEfforts: supportedReasoningEfforts,
				DefaultReasoningEffort:    defaultReasoningEffort,
				Quantization:              quantization,
				HuggingFaceID:             huggingFaceID,
				Description:               description,
				ImagePrice:                imagePrice,
				Tiers:                     tiers,
				VideoPricing:              videoPricing,
				AudioPricing:              audioPricing,
				PerCallPricing:            perCallPricing,
				PerPagePricing:            perPagePricing,
				ImagePricing:              imagePricing,
				EmbeddingPricing:          embeddingPricing,
				TimeWindows:               timeWindows,
				ActiveTimeWindow:          activeTimeWindow,
			}
			if !filters.matchesModel(info) {
				continue
			}
			result[modelName] = info
			if inputPrice == 0 && cachedInputPrice == 0 && outputPrice == 0 && imagePrice == 0 && lg != nil {
				lg.Debug("model display missing pricing metadata",
					channel.Ref().AppendZap([]zap.Field{
						zap.String("model", modelName),
						zap.String("resolved_model", actual),
						zap.Bool("override_applied", overrideApplied),
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

		// Anonymous path with cache + singleflight to mitigate DB load and thundering herd
		cacheKey := "kw:" + keyword
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
