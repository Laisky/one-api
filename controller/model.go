package controller

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	gutils "github.com/Laisky/go-utils/v6"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/dto"
	"github.com/Laisky/one-api/middleware"
	"github.com/Laisky/one-api/model"
	relay "github.com/Laisky/one-api/relay"
	adaptorpkg "github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/channeltype"
	"github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	relaypricing "github.com/Laisky/one-api/relay/pricing"
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
	// modelCatalogCreated is the frozen `created` timestamp every catalog entry
	// carries. It must stay a constant: the supported-models snapshot is rebuilt
	// whenever the enabled-channel signature changes, and because a billing write
	// bumps channels.updated_at that happens on essentially every relayed request.
	// A time.Now() here would make `created` churn on every rebuild.
	modelCatalogCreated = 1626777600

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
		Created:            modelCatalogCreated,
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
		// apitype.Azure's adaptor advertises the entire OpenAI catalog plus the
		// Foundry Claude models, which are already emitted under apitype.OpenAI and
		// apitype.Anthropic respectively. Skip it here to avoid duplicate rows (and
		// the "openai"-owner mislabel from a non-Init'd adaptor). The Azure channel
		// still lists both families via channelId2Models below and the DB-channel
		// pass in listAllSupportedModels. Mirrors the channeltype.Azure skips in the
		// CompatibleChannels loop below and in openrouter_provider.go.
		if i == apitype.Azure {
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
				Created:    modelCatalogCreated,
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
				Created:    modelCatalogCreated,
				OwnedBy:    channelName,
				Permission: permission,
				Root:       modelName,
				Parent:     nil,
			})
		}
	}
	allModels = dedupeStaticModelsByOwner(allModels)
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

// dedupeStaticModelsByOwner collapses the aggregated static catalog to exactly
// one row per model id.
//
// Many providers legitimately serve the same model id -- open-weight ids such as
// Qwen/Qwen3.5-9B are hosted by several upstreams at once, Claude ids are served
// by both the Anthropic and AWS adaptors, and the Zhipu (open.bigmodel.cn) and
// Zai (api.z.ai) channels are two brands of the same GLM catalog. The aggregation
// loops above append unconditionally, so without this pass /v1/models emits a
// duplicate row per extra provider, and the modelsMap built from it resolves the
// owner by whichever adaptor happened to be swept last.
//
// This is the FALLBACK ranking, used only where no channel is available to ask:
// allModels is built at init() from the compiled-in adaptors, long before the
// database is readable. Wherever channels exist the owner comes from them instead
// -- listAllSupportedModels ranks enabled channels ahead of this catalog, and
// /v1/models resolves the owner from the ability's own channel (bestAbilityPerModel).
// Here the winner is simply the smallest OwnedBy in byte order, with the original
// position as a stable tie-break, so the catalog is at least deterministic rather
// than dependent on the apitype iota or map iteration order.
//
// Billing is unaffected either way: quota resolves per request through the
// channel's own apitype and price table, so glm-4.7 on a Zhipu channel bills at
// BigModel's CNY tiers while the same id on a Zai channel bills at Z.AI's flat USD
// rate, regardless of which one owns the row in any listing.
func dedupeStaticModelsByOwner(models []OpenAIModels) []OpenAIModels {
	winners := make(map[string]int, len(models))
	for i, m := range models {
		prev, seen := winners[m.Id]
		if !seen || m.OwnedBy < models[prev].OwnedBy {
			winners[m.Id] = i
		}
	}
	deduped := make([]OpenAIModels, 0, len(winners))
	for i, m := range models {
		if winners[m.Id] == i {
			deduped = append(deduped, m)
		}
	}
	return deduped
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
	models, err := getSupportedModelsSnapshotWithContext(gmw.Ctx(c))
	if err != nil {
		middleware.AbortWithError(c, http.StatusInternalServerError, errors.Wrap(err, "load supported models"))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

// getSupportedModelsSnapshotWithContext returns the cached supported-model
// snapshot, rebuilding it with request-correlated diagnostics when stale.
// Parameters: ctx carries cancellation and logging values for snapshot loading.
// Returns: the supported models or a wrapped loading error.
func getSupportedModelsSnapshotWithContext(ctx context.Context) ([]OpenAIModels, error) {
	version, err := model.GetEnabledChannelsVersionSignature()
	if err != nil {
		return nil, errors.Wrap(err, "channels version signature")
	}

	if entry, ok := cachedListAllModels.Get(); ok && entry.Version == version {
		return entry.Models, nil
	}

	models, err := listAllSupportedModels(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "list models")
	}

	cachedListAllModels.Set(listAllModelsCacheEntry{
		Models:  models,
		Version: version,
	})

	return models, nil
}

// getRequestUserGroup returns the request context and resolved user group for authenticated model endpoints.
func getRequestUserGroup(c *gin.Context) (context.Context, string, error) {
	ctx := gmw.Ctx(c)
	if userObj, exists := c.Get(ctxkey.UserObj); exists {
		if user, ok := userObj.(*model.User); ok {
			group := strings.TrimSpace(user.Group)
			if group != "" {
				return ctx, group, nil
			}
		}
	}
	group, err := model.CacheGetUserGroup(ctx, c.GetInt(ctxkey.Id))
	if err != nil {
		return ctx, "", errors.Wrap(err, "cache get user group")
	}
	return ctx, group, nil
}

// loadChannelCached loads a channel once and reuses it from the provided cache.
//
// Failures are memoized as a nil entry. Channel.Delete is not transactional, so
// orphaned ability rows are a real state; without negative memoization one
// deleted channel costs a failed query per ability it used to back, on every
// request that lists models.
func loadChannelCached(channelID int, cache map[int]*model.Channel) (*model.Channel, error) {
	if channelID == 0 {
		return nil, errors.New("channel id is required")
	}
	if channel, ok := cache[channelID]; ok {
		if channel == nil {
			return nil, errors.Errorf("channel %d is not resolvable", channelID)
		}
		return channel, nil
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
		cache[channelID] = nil
		return nil, errors.Wrapf(err, "load channel %d", channelID)
	}
	cache[channelID] = channel
	return channel, nil
}

// isVisibleAbilityModel reports whether the ability's model remains publicly visible after hidden-model filtering.
func isVisibleAbilityModel(ability dto.EnabledAbility, cache map[int]*model.Channel) bool {
	modelName := strings.TrimSpace(ability.Model)
	if modelName == "" {
		return false
	}
	channel, err := loadChannelCached(ability.ChannelId, cache)
	if err != nil {
		return false
	}
	return !channel.IsModelHidden(modelName)
}

// filterVisibleAbilities removes stale or hidden ability rows from public model responses.
func filterVisibleAbilities(abilities []dto.EnabledAbility, cache map[int]*model.Channel) []dto.EnabledAbility {
	visible := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		if !isVisibleAbilityModel(ability, cache) {
			continue
		}
		visible = append(visible, ability)
	}
	return visible
}

// filterAbilitiesByTokenAllowList narrows abilities to what the calling API token
// is permitted to invoke.
//
// Group abilities answer "what can this deployment serve for this user", which is
// a coarser question than "what may this key call". TokenAuth enforces the token's
// own allow-list on every request and 403s anything outside it, so a listing that
// skipped this filter advertised models the caller would then be refused.
//
// The membership test is middleware.IsModelInList -- the very predicate that
// produces the 403 -- so discovery and invocability cannot disagree. The TOKEN
// entry is matched raw (that helper does not trim), so an allow-list stored as
// "a, b" genuinely does not permit "b" and the listing reflects that rather than
// hiding it. The ABILITY name is trimmed because the trimmed form is the id
// actually advertised, and therefore the string a client sends back; see
// bestAbilityPerModel, which trims identically.
//
// An unrestricted token has no ctxkey.AvailableModels entry at all (TokenAuth sets
// it only for a non-empty Token.Models), so the absent case must return everything
// -- returning an empty list there would blank the catalog for most callers.
func filterAbilitiesByTokenAllowList(c *gin.Context, abilities []dto.EnabledAbility) []dto.EnabledAbility {
	raw, restricted := c.Get(ctxkey.AvailableModels)
	if !restricted {
		return abilities
	}
	allowList, ok := raw.(string)
	if !ok {
		// Only TokenAuth writes this key, and always as a string. Reaching here
		// means something else clobbered it; the relay would still enforce the
		// allow-list, so failing open would advertise models that then 403.
		gmw.GetLogger(c).Warn("ctxkey.AvailableModels is not a string; token allow-list filter skipped")
		return abilities
	}
	if allowList == "" {
		return abilities
	}

	permitted := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		if middleware.IsModelInList(strings.TrimSpace(ability.Model), allowList) {
			permitted = append(permitted, ability)
		}
	}
	return permitted
}

// respondModelNotFound returns the OpenAI-compatible model-not-found error payload.
//
// HTTP 404, matching OpenAI: SDKs key their NotFoundError on the status, so a 200
// carrying an error body is parsed as a successful model descriptor and surfaces
// as a confusing decode failure rather than a clean "model not found".
//
// The same payload is returned whether the model does not exist, is hidden on its
// channel, or is outside the calling key's allow-list. That is deliberate -- three
// distinguishable responses would let any key enumerate the deployment's catalog --
// which is also why the message follows OpenAI in saying "or you do not have access
// to it" rather than asserting the model does not exist.
func respondModelNotFound(c *gin.Context, modelID string) {
	msg := fmt.Sprintf("The model '%s' does not exist or you do not have access to it", modelID)
	respErr := relaymodel.Error{Message: msg, Type: relaymodel.ErrorTypeInvalidRequest, Param: "model", Code: "model_not_found", RawError: errors.New(msg)}
	c.JSON(http.StatusNotFound, gin.H{
		"error": respErr,
	})
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

// PerCallDisplayPricing represents flat per-invocation pricing for display.
// Providers commonly price by query ("$X per 1K calls"); rerank is the canonical
// example. Display surfaces both per-1K-calls and the derived per-call USD figure.
type PerCallDisplayPricing struct {
	UsdPerThousandCalls float64 `json:"usd_per_thousand_calls,omitempty"` // USD per 1000 invocations
	UsdPerCall          float64 `json:"usd_per_call,omitempty"`           // Derived USD per single invocation
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
	if cfg.Video != nil {
		converted.Video = &adaptorpkg.VideoPricingConfig{
			PerSecondUsd:          cfg.Video.PerSecondUsd,
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

// modelDisplayFilters describes the optional filter parameters accepted by /api/models/display.
// All fields are derived from query string and applied AFTER pricing info is collected per model.
type modelDisplayFilters struct {
	inputModalities   []string // any-match against ModelConfig.InputModalities (empty = no filter)
	outputModalities  []string // any-match against ModelConfig.OutputModalities (empty = no filter)
	features          []string // all-match against ModelConfig.SupportedFeatures (empty = no filter)
	reasoningEfforts  []string // any-match against ModelConfig.SupportedReasoningEfforts (empty = no filter)
	channelTypes      []int    // restrict to specific channel type ids (empty = no filter)
	minContextLength  int32    // require ContextLength >= this (0 = no filter)
	maxInputPriceUsd  float64  // require InputPrice <= this (per 1M tokens, 0 = no filter)
	requireImage      bool     // require image pricing or image output
	requireVideo      bool     // require video pricing or video output
	requireAudio      bool     // require audio pricing or audio output
	requireEmbedding  bool     // require embedding pricing
	requireReasoning  bool     // require reasoning feature (any of supported_features contains "reasoning")
	requireTools      bool     // require tools feature
	requireWebSearch  bool     // require web_search feature
	requireStructured bool     // require structured_outputs feature
}

// hasAny returns true when haystack contains any of the needles (case-insensitive).
func hasAny(haystack []string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; ok {
			return true
		}
	}
	return false
}

// hasAll returns true when haystack contains every needle (case-insensitive).
func hasAll(haystack []string, needles []string) bool {
	if len(needles) == 0 {
		return true
	}
	if len(haystack) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(haystack))
	for _, h := range haystack {
		set[strings.ToLower(strings.TrimSpace(h))] = struct{}{}
	}
	for _, n := range needles {
		if _, ok := set[strings.ToLower(strings.TrimSpace(n))]; !ok {
			return false
		}
	}
	return true
}

// parseCSVQuery returns trimmed, lower-cased, non-empty tokens from a query parameter.
// Supports comma-separated single values and repeated query parameters.
func parseCSVQuery(c *gin.Context, key string) []string {
	values := c.QueryArray(key)
	if v := c.Query(key); v != "" && len(values) == 0 {
		values = []string{v}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			t := strings.ToLower(strings.TrimSpace(part))
			if t == "" {
				continue
			}
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

func parseIntCSV(c *gin.Context, key string) []int {
	raw := parseCSVQuery(c, key)
	out := make([]int, 0, len(raw))
	for _, v := range raw {
		if i, err := strconv.Atoi(v); err == nil {
			out = append(out, i)
		}
	}
	return out
}

func parseBoolQuery(c *gin.Context, key string) bool {
	v := strings.ToLower(strings.TrimSpace(c.Query(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func parseInt32Query(c *gin.Context, key string) int32 {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

func parseFloatQuery(c *gin.Context, key string) float64 {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return 0
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return 0
	}
	return f
}

// parseModelDisplayFilters extracts every supported filter query parameter.
func parseModelDisplayFilters(c *gin.Context) modelDisplayFilters {
	return modelDisplayFilters{
		inputModalities:   parseCSVQuery(c, "input_modality"),
		outputModalities:  parseCSVQuery(c, "output_modality"),
		features:          parseCSVQuery(c, "feature"),
		reasoningEfforts:  parseCSVQuery(c, "reasoning_effort"),
		channelTypes:      parseIntCSV(c, "channel_type"),
		minContextLength:  parseInt32Query(c, "min_context_length"),
		maxInputPriceUsd:  parseFloatQuery(c, "max_input_price"),
		requireImage:      parseBoolQuery(c, "has_image"),
		requireVideo:      parseBoolQuery(c, "has_video"),
		requireAudio:      parseBoolQuery(c, "has_audio"),
		requireEmbedding:  parseBoolQuery(c, "has_embedding"),
		requireReasoning:  parseBoolQuery(c, "has_reasoning"),
		requireTools:      parseBoolQuery(c, "has_tools"),
		requireWebSearch:  parseBoolQuery(c, "has_web_search"),
		requireStructured: parseBoolQuery(c, "has_structured_outputs"),
	}
}

// hasContent reports whether any filter parameter is active.
func (f modelDisplayFilters) hasContent() bool {
	return len(f.inputModalities) > 0 || len(f.outputModalities) > 0 || len(f.features) > 0 ||
		len(f.reasoningEfforts) > 0 || len(f.channelTypes) > 0 || f.minContextLength > 0 ||
		f.maxInputPriceUsd > 0 || f.requireImage || f.requireVideo || f.requireAudio ||
		f.requireEmbedding || f.requireReasoning || f.requireTools || f.requireWebSearch || f.requireStructured
}

// matchesChannel reports whether the channel type is allowed by the filter (or no filter set).
func (f modelDisplayFilters) matchesChannel(channelType int) bool {
	if len(f.channelTypes) == 0 {
		return true
	}
	for _, t := range f.channelTypes {
		if t == channelType {
			return true
		}
	}
	return false
}

// matchesModel evaluates the per-model portion of the filter against the assembled ModelDisplayInfo.
// Empty filter fields are treated as "no constraint".
func (f modelDisplayFilters) matchesModel(info ModelDisplayInfo) bool {
	if len(f.inputModalities) > 0 && !hasAny(info.InputModalities, f.inputModalities) {
		return false
	}
	if len(f.outputModalities) > 0 && !hasAny(info.OutputModalities, f.outputModalities) {
		return false
	}
	if len(f.features) > 0 && !hasAll(info.SupportedFeatures, f.features) {
		return false
	}
	if len(f.reasoningEfforts) > 0 && !hasAny(info.SupportedReasoningEfforts, f.reasoningEfforts) {
		return false
	}
	if f.minContextLength > 0 && info.ContextLength < f.minContextLength {
		return false
	}
	if f.maxInputPriceUsd > 0 && info.InputPrice > f.maxInputPriceUsd {
		return false
	}
	if f.requireImage && info.ImagePricing == nil && !hasAny(info.OutputModalities, []string{"image"}) {
		return false
	}
	if f.requireVideo && info.VideoPricing == nil && !hasAny(info.OutputModalities, []string{"video"}) {
		return false
	}
	if f.requireAudio && info.AudioPricing == nil && !hasAny(info.OutputModalities, []string{"audio"}) && !hasAny(info.InputModalities, []string{"audio"}) {
		return false
	}
	if f.requireEmbedding && info.EmbeddingPricing == nil {
		return false
	}
	if f.requireReasoning && !hasAny(info.SupportedFeatures, []string{"reasoning"}) {
		return false
	}
	if f.requireTools && !hasAny(info.SupportedFeatures, []string{"tools"}) {
		return false
	}
	if f.requireWebSearch && !hasAny(info.SupportedFeatures, []string{"web_search"}) {
		return false
	}
	if f.requireStructured && !hasAny(info.SupportedFeatures, []string{"structured_outputs"}) {
		return false
	}
	return true
}

// GetModelsDisplay returns models available to the current user grouped by channel/adaptor with pricing information
// This endpoint is designed for the Models display page in the frontend
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
			var audioPricing *AudioDisplayPricing
			var imagePricing *ImageDisplayPricing
			var embeddingPricing *EmbeddingDisplayPricing
			var timeWindows []TimeWindowDisplay
			var activeTimeWindow string
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

			if cfg, ok := defaultPricing[actual]; ok {
				timeWindows, activeTimeWindow = buildTimeWindowDisplays(cfg.TimeWindows, convertRatioToPrice(cfg.Ratio), cfg.CompletionRatio, displayNow, convertRatioToPrice)
				if cfg.Image != nil && cfg.Image.PricePerImageUsd > 0 && cfg.Ratio == 0 && cfg.CachedInputRatio <= 0 {
					info := ModelDisplayInfo{
						MaxTokens:                 cfg.MaxTokens,
						ContextLength:             cfg.ContextLength,
						MaxOutputTokens:           cfg.MaxOutputTokens,
						MaxReasoningTokens:        cfg.MaxReasoningTokens,
						InputModalities:           append([]string(nil), cfg.InputModalities...),
						OutputModalities:          append([]string(nil), cfg.OutputModalities...),
						SupportedFeatures:         append([]string(nil), cfg.SupportedFeatures...),
						SupportedSampling:         append([]string(nil), cfg.SupportedSamplingParameters...),
						SupportedReasoningEfforts: append([]string(nil), cfg.SupportedReasoningEfforts...),
						DefaultReasoningEffort:    cfg.DefaultReasoningEffort,
						Quantization:              cfg.Quantization,
						HuggingFaceID:             cfg.HuggingFaceID,
						Description:               cfg.Description,
						ImagePrice:                cfg.Image.PricePerImageUsd,
						InputPrice:                0,
						CachedInputPrice:          0,
						ImagePricing:              buildImageDisplayPricing(cfg.Image, cfg.Image),
						TimeWindows:               timeWindows,
						ActiveTimeWindow:          activeTimeWindow,
					}
					if filters.matchesModel(info) {
						result[modelName] = info
					}
					continue
				}
				if cfg.PerCall != nil && cfg.PerCall.HasData() {
					info := ModelDisplayInfo{
						MaxTokens:                 cfg.MaxTokens,
						ContextLength:             cfg.ContextLength,
						MaxOutputTokens:           cfg.MaxOutputTokens,
						MaxReasoningTokens:        cfg.MaxReasoningTokens,
						InputModalities:           append([]string(nil), cfg.InputModalities...),
						OutputModalities:          append([]string(nil), cfg.OutputModalities...),
						SupportedFeatures:         append([]string(nil), cfg.SupportedFeatures...),
						SupportedSampling:         append([]string(nil), cfg.SupportedSamplingParameters...),
						SupportedReasoningEfforts: append([]string(nil), cfg.SupportedReasoningEfforts...),
						DefaultReasoningEffort:    cfg.DefaultReasoningEffort,
						Quantization:              cfg.Quantization,
						HuggingFaceID:             cfg.HuggingFaceID,
						Description:               cfg.Description,
						InputPrice:                0,
						CachedInputPrice:          0,
						OutputPrice:               0,
						PerCallPricing: &PerCallDisplayPricing{
							UsdPerThousandCalls: cfg.PerCall.UsdPerThousandCalls,
							UsdPerCall:          cfg.PerCall.UsdPerThousandCalls / 1000.0,
						},
						TimeWindows:      timeWindows,
						ActiveTimeWindow: activeTimeWindow,
					}
					if filters.matchesModel(info) {
						result[modelName] = info
					}
					continue
				}
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
							InputPrice:           tierInput,
							OutputPrice:          tierOutput,
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
				timeWindows, activeTimeWindow = buildTimeWindowDisplays(convertLocalDisplayTimeWindows(cfg.TimeWindows), inputPrice, baseCompletionRatio, displayNow, convertRatioToPrice)
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
				key := fmt.Sprintf("%s:%s", channeltype.IdToName(ch.Type), ch.Name)
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
				return nil, err
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
	for chID, modelSet := range ch2models {
		ch, err := model.GetChannelById(chID, true)
		if err != nil {
			continue
		}
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
	channelCache := make(map[int]*model.Channel)
	availableAbilities = filterVisibleAbilities(availableAbilities, channelCache)
	availableAbilities = filterAbilitiesByTokenAllowList(c, availableAbilities)

	respondModelList(c, resolveUserAvailableModels(availableAbilities, channelCache, lg))
}

// abilityChannelRef resolves the log identity of the channel backing an ability.
//
// It reads only the caller-supplied channel cache, never the database, because it
// runs inside the /v1/models listing loop. A cache miss degrades to an id-only
// reference so an operator's `channel_id=` grep still matches.
//
// Parameters:
//   - channelCache: channel snapshot keyed by channel id; may be nil.
//   - channelID: the ability's channel primary key.
//
// Return values:
//   - identity.ChannelRef: fullest reference resolvable without I/O.
func abilityChannelRef(channelCache map[int]*model.Channel, channelID int) identity.ChannelRef {
	if ch := channelCache[channelID]; ch != nil {
		return ch.Ref()
	}
	return identity.NewChannelRef(channelID, "", "")
}

// bestAbilityPerModel collapses the one-row-per-(model, channel) ability list to a
// single representative channel per exact model name, ranked the way routing
// ranks channels: highest priority first, lowest channel id as the tie-break.
//
// This is what makes a listing's owned_by honest. The same model id is routinely
// served by several channels -- open-weight ids hosted by many upstreams, and
// two-brand providers such as Zhipu (open.bigmodel.cn) and Z.ai (api.z.ai) whose
// catalogs are near-identical -- so without an explicit rank the attribution
// would fall out of row order or map iteration and move between restarts.
//
// Keyed by the exact model name because routing is case-sensitive: two abilities
// differing only in case are distinct routing keys (see issue #352).
func bestAbilityPerModel(abilities []dto.EnabledAbility) map[string]dto.EnabledAbility {
	best := make(map[string]dto.EnabledAbility, len(abilities))
	for _, ability := range abilities {
		modelName := strings.TrimSpace(ability.Model)
		if modelName == "" {
			continue
		}
		if current, ok := best[modelName]; ok && !ability.Beats(current) {
			continue
		}
		best[modelName] = ability
	}
	return best
}

// resolveUserAvailableModels converts a user group's enabled abilities into
// OpenAI-shaped model entries.
//
// Every field is derived from the abilities and their channels; the compiled-in
// adaptor catalog is deliberately not consulted. That catalog describes what this
// binary knows how to talk to, not what this deployment can serve, so borrowing
// from it produced two defects: a gateway running only a Zhipu channel reported
// glm-4.7 as owned by Z.ai (both adaptors ship the id), and an id whose catalog
// entry used a different casing could be advertised in a form channel routing
// cannot match (issue #352). Building from the ability makes the first impossible
// to get wrong and the second structurally impossible: Id/Root are the ability's
// own case-sensitive routing key by construction.
//
// The three remaining fields are constants or channel-derived: owned_by comes
// from the channel chosen by bestAbilityPerModel, created is the frozen
// modelCatalogCreated, and permission is defaultModelPermissions.
//
// Entries are keyed by the exact ability model name: two abilities that differ
// only in case are distinct routing keys and must both remain listable, while
// identical names collapse to a single entry.
func resolveUserAvailableModels(abilities []dto.EnabledAbility, channelCache map[int]*model.Channel, lg glog.Logger) []OpenAIModels {
	allowed := make(map[string]OpenAIModels, len(abilities))
	for modelName, ability := range bestAbilityPerModel(abilities) {
		if entry, ok := buildModelEntryFromAbility(modelName, ability.ChannelId, ability.ChannelType, channelCache); ok {
			allowed[modelName] = entry
			continue
		}
		if lg != nil {
			// The ability's channel is not the request's own channel, so name it
			// explicitly; the cache is already in hand, so this costs no query.
			lg.Debug("unable to build model entry for ability",
				abilityChannelRef(channelCache, ability.ChannelId).AppendZap([]zap.Field{
					zap.String("model", modelName),
					zap.Int("channel_type", ability.ChannelType),
				})...)
		}
	}

	userAvailableModels := make([]OpenAIModels, 0, len(allowed))
	for _, entry := range allowed {
		userAvailableModels = append(userAvailableModels, entry)
	}

	sort.Slice(userAvailableModels, func(i, j int) bool {
		return userAvailableModels[i].Id < userAvailableModels[j].Id
	})

	return userAvailableModels
}

// abilityOwnerFromCache resolves the owned_by label for the channel backing an
// ability WITHOUT any database access, so it is safe inside the /v1/models
// listing loop (same constraint as abilityChannelRef).
//
// The cached channel's own type wins when available; otherwise the ability's
// channel_type is authoritative on its own, because GetGroupModelsV2 reads it by
// joining channels. An unnamed or unknown type degrades to "channel-<uuid>" when
// the cached channel carries its external UUID, and to "unknown" otherwise; the
// internal integer id never reaches the public catalog and this path stays
// I/O-free.
func abilityOwnerFromCache(channelID int, channelType int, cache map[int]*model.Channel) string {
	owner := channeltype.IdToName(channelType)
	if channelID > 0 {
		if channel, ok := cache[channelID]; ok && channel != nil {
			owner = channeltype.IdToName(channel.Type)
			if owner == "" || owner == "unknown" {
				owner = channelUUIDOwner(channel.UUID)
			}
		}
	}
	if owner == "" || owner == "unknown" {
		return "unknown"
	}
	return owner
}

// channelUUIDOwner builds the owned_by fallback label for a channel whose type
// has no display name.
// Parameters:
//   - channelUUID: the channel's external UUID; may be empty when not backfilled.
//
// Return values:
//   - string: "channel-<uuid>" or "unknown" when the UUID is empty.
func channelUUIDOwner(channelUUID string) string {
	channelUUID = strings.TrimSpace(channelUUID)
	if channelUUID == "" {
		return "unknown"
	}
	return "channel-" + channelUUID
}

// buildModelEntryFromAbility renders one ability as an OpenAI model entry.
//
// It performs no I/O: callers run it once per advertised model, and every caller
// has already populated the channel cache via filterVisibleAbilities.
func buildModelEntryFromAbility(modelName string, channelID int, channelType int, cache map[int]*model.Channel) (OpenAIModels, bool) {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return OpenAIModels{}, false
	}

	owner := abilityOwnerFromCache(channelID, channelType, cache)

	return OpenAIModels{
		Id:         modelName,
		Object:     "model",
		Created:    modelCatalogCreated,
		OwnedBy:    owner,
		Permission: defaultModelPermissions,
		Root:       modelName,
		Parent:     nil,
	}, true
}

// matchVisibleAbilityByModelID selects an ability for a requested model ID. The
// exact trimmed routing ID wins; otherwise, the lexicographically smallest
// case-folded match is returned. Within one model name the channel is chosen the
// way routing chooses it -- highest priority, then lowest channel id -- so the
// metadata reported for a model served by several channels matches what
// /v1/models reports for the same model.
func matchVisibleAbilityByModelID(abilities []dto.EnabledAbility, modelID string) (dto.EnabledAbility, bool) {
	modelID = strings.TrimSpace(modelID)
	candidates := make([]dto.EnabledAbility, 0, len(abilities))
	for _, ability := range abilities {
		ability.Model = strings.TrimSpace(ability.Model)
		if ability.Model == "" || !strings.EqualFold(ability.Model, modelID) {
			continue
		}
		candidates = append(candidates, ability)
	}
	if len(candidates) == 0 {
		return dto.EnabledAbility{}, false
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Model != candidates[j].Model {
			return candidates[i].Model < candidates[j].Model
		}
		if candidates[i].Priority != candidates[j].Priority {
			return candidates[i].Priority > candidates[j].Priority
		}
		if candidates[i].ChannelId != candidates[j].ChannelId {
			return candidates[i].ChannelId < candidates[j].ChannelId
		}
		return candidates[i].ChannelType < candidates[j].ChannelType
	})
	for _, candidate := range candidates {
		if candidate.Model == modelID {
			return candidate, true
		}
	}
	return candidates[0], true
}

// RetrieveModel returns details about a specific model or an error when it does not exist.
func RetrieveModel(c *gin.Context) {
	// Scoped to the caller's group and key, so it must not be shared by a cache.
	setPrivateCatalogHeaders(c)
	modelId := strings.TrimSpace(c.Param("model"))
	ctx, userGroup, err := getRequestUserGroup(c)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}
	abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		middleware.AbortWithError(c, http.StatusBadRequest, err)
		return
	}
	channelCache := make(map[int]*model.Channel)
	visibleAbilities := filterVisibleAbilities(abilities, channelCache)
	// A model outside the token's allow-list is reported as not found rather than
	// described: the caller cannot invoke it, so advertising its metadata would
	// both mislead and disclose part of the group catalog the key has no access to.
	visibleAbilities = filterAbilitiesByTokenAllowList(c, visibleAbilities)
	matched, ok := matchVisibleAbilityByModelID(visibleAbilities, modelId)
	if !ok {
		respondModelNotFound(c, modelId)
		return
	}
	modelId = matched.Model

	// modelId was rebound above to the ability's actual casing, the case-sensitive
	// routing key /v1/chat/completions matches (issue #352). The entry is built
	// from that ability rather than looked up in the compiled-in catalog, so this
	// endpoint reports exactly what GET /v1/models reports for the same model --
	// same id, same owner, same created -- and cannot drift from it.
	if entry, ok := buildModelEntryFromAbility(modelId, matched.ChannelId, matched.ChannelType, channelCache); ok {
		c.JSON(http.StatusOK, entry)
		return
	}
	respondModelNotFound(c, modelId)
}

// GetUserAvailableModels lists the model identifiers the authenticated user can access.
func GetUserAvailableModels(c *gin.Context) {
	ctx, userGroup, err := getRequestUserGroup(c)
	if err != nil {
		helper.RespondError(c, err)
		return
	}

	models, err := model.CacheGetGroupModelsV2(ctx, userGroup)
	if err != nil {
		helper.RespondError(c, err)
		return
	}
	channelCache := make(map[int]*model.Channel)
	models = filterVisibleAbilities(models, channelCache)

	modelNames := make([]string, 0)
	modelsMap := map[string]bool{}
	for _, model := range models {
		modelsMap[model.Model] = true
	}
	for modelName := range modelsMap {
		modelNames = append(modelNames, modelName)
	}
	sort.Strings(modelNames)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    modelNames,
	})
}

// intersectTokenModelIDs intersects a token's configured model IDs with visible
// abilities using exact trimmed routing IDs. It returns each canonical ability ID
// once, preserving the token configuration order.
func intersectTokenModelIDs(modelsString string, abilities []dto.EnabledAbility) []string {
	routingIDs := make(map[string]string, len(abilities))
	for _, ability := range abilities {
		modelName := strings.TrimSpace(ability.Model)
		if modelName == "" {
			continue
		}
		routingIDs[modelName] = modelName
	}

	// Membership is delegated to middleware.IsModelInList -- the same call the relay
	// makes before it 403s -- so this endpoint cannot advertise a model the caller
	// would be refused. It used to TrimSpace each entry, which made an allow-list
	// stored as "a, b" advertise "b" while the relay refused it.
	//
	// Iteration stays over the CSV so the caller's own ordering is preserved.
	tokenModels := strings.Split(modelsString, ",")
	modelNames := make([]string, 0, len(tokenModels))
	seen := make(map[string]struct{}, len(tokenModels))
	for _, rawModel := range tokenModels {
		canonicalModel, ok := routingIDs[rawModel]
		if !ok || !middleware.IsModelInList(canonicalModel, modelsString) {
			continue
		}
		if _, ok := seen[canonicalModel]; ok {
			continue
		}
		seen[canonicalModel] = struct{}{}
		modelNames = append(modelNames, canonicalModel)
	}
	return modelNames
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
		// Token has model restrictions, use those models. The assertion is checked:
		// an unchecked one would panic the handler on the same corrupted-context
		// state that filterAbilitiesByTokenAllowList merely logs.
		modelsString, _ := availableModels.(string)
		if modelsString != "" {
			ctx, userGroup, err := getRequestUserGroup(c)
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
			abilities, err := model.CacheGetGroupModelsV2(ctx, userGroup)
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
			channelCache := make(map[int]*model.Channel)
			visibleAbilities := filterVisibleAbilities(abilities, channelCache)
			modelNames := intersectTokenModelIDs(modelsString, visibleAbilities)
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
