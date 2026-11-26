package controller

import (
	"time"

	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"

	"github.com/Laisky/one-api/model"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
	relaypricing "github.com/Laisky/one-api/relay/pricing"
)

// displayWindowSource identifies which price schedule a model display's time
// windows belong to, and therefore where billing merges their overlays.
type displayWindowSource int

const (
	// displayWindowsNone means the displayed tariff has no time windows.
	displayWindowsNone displayWindowSource = iota
	// displayWindowsProvider means the windows are the provider-default (or
	// global) catalog's, merged into that catalog entry.
	displayWindowsProvider
	// displayWindowsChannel means the windows are the channel entry's own, or
	// the provider windows the entry inherits, merged into the channel entry.
	displayWindowsChannel
)

// modelDisplayView selects the tariff state one render prices. The zero value
// renders the base schedule; a view with an overlay renders the tariff billing
// applies while the window carrying that overlay is the first match.
type modelDisplayView struct {
	source  displayWindowSource
	overlay *adaptorpkg.ModelConfig
}

// mergesProvider reports whether the view reprices the provider catalog entry.
// It takes no parameters and returns true only for a provider-window overlay.
func (v modelDisplayView) mergesProvider() bool {
	return v.overlay != nil && v.source == displayWindowsProvider
}

// mergesChannel reports whether the view reprices the channel entry.
// It takes no parameters and returns true only for a channel-window overlay.
func (v modelDisplayView) mergesChannel() bool {
	return v.overlay != nil && v.source == displayWindowsChannel
}

// modelDisplayRender is one rendered model and the schedule it was priced from.
type modelDisplayRender struct {
	info    ModelDisplayInfo
	source  displayWindowSource
	windows []adaptorpkg.TimeWindow
}

// modelDisplayContext carries the per-channel inputs every model render shares.
type modelDisplayContext struct {
	channel        *model.Channel
	adaptor        adaptorpkg.Adaptor
	defaultPricing map[string]adaptorpkg.ModelConfig
	convert        func(float64) float64
	now            time.Time
	lg             glog.Logger
}

// lookupDisplayOverride returns the channel entry billing prices a listed model
// with. Parameters: overrides is the channel model_configs, listed the
// client-facing model name and actual its mapped upstream name. Returns: a copy
// of the entry keyed by listed, else by actual, else nil.
func lookupDisplayOverride(overrides map[string]model.ModelConfigLocal, listed, actual string) *model.ModelConfigLocal {
	for _, key := range []string{listed, actual} {
		if cfg, ok := overrides[key]; ok {
			copied := cfg
			return &copied
		}
		if listed == actual {
			break
		}
	}
	return nil
}

// buildModelDisplay renders one model at display time.
//
// The top-level prices are what billing charges at dc.now: the base schedule,
// or, while a time window is the first match, the tariff billing's own merge
// (relaypricing.ApplyTimeWindowOverlay) produces for that window. Each window
// also carries the complete tariff it applies (effective_pricing), so a client
// that tracks the active window live switches tariffs without pricing anything
// itself, and the base schedule is kept as inactive_base_pricing while a window
// replaces it. Parameters: actual is the mapped model billing prices and
// override the channel entry (nil when none). Returns: the display record.
func (dc modelDisplayContext) buildModelDisplay(actual string, override *model.ModelConfigLocal) ModelDisplayInfo {
	base := dc.renderModel(actual, override, modelDisplayView{})
	info := base.info
	if base.source == displayWindowsNone || len(base.windows) != len(info.TimeWindows) {
		return info
	}
	active := -1
	for i := range base.windows {
		overlay := base.windows[i].Overlay
		tariff := dc.renderModel(actual, override, modelDisplayView{source: base.source, overlay: &overlay}).info.ModelDisplayTariff
		info.TimeWindows[i].EffectivePricing = &tariff
		if active < 0 && relaypricing.MatchTimeWindow(base.windows[i], dc.now) {
			active = i
		}
	}
	if active >= 0 {
		inactive := info.ModelDisplayTariff
		info.InactiveBasePricing = &inactive
		info.ModelDisplayTariff = *info.TimeWindows[active].EffectivePricing
		if dc.lg != nil && dc.channel != nil {
			dc.lg.Debug("model display quotes the active time window tariff",
				dc.channel.Ref().AppendZap([]zap.Field{
					zap.String("resolved_model", actual),
					zap.Int("window_index", active),
					zap.String("window_name", base.windows[active].Name),
					zap.Bool("channel_schedule", base.source == displayWindowsChannel),
				})...)
		}
	}
	return info
}

// renderModel prices one model for one tariff state.
//
// It follows billing's layer order: the provider default (or the adaptor's
// scalar ratios), then the channel entry, which inherits the provider schedule
// when it omits a base ratio. A view with an overlay merges it where billing
// does: into the provider entry for provider windows, or into the channel
// entry after schedule inheritance for channel windows; the provider layer a
// channel entry falls back to then stays unwindowed, as in billing.
// Parameters: actual is the mapped model, override the channel entry (nil when
// none) and view the tariff state. Returns: the rendered model, plus the base
// view's window source and windows in billing order.
func (dc modelDisplayContext) renderModel(actual string, override *model.ModelConfigLocal, view modelDisplayView) modelDisplayRender {
	convertRatioToPrice := dc.convert
	channel, adaptor, lg := dc.channel, dc.adaptor, dc.lg

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
	// windowSource and scheduleWindows record which schedule timeWindows renders.
	windowSource := displayWindowsNone
	var scheduleWindows []adaptorpkg.TimeWindow
	baseCompletionRatio := 0.0
	// tierBase is the ratio-space token tariff billing prices tiers and
	// windows from: the provider default, or the channel config wholesale.
	var tierBase adaptorpkg.ModelConfig
	// providerSchedule is the provider's unwindowed token schedule (base
	// scalars, tiers and windows) a channel entry without ratio inherits.
	var providerSchedule adaptorpkg.ModelConfig

	if cfg, ok := dc.defaultPricing[actual]; ok {
		if view.mergesProvider() {
			cfg = relaypricing.ApplyTimeWindowOverlay(cfg, *view.overlay)
		}
		tierBase = displayTierBase(cfg)
		providerSchedule = tierBase
		providerSchedule.TimeWindows = cfg.TimeWindows
		timeWindows, activeTimeWindow = buildTimeWindowDisplays(cfg.TimeWindows, tierBase, convertRatioToPrice(cfg.Ratio), dc.now, convertRatioToPrice)
		if len(timeWindows) > 0 {
			windowSource, scheduleWindows = displayWindowsProvider, cfg.TimeWindows
		}
		perCallPricing = buildPerCallDisplayPricing(cfg.PerCall)
		perPagePricing = buildPerPageDisplayPricing(cfg.PerPage)
		inputPrice = convertRatioToPrice(cfg.Ratio)
		cachedInputPrice = inputPrice
		if cfg.CachedInputRatio != 0 {
			cachedInputPrice = convertRatioToPrice(cfg.CachedInputRatio)
			if inputPrice == 0 && cfg.CachedInputRatio > 0 {
				if lg != nil && view.overlay == nil {
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
			imagePricing = buildAdaptorImageDisplayPricing(cfg.Image)
		}
		// Tiered pricing, with billing's cumulative tier inheritance.
		tiers = buildDisplayTiers(tierBase, convertRatioToPrice)
		videoPricing = buildVideoDisplayPricing(cfg.Video)
		audioPricing = buildAudioDisplayPricing(cfg.Audio)
		embeddingPricing = buildEmbeddingDisplayPricing(cfg.Embedding, convertRatioToPrice)
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

	if cfg := override; cfg != nil {
		if cfg.MaxTokens != 0 {
			maxTokens = cfg.MaxTokens
		}
		// Runtime admission prefers the channel context window when one is
		// configured (pricing.ResolveModelConfig carries it); zero keeps the
		// adaptor value. Apply it before the min_context_length filter.
		if cfg.ContextLength > 0 {
			contextLength = cfg.ContextLength
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
		baseLocalWindows := localSchedule.TimeWindows
		if view.mergesChannel() {
			// Billing merges the active window after schedule inheritance. An
			// inherited window carries token prices only, so it leaves the
			// entry's media blocks as configured.
			localSchedule = relaypricing.ApplyTimeWindowOverlay(localSchedule, *view.overlay)
			converted = relaypricing.ApplyTimeWindowOverlay(converted, *view.overlay)
		}
		// priced holds the entry's prices in this view; cfg keeps deciding
		// which tariff kind the entry configures.
		priced := displayTierBase(localSchedule)

		if priced.Ratio != 0 {
			prevInputPrice := inputPrice
			inputPrice = convertRatioToPrice(priced.Ratio)
			if priced.CachedInputRatio != 0 {
				cachedInputPrice = convertRatioToPrice(priced.CachedInputRatio)
			} else if cachedInputPrice == prevInputPrice {
				// No adaptor-level cache pricing existed, follow the new input price
				cachedInputPrice = inputPrice
			}
			// else: preserve the adaptor-level cachedInputPrice
			if priced.CompletionRatio != 0 {
				outputPrice = inputPrice * priced.CompletionRatio
			} else if baseCompletionRatio != 0 {
				outputPrice = inputPrice * baseCompletionRatio
			} else if outputPrice == 0 {
				outputPrice = inputPrice
			}
		} else if priced.CompletionRatio != 0 && inputPrice > 0 {
			outputPrice = inputPrice * priced.CompletionRatio
		}
		if priced.CompletionRatio != 0 {
			baseCompletionRatio = priced.CompletionRatio
		}
		if priced.CacheWrite5mRatio != 0 {
			cacheWrite5mPrice = convertRatioToPrice(priced.CacheWrite5mRatio)
		}
		if priced.CacheWrite1hRatio != 0 {
			cacheWrite1hPrice = convertRatioToPrice(priced.CacheWrite1hRatio)
		}
		if cfg.Image != nil && converted.Image != nil && converted.Image.PricePerImageUsd > 0 {
			imagePrice = converted.Image.PricePerImageUsd
			imagePricing = buildAdaptorImageDisplayPricing(converted.Image)
		}
		localTierBase := priced
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
				UsdPerThousandCalls: priced.Ratio * 1000 / ratio.QuotaPerUsd,
			})
		}
		var dropInheritedSchedule bool
		perCallPricing, perPagePricing, dropInheritedSchedule = applyUnitTariffDisplayOverride(converted, perCallPricing, perPagePricing)
		if dropInheritedSchedule {
			// The local override replaces an inherited page tariff wholesale,
			// so the provider's page-price schedule no longer applies.
			timeWindows, activeTimeWindow = nil, ""
			windowSource, scheduleWindows = displayWindowsNone, nil
		}
		if converted.Audio != nil && converted.Audio.HasData() {
			audioPricing = buildAudioDisplayPricing(converted.Audio)
		} else if cfg.Audio == nil && cfg.Ratio != 0 {
			audioPricing = buildLegacyAudioTariffDisplay(audioPricing, priced.Ratio)
		}
		hasUnit := perCallPricing != nil || perPagePricing != nil || audioPricing != nil
		hasMedia := videoPricing != nil || imagePricing != nil || embeddingPricing != nil
		pricedLocal := *cfg
		pricedLocal.CachedInputRatio = priced.CachedInputRatio
		dropInheritedSchedule, cachedInputPrice = applyLocalTokenDisplayOverride(
			pricedLocal, converted, inputPrice, cachedInputPrice, hasUnit, hasMedia, convertRatioToPrice)
		if dropInheritedSchedule {
			timeWindows, activeTimeWindow = nil, ""
			windowSource, scheduleWindows = displayWindowsNone, nil
		}
		// The entry's own windows, or the provider windows a token entry
		// inherits with token-only overlays.
		if len(cfg.TimeWindows) > 0 || (!hasUnit && !hasMedia && len(baseLocalWindows) > 0) {
			timeWindows, activeTimeWindow = buildTimeWindowDisplays(baseLocalWindows, tierBase, inputPrice, dc.now, convertRatioToPrice)
			windowSource, scheduleWindows = displayWindowsNone, nil
			if len(timeWindows) > 0 {
				windowSource, scheduleWindows = displayWindowsChannel, baseLocalWindows
			}
		}
	}

	if perCallPricing != nil || perPagePricing != nil {
		inputPrice, outputPrice, cachedInputPrice = 0, 0, 0
		cacheWrite5mPrice, cacheWrite1hPrice = 0, 0
		tiers = nil
	}

	return modelDisplayRender{
		info: ModelDisplayInfo{
			ModelDisplayTariff: ModelDisplayTariff{
				InputPrice:        inputPrice,
				CachedInputPrice:  cachedInputPrice,
				CacheWrite5mPrice: cacheWrite5mPrice,
				CacheWrite1hPrice: cacheWrite1hPrice,
				OutputPrice:       outputPrice,
				ImagePrice:        imagePrice,
				Tiers:             tiers,
				VideoPricing:      videoPricing,
				AudioPricing:      audioPricing,
				PerCallPricing:    perCallPricing,
				PerPagePricing:    perPagePricing,
				ImagePricing:      imagePricing,
				EmbeddingPricing:  embeddingPricing,
			},
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
			TimeWindows:               timeWindows,
			ActiveTimeWindow:          activeTimeWindow,
		},
		source:  windowSource,
		windows: scheduleWindows,
	}
}
