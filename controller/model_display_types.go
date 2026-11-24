package controller

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"

	"github.com/Laisky/one-api/model"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/channeltype"
	relaypricing "github.com/Laisky/one-api/relay/pricing"
)

// ModelsDisplayResponse represents the response structure for the models display page
type ModelsDisplayResponse struct {
	Success bool                                `json:"success"`
	Message string                              `json:"message"`
	Data    map[string]ChannelModelsDisplayInfo `json:"data"`
}

// ChannelModelsDisplayInfo represents model information for a specific channel/adaptor
type ChannelModelsDisplayInfo struct {
	ChannelName string                      `json:"channel_name"`
	ChannelType int                         `json:"channel_type"`
	Models      map[string]ModelDisplayInfo `json:"models"`
}

// ModelDisplayInfo represents display information for a single model
type ModelDisplayInfo struct {
	InputPrice                float64                  `json:"input_price"`                             // Price per 1M input tokens in USD
	CachedInputPrice          float64                  `json:"cached_input_price"`                      // Price per 1M cached input tokens in USD (falls back to input price when unspecified)
	CacheWrite5mPrice         float64                  `json:"cache_write_5m_price,omitempty"`          // Price per 1M tokens for 5-minute cache write
	CacheWrite1hPrice         float64                  `json:"cache_write_1h_price,omitempty"`          // Price per 1M tokens for 1-hour cache write
	OutputPrice               float64                  `json:"output_price"`                            // Price per 1M output tokens in USD
	MaxTokens                 int32                    `json:"max_tokens"`                              // Maximum tokens limit, 0 means unlimited
	ContextLength             int32                    `json:"context_length,omitempty"`                // Maximum total context window (input + output)
	MaxOutputTokens           int32                    `json:"max_output_tokens,omitempty"`             // Maximum output tokens per response
	InputModalities           []string                 `json:"input_modalities,omitempty"`              // Supported request input modalities
	OutputModalities          []string                 `json:"output_modalities,omitempty"`             // Supported response output modalities
	SupportedFeatures         []string                 `json:"supported_features,omitempty"`            // Capability flags such as tools/json_mode/reasoning
	SupportedSampling         []string                 `json:"supported_sampling_parameters,omitempty"` // Supported OpenAI-compatible sampling parameters
	SupportedReasoningEfforts []string                 `json:"supported_reasoning_efforts,omitempty"`   // Discrete reasoning_effort levels accepted (minimal/low/medium/high)
	DefaultReasoningEffort    string                   `json:"default_reasoning_effort,omitempty"`      // Default reasoning_effort the relay applies when omitted
	MaxReasoningTokens        int32                    `json:"max_reasoning_tokens,omitempty"`          // Upstream reasoning/thinking budget cap (Anthropic/Gemini style)
	Quantization              string                   `json:"quantization,omitempty"`                  // Numeric precision label (for OpenRouter-compatible metadata)
	HuggingFaceID             string                   `json:"hugging_face_id,omitempty"`               // HuggingFace model identifier when applicable
	Description               string                   `json:"description,omitempty"`                   // Human-readable short model description
	ImagePrice                float64                  `json:"image_price,omitempty"`                   // USD per image (image models only)
	Tiers                     []ModelDisplayTier       `json:"tiers,omitempty"`                         // Tiered pricing (volume-based)
	VideoPricing              *VideoDisplayPricing     `json:"video_pricing,omitempty"`                 // Video generation pricing
	AudioPricing              *AudioDisplayPricing     `json:"audio_pricing,omitempty"`                 // Audio prompt/completion pricing
	ImagePricing              *ImageDisplayPricing     `json:"image_pricing,omitempty"`                 // Detailed image pricing with size/quality multipliers
	EmbeddingPricing          *EmbeddingDisplayPricing `json:"embedding_pricing,omitempty"`             // Embedding pricing by modality
	PerCallPricing            *PerCallDisplayPricing   `json:"per_call_pricing,omitempty"`              // Flat per-invocation pricing (mutually exclusive with token pricing)
	PerPagePricing            *PerPageDisplayPricing   `json:"per_page_pricing,omitempty"`              // Flat per-processed-page pricing (mutually exclusive with token and per-call pricing)
	TimeWindows               []TimeWindowDisplay      `json:"time_windows,omitempty"`                  // Time-of-day pricing windows
	ActiveTimeWindow          string                   `json:"active_time_window,omitempty"`            // First active window name at display time
}

// TimeWindowDisplay represents one time-of-day pricing window for read-only model display.
type TimeWindowDisplay struct {
	Name       string                   `json:"name,omitempty"`         // Human-readable window label
	TimeZone   string                   `json:"timezone,omitempty"`     // IANA timezone name
	Ranges     []ClockRangeDisplay      `json:"ranges"`                 // Local wall-clock ranges
	DaysOfWeek []int                    `json:"days_of_week,omitempty"` // Optional weekday filter, Sunday=0
	DateFrom   string                   `json:"date_from,omitempty"`    // Inclusive local date bound
	DateTo     string                   `json:"date_to,omitempty"`      // Exclusive local date bound
	Overlay    TimeWindowOverlayDisplay `json:"overlay"`                // Sparse price overlay rendered for display
}

// ClockRangeDisplay represents one local wall-clock range in a display window.
type ClockRangeDisplay struct {
	Start string `json:"start"` // Inclusive local HH:MM start
	End   string `json:"end"`   // Exclusive local HH:MM end
}

// TimeWindowOverlayDisplay represents the displayable pricing fields overridden by a window.
type TimeWindowOverlayDisplay struct {
	InputPrice        float64                  `json:"input_price,omitempty"`
	CachedInputPrice  float64                  `json:"cached_input_price,omitempty"`
	CacheWrite5mPrice float64                  `json:"cache_write_5m_price,omitempty"`
	CacheWrite1hPrice float64                  `json:"cache_write_1h_price,omitempty"`
	OutputPrice       float64                  `json:"output_price,omitempty"`
	Tiers             []ModelDisplayTier       `json:"tiers,omitempty"`
	VideoPricing      *VideoDisplayPricing     `json:"video_pricing,omitempty"`
	AudioPricing      *AudioDisplayPricing     `json:"audio_pricing,omitempty"`
	ImagePricing      *ImageDisplayPricing     `json:"image_pricing,omitempty"`
	EmbeddingPricing  *EmbeddingDisplayPricing `json:"embedding_pricing,omitempty"`
	PerCallPricing    *PerCallDisplayPricing   `json:"per_call_pricing,omitempty"`
	PerPagePricing    *PerPageDisplayPricing   `json:"per_page_pricing,omitempty"`
}

// ModelDisplayTier represents a single tier in volume-based pricing
type ModelDisplayTier struct {
	InputPrice           float64 `json:"input_price"`                      // Price per 1M input tokens for this tier
	OutputPrice          float64 `json:"output_price"`                     // Price per 1M output tokens for this tier
	CachedInputPrice     float64 `json:"cached_input_price,omitempty"`     // Cached input price for this tier
	CacheWrite5mPrice    float64 `json:"cache_write_5m_price,omitempty"`   // 5-min cache write price for this tier
	CacheWrite1hPrice    float64 `json:"cache_write_1h_price,omitempty"`   // 1-hour cache write price for this tier
	InputTokenThreshold  int     `json:"input_token_threshold"`            // Minimum input tokens to reach this tier
	OutputTokenThreshold int     `json:"output_token_threshold,omitempty"` // Minimum output tokens to reach this tier
}

// VideoDisplayPricing represents video generation pricing for display
type VideoDisplayPricing struct {
	InputImageUsd         float64            `json:"input_image_usd,omitempty"`
	PerSecondUsd          float64            `json:"per_second_usd"`                   // USD per rendered second at base resolution
	BaseResolution        string             `json:"base_resolution,omitempty"`        // Base resolution (e.g. "1280x720")
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"` // Resolution -> multiplier map
}

// AudioDisplayPricing represents audio pricing for display
type AudioDisplayPricing struct {
	InputPriceQuantity float64 `json:"input_price_quantity,omitempty"`
	// InputUnit makes direct input pricing explicit: characters, utf8_bytes, or seconds.
	InputUnit               string  `json:"input_unit,omitempty"`
	InputPriceUsd           float64 `json:"input_price_usd,omitempty"`
	MinimumBillableSeconds  float64 `json:"minimum_billable_seconds,omitempty"`
	BillingIncrementSeconds float64 `json:"billing_increment_seconds,omitempty"`

	PromptTokenRatio          float64 `json:"prompt_token_ratio,omitempty"`           // Audio-to-text token conversion ratio for prompt
	CompletionTokenRatio      float64 `json:"completion_token_ratio,omitempty"`       // Audio-to-text token conversion ratio for completion
	PromptTokensPerSecond     float64 `json:"prompt_tokens_per_second,omitempty"`     // Tokens generated per second of prompt audio
	CompletionTokensPerSecond float64 `json:"completion_tokens_per_second,omitempty"` // Tokens generated per second of completion audio
	UsdPerSecond              float64 `json:"usd_per_second,omitempty"`               // Direct USD per second pricing
}

// ImageDisplayPricing represents detailed image pricing for display
type ImageDisplayPricing struct {
	PricePerImageUsd       float64                       `json:"price_per_image_usd,omitempty"`      // Base USD per image
	DefaultSize            string                        `json:"default_size,omitempty"`             // Default resolution
	DefaultQuality         string                        `json:"default_quality,omitempty"`          // Default quality level
	MinImages              int                           `json:"min_images,omitempty"`               // Minimum images per request
	MaxImages              int                           `json:"max_images,omitempty"`               // Maximum images per request
	SizeMultipliers        map[string]float64            `json:"size_multipliers,omitempty"`         // Resolution -> multiplier
	QualityMultipliers     map[string]float64            `json:"quality_multipliers,omitempty"`      // Quality -> multiplier
	QualitySizeMultipliers map[string]map[string]float64 `json:"quality_size_multipliers,omitempty"` // Quality -> Size -> multiplier
}

// PerCallDisplayPricing represents flat per-invocation pricing for display.
// Providers commonly price by query ("$X per 1K calls"); rerank is the canonical
// example. Display surfaces both per-1K-calls and the derived per-call USD figure.
type PerCallDisplayPricing struct {
	UsdPerThousandCalls float64 `json:"usd_per_thousand_calls"` // USD per 1000 invocations
	UsdPerCall          float64 `json:"usd_per_call"`           // Derived USD per single invocation
}

// EmbeddingDisplayPricing represents embedding pricing for display
type EmbeddingDisplayPricing struct {
	TextTokenPrice     float64 `json:"text_token_price,omitempty"`      // Price per 1M text tokens
	ImageTokenPrice    float64 `json:"image_token_price,omitempty"`     // Price per 1M image tokens
	AudioTokenPrice    float64 `json:"audio_token_price,omitempty"`     // Price per 1M audio tokens
	VideoTokenPrice    float64 `json:"video_token_price,omitempty"`     // Price per 1M video tokens
	DocumentTokenPrice float64 `json:"document_token_price,omitempty"`  // Price per 1M document tokens
	UsdPerImage        float64 `json:"usd_per_image,omitempty"`         // Direct USD per image
	UsdPerAudioSecond  float64 `json:"usd_per_audio_second,omitempty"`  // Direct USD per audio second
	UsdPerVideoFrame   float64 `json:"usd_per_video_frame,omitempty"`   // Direct USD per video frame
	UsdPerDocumentPage float64 `json:"usd_per_document_page,omitempty"` // Direct USD per document page
}

// buildDisplayTiers converts ratio tiers into display prices.
// Parameters: tiers is the pricing tier list, baseCompletionRatio is inherited by tiers with no completion override, and convertRatioToPrice converts quota ratios to USD per million tokens.
// Returns: display-ready tier entries.
func buildDisplayTiers(tiers []adaptorpkg.ModelRatioTier, baseCompletionRatio float64, convertRatioToPrice func(float64) float64) []ModelDisplayTier {
	if len(tiers) == 0 {
		return nil
	}
	display := make([]ModelDisplayTier, 0, len(tiers))
	for _, tier := range tiers {
		tierInput := convertRatioToPrice(tier.Ratio)
		tierCompletionRatio := tier.CompletionRatio
		if tierCompletionRatio == 0 {
			tierCompletionRatio = baseCompletionRatio
		}
		dt := ModelDisplayTier{
			InputPrice:           tierInput,
			OutputPrice:          tierInput * tierCompletionRatio,
			InputTokenThreshold:  tier.InputTokenThreshold,
			OutputTokenThreshold: tier.OutputTokenThreshold,
		}
		if tier.CachedInputRatio != 0 {
			dt.CachedInputPrice = convertRatioToPrice(tier.CachedInputRatio)
		}
		if tier.CacheWrite5mRatio != 0 {
			dt.CacheWrite5mPrice = convertRatioToPrice(tier.CacheWrite5mRatio)
		}
		if tier.CacheWrite1hRatio != 0 {
			dt.CacheWrite1hPrice = convertRatioToPrice(tier.CacheWrite1hRatio)
		}
		display = append(display, dt)
	}
	return display
}

// buildVideoDisplayPricing converts video pricing into display data.
// Parameters: cfg is the video pricing block.
// Returns: display pricing, or nil when no data is present.
func buildVideoDisplayPricing(cfg *adaptorpkg.VideoPricingConfig) *VideoDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	return &VideoDisplayPricing{
		PerSecondUsd:          cfg.PerSecondUsd,
		InputImageUsd:         cfg.InputImageUsd,
		BaseResolution:        cfg.BaseResolution,
		ResolutionMultipliers: cfg.ResolutionMultipliers,
	}
}

// buildAudioDisplayPricing converts audio pricing into display data.
// Parameters: cfg is the audio pricing block.
// Returns: display pricing, or nil when no data is present.
func buildAudioDisplayPricing(cfg *adaptorpkg.AudioPricingConfig) *AudioDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	return &AudioDisplayPricing{
		PromptTokenRatio:          cfg.PromptRatio,
		CompletionTokenRatio:      cfg.CompletionRatio,
		PromptTokensPerSecond:     cfg.PromptTokensPerSecond,
		CompletionTokensPerSecond: cfg.CompletionTokensPerSecond,
		UsdPerSecond:              cfg.UsdPerSecond,
		InputUnit:                 cfg.InputUnit,
		InputPriceUsd:             cfg.InputPriceUsd,
		InputPriceQuantity:        cfg.InputPriceQuantity,
		MinimumBillableSeconds:    cfg.MinimumBillableSeconds,
		BillingIncrementSeconds:   cfg.BillingIncrementSeconds,
	}
}

// buildEmbeddingDisplayPricing converts embedding pricing into display data.
// Parameters: cfg is the embedding pricing block and convertRatioToPrice converts quota ratios to USD per million tokens.
// Returns: display pricing, or nil when no data is present.
func buildEmbeddingDisplayPricing(cfg *adaptorpkg.EmbeddingPricingConfig, convertRatioToPrice func(float64) float64) *EmbeddingDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	return &EmbeddingDisplayPricing{
		TextTokenPrice:     convertRatioToPrice(cfg.TextTokenRatio),
		ImageTokenPrice:    convertRatioToPrice(cfg.ImageTokenRatio),
		AudioTokenPrice:    convertRatioToPrice(cfg.AudioTokenRatio),
		VideoTokenPrice:    convertRatioToPrice(cfg.VideoTokenRatio),
		DocumentTokenPrice: convertRatioToPrice(cfg.DocumentTokenRatio),
		UsdPerImage:        cfg.UsdPerImage,
		UsdPerAudioSecond:  cfg.UsdPerAudioSecond,
		UsdPerVideoFrame:   cfg.UsdPerVideoFrame,
		UsdPerDocumentPage: cfg.UsdPerDocumentPage,
	}
}

// buildPerCallDisplayPricing converts per-call pricing into display data.
// Parameters: cfg is the per-call pricing block.
// Returns: display pricing, or nil when no data is present.
func buildPerCallDisplayPricing(cfg *adaptorpkg.PerCallPricingConfig) *PerCallDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	return &PerCallDisplayPricing{
		UsdPerThousandCalls: cfg.UsdPerThousandCalls,
		UsdPerCall:          cfg.UsdPerThousandCalls / 1000.0,
	}
}

// buildTimeWindowOverlayDisplay converts a sparse pricing overlay into display prices.
// Parameters: overlay is the sparse pricing overlay, baseInputPrice and baseCompletionRatio provide inherited output context, and convertRatioToPrice converts quota ratios to USD per million tokens.
// Returns: displayable sparse overlay prices.
func buildTimeWindowOverlayDisplayWithBase(overlay adaptorpkg.ModelConfig, baseInputPrice float64, baseCompletionRatio float64, convertRatioToPrice func(float64) float64) TimeWindowOverlayDisplay {
	display := TimeWindowOverlayDisplay{}
	inputPrice := baseInputPrice
	if overlay.Ratio != 0 {
		display.InputPrice = convertRatioToPrice(overlay.Ratio)
		inputPrice = display.InputPrice
	}
	if overlay.CachedInputRatio != 0 {
		display.CachedInputPrice = convertRatioToPrice(overlay.CachedInputRatio)
	}
	if overlay.CacheWrite5mRatio != 0 {
		display.CacheWrite5mPrice = convertRatioToPrice(overlay.CacheWrite5mRatio)
	}
	if overlay.CacheWrite1hRatio != 0 {
		display.CacheWrite1hPrice = convertRatioToPrice(overlay.CacheWrite1hRatio)
	}
	if overlay.Ratio != 0 || overlay.CompletionRatio != 0 {
		completionRatio := baseCompletionRatio
		if overlay.CompletionRatio != 0 {
			completionRatio = overlay.CompletionRatio
		}
		display.OutputPrice = inputPrice * completionRatio
	}
	display.Tiers = buildDisplayTiers(overlay.Tiers, baseCompletionRatio, convertRatioToPrice)
	display.VideoPricing = buildVideoDisplayPricing(overlay.Video)
	display.AudioPricing = buildAudioDisplayPricing(overlay.Audio)
	display.ImagePricing = buildAdaptorImageDisplayPricing(overlay.Image)
	display.EmbeddingPricing = buildEmbeddingDisplayPricing(overlay.Embedding, convertRatioToPrice)
	display.PerCallPricing = buildPerCallDisplayPricing(overlay.PerCall)
	display.PerPagePricing = buildPerPageDisplayPricing(overlay.PerPage)
	return display
}

// buildAdaptorImageDisplayPricing converts adaptor image pricing into display data.
// Parameters: cfg is the adaptor image pricing block.
// Returns: display pricing, or nil when no data is present.
func buildAdaptorImageDisplayPricing(cfg *adaptorpkg.ImagePricingConfig) *ImageDisplayPricing {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	dp := &ImageDisplayPricing{
		PricePerImageUsd: cfg.PricePerImageUsd,
		DefaultSize:      cfg.DefaultSize,
		DefaultQuality:   cfg.DefaultQuality,
		MinImages:        cfg.MinImages,
		MaxImages:        cfg.MaxImages,
	}
	if len(cfg.SizeMultipliers) > 0 {
		dp.SizeMultipliers = cfg.SizeMultipliers
	}
	if len(cfg.QualityMultipliers) > 0 {
		dp.QualityMultipliers = cfg.QualityMultipliers
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		dp.QualitySizeMultipliers = cfg.QualitySizeMultipliers
	}
	return dp
}

// buildTimeWindowDisplays converts time windows into display data and active-window metadata.
// Parameters: windows is the ordered window list, baseInputPrice and baseCompletionRatio provide inherited output context, now is the display-time instant, and convertRatioToPrice converts quota ratios to USD per million tokens.
// Returns: display windows and the first active window name.
func buildTimeWindowDisplays(windows []adaptorpkg.TimeWindow, baseInputPrice float64, baseCompletionRatio float64, now time.Time, convertRatioToPrice func(float64) float64) ([]TimeWindowDisplay, string) {
	if len(windows) == 0 {
		return nil, ""
	}
	display := make([]TimeWindowDisplay, 0, len(windows))
	active := ""
	for _, window := range windows {
		ranges := make([]ClockRangeDisplay, 0, len(window.Ranges))
		for _, clockRange := range window.Ranges {
			ranges = append(ranges, ClockRangeDisplay{Start: clockRange.Start, End: clockRange.End})
		}
		display = append(display, TimeWindowDisplay{
			Name:       window.Name,
			TimeZone:   window.TimeZone,
			Ranges:     ranges,
			DaysOfWeek: append([]int(nil), window.DaysOfWeek...),
			DateFrom:   window.DateFrom,
			DateTo:     window.DateTo,
			Overlay:    buildTimeWindowOverlayDisplayWithBase(window.Overlay, baseInputPrice, baseCompletionRatio, convertRatioToPrice),
		})
		if active == "" && relaypricing.MatchTimeWindow(window, now) {
			active = window.Name
		}
	}
	return display, active
}

// convertLocalDisplayConfig converts channel-local pricing fields into adaptor-shaped display data.
// Parameters: cfg is the channel-local pricing configuration.
// Returns: an adaptor-shaped config for display rendering only.
func convertLocalDisplayConfig(cfg model.ModelConfigLocal) adaptorpkg.ModelConfig {
	converted := adaptorpkg.ModelConfig{
		Ratio:             cfg.Ratio,
		CompletionRatio:   cfg.CompletionRatio,
		CachedInputRatio:  cfg.CachedInputRatio,
		CacheWrite5mRatio: cfg.CacheWrite5mRatio,
		CacheWrite1hRatio: cfg.CacheWrite1hRatio,
		MaxTokens:         cfg.MaxTokens,
	}
	if len(cfg.Tiers) > 0 {
		converted.Tiers = make([]adaptorpkg.ModelRatioTier, 0, len(cfg.Tiers))
		for _, tier := range cfg.Tiers {
			converted.Tiers = append(converted.Tiers, adaptorpkg.ModelRatioTier{
				Ratio:                tier.Ratio,
				CompletionRatio:      tier.CompletionRatio,
				CachedInputRatio:     tier.CachedInputRatio,
				CacheWrite5mRatio:    tier.CacheWrite5mRatio,
				CacheWrite1hRatio:    tier.CacheWrite1hRatio,
				InputTokenThreshold:  tier.InputTokenThreshold,
				OutputTokenThreshold: tier.OutputTokenThreshold,
			})
		}
	}
	if cfg.PerCall != nil {
		converted.PerCall = &adaptorpkg.PerCallPricingConfig{UsdPerThousandCalls: cfg.PerCall.UsdPerThousandCalls}
	}
	converted.PerPage = convertLocalPerPageDisplayConfig(cfg.PerPage)
	if cfg.Video != nil {
		converted.Video = &adaptorpkg.VideoPricingConfig{
			PerSecondUsd:          cfg.Video.PerSecondUsd,
			InputImageUsd:         cfg.Video.InputImageUsd,
			BaseResolution:        cfg.Video.BaseResolution,
			ResolutionMultipliers: cfg.Video.ResolutionMultipliers,
		}
	}
	if cfg.Audio != nil {
		converted.Audio = &adaptorpkg.AudioPricingConfig{
			PromptRatio:               cfg.Audio.PromptRatio,
			CompletionRatio:           cfg.Audio.CompletionRatio,
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
	if cfg.Image != nil {
		converted.Image = &adaptorpkg.ImagePricingConfig{
			PricePerImageUsd:       cfg.Image.PricePerImageUsd,
			PromptRatio:            cfg.Image.PromptRatio,
			DefaultSize:            cfg.Image.DefaultSize,
			DefaultQuality:         cfg.Image.DefaultQuality,
			PromptTokenLimit:       cfg.Image.PromptTokenLimit,
			MinImages:              cfg.Image.MinImages,
			MaxImages:              cfg.Image.MaxImages,
			SizeMultipliers:        cfg.Image.SizeMultipliers,
			QualityMultipliers:     cfg.Image.QualityMultipliers,
			QualitySizeMultipliers: cfg.Image.QualitySizeMultipliers,
		}
	}
	if cfg.Embedding != nil {
		converted.Embedding = &adaptorpkg.EmbeddingPricingConfig{
			TextTokenRatio:     cfg.Embedding.TextTokenRatio,
			ImageTokenRatio:    cfg.Embedding.ImageTokenRatio,
			AudioTokenRatio:    cfg.Embedding.AudioTokenRatio,
			VideoTokenRatio:    cfg.Embedding.VideoTokenRatio,
			DocumentTokenRatio: cfg.Embedding.DocumentTokenRatio,
			UsdPerImage:        cfg.Embedding.UsdPerImage,
			UsdPerAudioSecond:  cfg.Embedding.UsdPerAudioSecond,
			UsdPerVideoFrame:   cfg.Embedding.UsdPerVideoFrame,
			UsdPerDocumentPage: cfg.Embedding.UsdPerDocumentPage,
		}
	}
	if len(cfg.TimeWindows) > 0 {
		converted.TimeWindows = convertLocalDisplayTimeWindows(cfg.TimeWindows)
	}
	return converted
}

// convertLocalDisplayTimeWindows converts channel-local time windows into adaptor-shaped display data.
// Parameters: windows is the local ordered time-window list.
// Returns: an adaptor-shaped ordered time-window list.
func convertLocalDisplayTimeWindows(windows []model.TimeWindowLocal) []adaptorpkg.TimeWindow {
	if len(windows) == 0 {
		return nil
	}
	converted := make([]adaptorpkg.TimeWindow, 0, len(windows))
	for _, window := range windows {
		ranges := make([]adaptorpkg.ClockRange, 0, len(window.Ranges))
		for _, clockRange := range window.Ranges {
			ranges = append(ranges, adaptorpkg.ClockRange{Start: clockRange.Start, End: clockRange.End})
		}
		converted = append(converted, adaptorpkg.TimeWindow{
			Name:       window.Name,
			TimeZone:   window.TimeZone,
			Ranges:     ranges,
			DaysOfWeek: append([]int(nil), window.DaysOfWeek...),
			DateFrom:   window.DateFrom,
			DateTo:     window.DateTo,
			Overlay:    convertLocalDisplayConfig(window.Overlay),
		})
	}
	return converted
}

// mergeModelNamesWithOverrides merges explicit channel models with pricing override entries, removing duplicates.
func mergeModelNamesWithOverrides(base []string, overrides map[string]model.ModelConfigLocal) []string {
	seen := make(map[string]struct{}, len(base))
	merged := make([]string, 0, len(base))
	for _, raw := range base {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		merged = append(merged, trimmed)
	}
	for raw := range overrides {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		merged = append(merged, trimmed)
	}
	return merged
}

// listAllSupportedModels builds a snapshot of every supported model, including admin-defined channel entries.
//
// TRADE OFF: deduplicate by case-insensitive model name, could miss some models with same name but different channels.
// Parameters: ctx is the request context used for channel configuration diagnostics.
// Returns: the supported model snapshot or a wrapped database error.
func listAllSupportedModels(ctx context.Context) ([]OpenAIModels, error) {
	channels, err := model.GetAllEnabledChannels()
	if err != nil {
		return nil, errors.Wrap(err, "get all enabled channels")
	}

	// Channels first, ranked the way routing ranks them: highest priority, then
	// lowest channel id. The first channel in this order that serves a name owns
	// it, so a model offered by two providers (two brands of one company, or an
	// open-weight id hosted by several upstreams) is attributed to the channel
	// most likely to actually serve it -- and the answer is stable across
	// restarts and replicas instead of following row order.
	sort.SliceStable(channels, func(i, j int) bool {
		if pi, pj := channels[i].GetPriority(), channels[j].GetPriority(); pi != pj {
			return pi > pj
		}
		return channels[i].Id < channels[j].Id
	})

	channelOwners := make(map[string]string)
	channelModels := make([]OpenAIModels, 0)
	for _, ch := range channels {
		overrides := ch.GetModelPriceConfigsWithContext(ctx)
		names := mergeModelNamesWithOverrides(ch.GetSupportedModelNames(), overrides)
		if len(names) == 0 {
			continue
		}
		owner := channeltype.IdToName(ch.Type)
		if owner == "" || owner == "unknown" {
			owner = channelUUIDOwner(ch.UUID)
		}
		for _, name := range names {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if _, exists := channelOwners[lower]; exists {
				continue
			}
			channelOwners[lower] = owner
			channelModels = append(channelModels, OpenAIModels{
				Id:         trimmed,
				Object:     "model",
				Created:    modelCatalogCreated,
				OwnedBy:    owner,
				Permission: defaultModelPermissions,
				Root:       trimmed,
				Parent:     nil,
			})
		}
	}

	// The compiled-in catalog then fills in every model no enabled channel
	// serves. This endpoint feeds the admin channel editor, which must offer the
	// full universe of model names before any channel exists -- so unlike
	// /v1/models it cannot be channel-derived alone. Where a channel does serve
	// the name, that channel's owner wins over the catalog's.
	models := make([]OpenAIModels, 0, len(allModels)+len(channelModels))
	seen := make(map[string]struct{}, len(allModels)+len(channelModels))
	for _, entry := range channelModels {
		models = append(models, entry)
		seen[strings.ToLower(entry.Id)] = struct{}{}
	}
	for _, base := range allModels {
		lower := strings.ToLower(base.Id)
		if _, exists := seen[lower]; exists {
			continue
		}
		models = append(models, base)
		seen[lower] = struct{}{}
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Id < models[j].Id
	})
	return models, nil
}
