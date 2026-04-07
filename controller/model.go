package controller

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/middleware"
	"github.com/songquanpeng/one-api/model"
	relay "github.com/songquanpeng/one-api/relay"
	adaptorpkg "github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/apitype"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// https://platform.openai.com/docs/api-reference/models/list

type OpenAIModelPermission struct {
	Id                 string  `json:"id"`
	Object             string  `json:"object"`
	Created            int     `json:"created"`
	AllowCreateEngine  bool    `json:"allow_create_engine"`
	AllowSampling      bool    `json:"allow_sampling"`
	AllowLogprobs      bool    `json:"allow_logprobs"`
	AllowSearchIndices bool    `json:"allow_search_indices"`
	AllowView          bool    `json:"allow_view"`
	AllowFineTuning    bool    `json:"allow_fine_tuning"`
	Organization       string  `json:"organization"`
	Group              *string `json:"group"`
	IsBlocking         bool    `json:"is_blocking"`
}

type OpenAIModels struct {
	// Id model's name
	//
	// BUG: Different channels may have the same model name
	Id      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	// OwnedBy is the channel's adaptor name
	OwnedBy    string                  `json:"owned_by"`
	Permission []OpenAIModelPermission `json:"permission"`
	Root       string                  `json:"root"`
	Parent     *string                 `json:"parent"`
}

// BUG(#39): 更新 custom channel 时，应该同步更新所有自定义的 models 到 allModels
var (
	allModels               []OpenAIModels
	modelsMap               map[string]OpenAIModels
	channelId2Models        map[int][]string
	defaultModelPermissions []OpenAIModelPermission
)

// Anonymous models display cache (1-minute TTL) to avoid repeated heavy loads.
// Keyed by normalized keyword filter.
var (
	anonymousModelsDisplayCache = gutils.NewExpCache[map[string]ChannelModelsDisplayInfo](context.Background(), time.Minute)
	anonymousModelsDisplayGroup singleflight.Group
)

func init() {
	var permission []OpenAIModelPermission
	permission = append(permission, OpenAIModelPermission{
		Id:                 "modelperm-LwHkVFn8AcMItP432fKKDIKJ",
		Object:             "model_permission",
		Created:            1626777600,
		AllowCreateEngine:  true,
		AllowSampling:      true,
		AllowLogprobs:      true,
		AllowSearchIndices: false,
		AllowView:          true,
		AllowFineTuning:    false,
		Organization:       "*",
		Group:              nil,
		IsBlocking:         false,
	})
	defaultModelPermissions = append([]OpenAIModelPermission(nil), permission...)
	// https://platform.openai.com/docs/models/model-endpoint-compatibility
	for i := range apitype.Dummy {
		if i == apitype.AIProxyLibrary {
			continue
		}
		adaptor := relay.GetAdaptor(i)
		if adaptor == nil {
			continue
		}

		channelName := adaptor.GetChannelName()
		modelNames := adaptor.GetModelList()
		for _, modelName := range modelNames {
			allModels = append(allModels, OpenAIModels{
				Id:         modelName,
				Object:     "model",
				Created:    1626777600,
				OwnedBy:    channelName,
				Permission: permission,
				Root:       modelName,
				Parent:     nil,
			})
		}
	}
	for _, channelType := range openai.CompatibleChannels {
		if channelType == channeltype.Azure {
			continue
		}
		channelName, channelModelList := openai.GetCompatibleChannelMeta(channelType)
		for _, modelName := range channelModelList {
			allModels = append(allModels, OpenAIModels{
				Id:         modelName,
				Object:     "model",
				Created:    1626777600,
				OwnedBy:    channelName,
				Permission: permission,
				Root:       modelName,
				Parent:     nil,
			})
		}
	}
	modelsMap = make(map[string]OpenAIModels)
	for _, model := range allModels {
		modelsMap[model.Id] = model
	}
	channelId2Models = make(map[int][]string)
	for i := 1; i < channeltype.Dummy; i++ {
		adaptor := relay.GetAdaptor(channeltype.ToAPIType(i))
		if adaptor == nil {
			continue
		}

		meta := &meta.Meta{
			ChannelType: i,
		}
		adaptor.Init(meta)
		channelId2Models[i] = adaptor.GetModelList()
	}
}

// DashboardListModels returns the complete channel-to-model mapping for administrative dashboards.
func DashboardListModels(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channelId2Models,
	})
}

type listAllModelsCacheEntry struct {
	Models  []OpenAIModels
	Version string
}

// cachedListAllModels is a short-term cache for ListAllModels to reduce load.
var cachedListAllModels = gutils.NewSingleItemExpCache[listAllModelsCacheEntry](time.Minute)

// ListAllModels returns every known model in the OpenAI-compatible format regardless of user permissions.
func ListAllModels(c *gin.Context) {
	models, err := getSupportedModelsSnapshot()
	if err != nil {
		middleware.AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "load supported models"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func getSupportedModelsSnapshot() ([]OpenAIModels, error) {
	version, err := model.GetEnabledChannelsVersionSignature()
	if err != nil {
		return nil, errors.Wrap(err, "channels version signature")
	}

	if entry, ok := cachedListAllModels.Get(); ok && entry.Version == version {
		return entry.Models, nil
	}

	models, err := listAllSupportedModels()
	if err != nil {
		return nil, errors.Wrap(err, "list models")
	}

	cachedListAllModels.Set(listAllModelsCacheEntry{
		Models:  models,
		Version: version,
	})

	return models, nil
}

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
	InputPrice        float64                  `json:"input_price"`                    // Price per 1M input tokens in USD
	CachedInputPrice  float64                  `json:"cached_input_price"`             // Price per 1M cached input tokens in USD (falls back to input price when unspecified)
	CacheWrite5mPrice float64                  `json:"cache_write_5m_price,omitempty"` // Price per 1M tokens for 5-minute cache write
	CacheWrite1hPrice float64                  `json:"cache_write_1h_price,omitempty"` // Price per 1M tokens for 1-hour cache write
	OutputPrice       float64                  `json:"output_price"`                   // Price per 1M output tokens in USD
	MaxTokens         int32                    `json:"max_tokens"`                     // Maximum tokens limit, 0 means unlimited
	ImagePrice        float64                  `json:"image_price,omitempty"`          // USD per image (image models only)
	Tiers             []ModelDisplayTier       `json:"tiers,omitempty"`                // Tiered pricing (volume-based)
	VideoPricing      *VideoDisplayPricing     `json:"video_pricing,omitempty"`        // Video generation pricing
	AudioPricing      *AudioDisplayPricing     `json:"audio_pricing,omitempty"`        // Audio prompt/completion pricing
	ImagePricing      *ImageDisplayPricing     `json:"image_pricing,omitempty"`        // Detailed image pricing with size/quality multipliers
	EmbeddingPricing  *EmbeddingDisplayPricing `json:"embedding_pricing,omitempty"`    // Embedding pricing by modality
}

// ModelDisplayTier represents a single tier in volume-based pricing
type ModelDisplayTier struct {
	InputPrice          float64 `json:"input_price"`                    // Price per 1M input tokens for this tier
	OutputPrice         float64 `json:"output_price"`                   // Price per 1M output tokens for this tier
	CachedInputPrice    float64 `json:"cached_input_price,omitempty"`   // Cached input price for this tier
	CacheWrite5mPrice   float64 `json:"cache_write_5m_price,omitempty"` // 5-min cache write price for this tier
	CacheWrite1hPrice   float64 `json:"cache_write_1h_price,omitempty"` // 1-hour cache write price for this tier
	InputTokenThreshold int     `json:"input_token_threshold"`          // Minimum input tokens to reach this tier
}

// VideoDisplayPricing represents video generation pricing for display
type VideoDisplayPricing struct {
	PerSecondUsd          float64            `json:"per_second_usd"`                   // USD per rendered second at base resolution
	BaseResolution        string             `json:"base_resolution,omitempty"`        // Base resolution (e.g. "1280x720")
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"` // Resolution -> multiplier map
}

// AudioDisplayPricing represents audio pricing for display
type AudioDisplayPricing struct {
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
func listAllSupportedModels() ([]OpenAIModels, error) {
	models := make([]OpenAIModels, 0, len(allModels))
	seen := make(map[string]struct{}, len(allModels))
	for _, base := range allModels {
		models = append(models, base)
		seen[strings.ToLower(base.Id)] = struct{}{}
	}
	channels, err := model.GetAllEnabledChannels()
	if err != nil {
		return nil, err
	}
	created := int(time.Now().Unix())
	for _, ch := range channels {
		overrides := ch.GetModelPriceConfigs()
		names := mergeModelNamesWithOverrides(ch.GetSupportedModelNames(), overrides)
		if len(names) == 0 {
			continue
		}
		owner := channeltype.IdToName(ch.Type)
		if owner == "" {
			owner = fmt.Sprintf("channel-%d", ch.Id)
		}
		for _, name := range names {
			trimmed := strings.TrimSpace(name)
			if trimmed == "" {
				continue
			}
			lower := strings.ToLower(trimmed)
			if _, exists := seen[lower]; exists {
				continue
			}
			entry := OpenAIModels{
				Id:         trimmed,
				Object:     "model",
				Created:    created,
				OwnedBy:    owner,
				Permission: defaultModelPermissions,
				Root:       trimmed,
				Parent:     nil,
			}
			models = append(models, entry)
			seen[lower] = struct{}{}
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Id < models[j].Id
	})
	return models, nil
}

// GetModelsDisplay returns models available to the current user grouped by channel/adaptor with pricing information
// This endpoint is designed for the Models display page in the frontend
func GetModelsDisplay(c *gin.Context) {
	// If logged-in, filter by user's allowed models; otherwise, show all supported models grouped by channel type
	userId := c.GetInt(ctxkey.Id)
	keyword := strings.ToLower(strings.TrimSpace(c.Query("keyword")))
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

		pricing := adaptor.GetDefaultModelPricing()
		modelMapping := channel.GetModelMapping()
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
			var videoPricing *VideoDisplayPricing
			var audioPricing *AudioDisplayPricing
			var imagePricing *ImageDisplayPricing
			var embeddingPricing *EmbeddingDisplayPricing
			baseCompletionRatio := 0.0
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

			if cfg, ok := pricing[actual]; ok {
				if cfg.Image != nil && cfg.Image.PricePerImageUsd > 0 && cfg.Ratio == 0 && cfg.CachedInputRatio <= 0 {
					info := ModelDisplayInfo{
						MaxTokens:        cfg.MaxTokens,
						ImagePrice:       cfg.Image.PricePerImageUsd,
						InputPrice:       0,
						CachedInputPrice: 0,
						ImagePricing:     buildImageDisplayPricing(cfg.Image, cfg.Image),
					}
					result[modelName] = info
					continue
				}
				inputPrice = convertRatioToPrice(cfg.Ratio)
				cachedInputPrice = inputPrice
				if cfg.CachedInputRatio != 0 {
					cachedInputPrice = convertRatioToPrice(cfg.CachedInputRatio)
					if inputPrice == 0 && cfg.CachedInputRatio > 0 {
						if lg != nil {
							lg.Debug("model display fell back to cached input ratio",
								zap.String("channel", channel.Name),
								zap.String("resolved_model", actual),
								zap.Float64("cached_ratio", cfg.CachedInputRatio))
						}
						inputPrice = cachedInputPrice
					}
				}
				cacheWrite5mPrice = convertRatioToPrice(cfg.CacheWrite5mRatio)
				cacheWrite1hPrice = convertRatioToPrice(cfg.CacheWrite1hRatio)
				baseCompletionRatio = cfg.CompletionRatio
				outputPrice = inputPrice * cfg.CompletionRatio
				maxTokens = cfg.MaxTokens
				if cfg.Image != nil {
					imagePrice = cfg.Image.PricePerImageUsd
					imagePricing = buildImageDisplayPricing(cfg.Image, cfg.Image)
				}
				// Tiered pricing
				if len(cfg.Tiers) > 0 {
					tiers = make([]ModelDisplayTier, 0, len(cfg.Tiers))
					for _, tier := range cfg.Tiers {
						tierInput := convertRatioToPrice(tier.Ratio)
						tierOutput := tierInput * tier.CompletionRatio
						if tier.CompletionRatio == 0 {
							tierOutput = tierInput * baseCompletionRatio
						}
						dt := ModelDisplayTier{
							InputPrice:          tierInput,
							OutputPrice:         tierOutput,
							InputTokenThreshold: tier.InputTokenThreshold,
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
						tiers = append(tiers, dt)
					}
				}
				// Video pricing
				if cfg.Video != nil && cfg.Video.HasData() {
					videoPricing = &VideoDisplayPricing{
						PerSecondUsd:          cfg.Video.PerSecondUsd,
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
				maxTokens = 0
				imagePrice = 0
			}

			applyOverride := func(cfg *model.ModelConfigLocal) {
				if cfg.MaxTokens != 0 {
					maxTokens = cfg.MaxTokens
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

			result[modelName] = ModelDisplayInfo{
				InputPrice:        inputPrice,
				CachedInputPrice:  cachedInputPrice,
				CacheWrite5mPrice: cacheWrite5mPrice,
				CacheWrite1hPrice: cacheWrite1hPrice,
				OutputPrice:       outputPrice,
				MaxTokens:         maxTokens,
				ImagePrice:        imagePrice,
				Tiers:             tiers,
				VideoPricing:      videoPricing,
				AudioPricing:      audioPricing,
				ImagePricing:      imagePricing,
				EmbeddingPricing:  embeddingPricing,
			}
			if inputPrice == 0 && cachedInputPrice == 0 && outputPrice == 0 && imagePrice == 0 && lg != nil {
				lg.Debug("model display missing pricing metadata",
					zap.String("channel", channel.Name),
					zap.String("model", modelName),
					zap.String("resolved_model", actual),
					zap.Bool("override_applied", overrideApplied))
			}
		}
		return result
	}

	// If userId is zero, treat as anonymous: list all channels and their supported models from DB and adaptor
	if userId == 0 {
		// Anonymous path with cache + singleflight to mitigate DB load and thundering herd
		cacheKey := "kw:" + keyword
		if data, ok := anonymousModelsDisplayCache.Load(cacheKey); ok {
			c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: data})
			return
		}

		v, err, _ := anonymousModelsDisplayGroup.Do(cacheKey, func() (any, error) {
			channels, err := model.GetAllEnabledChannels()
			if err != nil {
				return nil, errors.Wrap(err, "get all enabled channels")
			}
			result := make(map[string]ChannelModelsDisplayInfo)
			for _, ch := range channels {
				overrides := ch.GetModelPriceConfigs()
				supported := mergeModelNamesWithOverrides(ch.GetSupportedModelNames(), overrides)
				if len(supported) == 0 {
					continue
				}
				modelInfos := buildChannelModels(ch, supported, overrides)
				if len(modelInfos) == 0 {
					continue
				}
				key := fmt.Sprintf("%s:%s", channeltype.IdToName(ch.Type), ch.Name)
				result[key] = ChannelModelsDisplayInfo{ChannelName: key, ChannelType: ch.Type, Models: modelInfos}
			}
			anonymousModelsDisplayCache.Store(cacheKey, result)
			return result, nil
		})
		if err != nil {
			c.JSON(http.StatusOK, ModelsDisplayResponse{Success: false, Message: "Failed to load channels: " + err.Error()})
			return
		}
		data := v.(map[string]ChannelModelsDisplayInfo)
		c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: data})
		return
	}

	// Logged-in path: show only models allowed for the user group
	ctx := gmw.Ctx(c)
	var userGroup string
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			userGroup = u.Group
		}
	}
	if userGroup == "" {
		var err error
		userGroup, err = model.CacheGetUserGroup(ctx, userId)
		if err != nil {
			c.JSON(http.StatusOK, ModelsDisplayResponse{Success: false, Message: "Failed to get user group: " + err.Error()})
			return
		}
	}
	abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		c.JSON(http.StatusOK, ModelsDisplayResponse{Success: false, Message: "Failed to get available models: " + err.Error()})
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
	for chID, modelSet := range ch2models {
		ch, err := model.GetChannelById(chID, true)
		if err != nil {
			continue
		}
		overrides := ch.GetModelPriceConfigs()
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
		key := fmt.Sprintf("%s:%s", channeltype.IdToName(ch.Type), ch.Name)
		result[key] = ChannelModelsDisplayInfo{ChannelName: key, ChannelType: ch.Type, Models: infos}
	}

	c.JSON(http.StatusOK, ModelsDisplayResponse{Success: true, Message: "", Data: result})
}

// ListModels lists all models available to the user.
func ListModels(c *gin.Context) {
	userId := c.GetInt(ctxkey.Id)
	ctx := gmw.Ctx(c)
	lg := gmw.GetLogger(c)

	var userGroup string
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			userGroup = u.Group
		}
	}
	if userGroup == "" {
		var err error
		userGroup, err = model.CacheGetUserGroup(ctx, userId)
		if err != nil {
			middleware.AbortWithError(c, http.StatusBadRequest, err)
			return
		}
	}

	availableAbilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}

	snapshot, err := getSupportedModelsSnapshot()
	if err != nil {
		middleware.AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "load supported models snapshot"))
		return
	}

	snapshotByID := make(map[string]OpenAIModels, len(snapshot))
	for _, model := range snapshot {
		key := strings.ToLower(model.Id)
		snapshotByID[key] = model
	}

	allowed := make(map[string]OpenAIModels, len(availableAbilities))
	channelCache := make(map[int]*model.Channel)
	created := int(time.Now().Unix())

	for _, ability := range availableAbilities {
		modelName := strings.TrimSpace(ability.Model)
		if modelName == "" {
			continue
		}
		key := strings.ToLower(modelName)
		if entry, ok := snapshotByID[key]; ok {
			allowed[key] = entry
			continue
		}

		entry, ok := buildModelEntryFromAbility(modelName, ability.ChannelId, ability.ChannelType, created, channelCache)
		if ok {
			allowed[key] = entry
			continue
		}
		if lg != nil {
			lg.Debug("unable to build model entry for ability",
				zap.String("model", modelName),
				zap.Int("channel_id", ability.ChannelId),
				zap.Int("channel_type", ability.ChannelType))
		}
	}

	userAvailableModels := make([]OpenAIModels, 0, len(allowed))
	for _, model := range allowed {
		userAvailableModels = append(userAvailableModels, model)
	}

	sort.Slice(userAvailableModels, func(i, j int) bool {
		return userAvailableModels[i].Id < userAvailableModels[j].Id
	})

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   userAvailableModels,
	})
}

func buildModelEntryFromAbility(modelName string, channelID int, channelType int, created int, cache map[int]*model.Channel) (OpenAIModels, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return OpenAIModels{}, false
	}

	owner := channeltype.IdToName(channelType)
	if channelID > 0 {
		if channel, ok := cache[channelID]; ok {
			owner = channeltype.IdToName(channel.Type)
			if owner == "" || owner == "unknown" {
				owner = fmt.Sprintf("channel-%d", channel.Id)
			}
		} else {
			channel, err := model.GetChannelById(channelID, false)
			if err == nil {
				cache[channelID] = channel
				owner = channeltype.IdToName(channel.Type)
				if owner == "" || owner == "unknown" {
					owner = fmt.Sprintf("channel-%d", channel.Id)
				}
			} else if owner == "" {
				owner = fmt.Sprintf("channel-%d", channelID)
			}
		}
	}
	if owner == "" {
		owner = "unknown"
	}

	return OpenAIModels{
		Id:         modelName,
		Object:     "model",
		Created:    created,
		OwnedBy:    owner,
		Permission: defaultModelPermissions,
		Root:       modelName,
		Parent:     nil,
	}, true
}

// RetrieveModel returns details about a specific model or an error when it does not exist.
func RetrieveModel(c *gin.Context) {
	modelId := c.Param("model")
	if model, ok := modelsMap[modelId]; ok {
		c.JSON(http.StatusOK, model)
		return
	}
	lg := gmw.GetLogger(c)
	if snapshot, err := listAllSupportedModels(); err == nil {
		for _, m := range snapshot {
			if strings.EqualFold(m.Id, modelId) {
				c.JSON(http.StatusOK, m)
				return
			}
		}
	} else if lg != nil {
		lg.Debug("failed to build supported models snapshot for lookup", zap.Error(err))
	}
	msg := fmt.Sprintf("The model '%s' does not exist", modelId)
	Error := relaymodel.Error{Message: msg, Type: relaymodel.ErrorTypeInvalidRequest, Param: "model", Code: "model_not_found", RawError: errors.New(msg)}
	c.JSON(http.StatusOK, gin.H{
		"error": Error,
	})
}

// GetUserAvailableModels lists the model identifiers the authenticated user can access.
func GetUserAvailableModels(c *gin.Context) {
	ctx := gmw.Ctx(c)
	id := c.GetInt(ctxkey.Id)
	var userGroup string
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if u, ok := userObj.(*model.User); ok {
			userGroup = u.Group
		}
	}
	if userGroup == "" {
		var err error
		userGroup, err = model.CacheGetUserGroup(ctx, id)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}

	models, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var modelNames []string
	modelsMap := map[string]bool{}
	for _, model := range models {
		modelsMap[model.Model] = true
	}
	for modelName := range modelsMap {
		modelNames = append(modelNames, modelName)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    modelNames,
	})
}

// GetAvailableModelsByToken reports the models allowed for the current API token when explicitly restricted.
func GetAvailableModelsByToken(c *gin.Context) {
	// Get token information to determine status
	tokenID := c.GetInt(ctxkey.TokenId)
	userID := c.GetInt(ctxkey.Id)
	token, err := model.GetTokenByIds(tokenID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": err.Error(),
			"data": gin.H{
				"available": nil,
				"enabled":   false,
			},
		})
		return
	}

	// Determine if token is enabled
	statusToken := token.Status == model.TokenStatusEnabled

	// Check if the token has specific model restrictions
	if availableModels, exists := c.Get(ctxkey.AvailableModels); exists {
		// Token has model restrictions, use those models
		modelsString := availableModels.(string)
		if modelsString != "" {
			modelNames := strings.Split(modelsString, ",")
			// Trim whitespace from each model name
			for i := range modelNames {
				modelNames[i] = strings.TrimSpace(modelNames[i])
			}
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data": gin.H{
					"available": modelNames,
					"enabled":   statusToken,
				},
			})
			return
		}
	}

	// Token has no model restrictions, return error instead of fallback
	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "the token has no available models",
		"data": gin.H{
			"available": nil,
			"enabled":   statusToken,
		},
	})
}
