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
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/common/helper"
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
func loadChannelCached(channelID int, cache map[int]*model.Channel) (*model.Channel, error) {
	if channelID == 0 {
		return nil, errors.New("channel id is required")
	}
	if channel, ok := cache[channelID]; ok {
		return channel, nil
	}
	channel, err := model.GetChannelById(channelID, false)
	if err != nil {
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

// respondModelNotFound returns the OpenAI-compatible model-not-found error payload.
func respondModelNotFound(c *gin.Context, modelID string) {
	msg := fmt.Sprintf("The model '%s' does not exist", modelID)
	respErr := relaymodel.Error{Message: msg, Type: relaymodel.ErrorTypeInvalidRequest, Param: "model", Code: "model_not_found", RawError: errors.New(msg)}
	c.JSON(http.StatusOK, gin.H{
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
		return nil, errors.Wrap(err, "get all enabled channels")
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
			}
			if !filters.matchesModel(info) {
				continue
			}
			result[modelName] = info
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
	channelCache := make(map[int]*model.Channel)
	availableAbilities = filterVisibleAbilities(availableAbilities, channelCache)

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
	var matched *dto.EnabledAbility
	for i := range visibleAbilities {
		if strings.EqualFold(visibleAbilities[i].Model, modelId) {
			matched = &visibleAbilities[i]
			modelId = strings.TrimSpace(visibleAbilities[i].Model)
			break
		}
	}
	if matched == nil {
		respondModelNotFound(c, modelId)
		return
	}

	if model, ok := modelsMap[modelId]; ok {
		c.JSON(http.StatusOK, model)
		return
	}
	for key, modelEntry := range modelsMap {
		if strings.EqualFold(key, modelId) {
			c.JSON(http.StatusOK, modelEntry)
			return
		}
	}
	lg := gmw.GetLogger(c)
	if snapshot, err := getSupportedModelsSnapshot(); err == nil {
		for _, m := range snapshot {
			if strings.EqualFold(m.Id, modelId) {
				c.JSON(http.StatusOK, m)
				return
			}
		}
	} else if lg != nil {
		lg.Debug("failed to build supported models snapshot for lookup", zap.Error(err))
	}
	if entry, ok := buildModelEntryFromAbility(modelId, matched.ChannelId, matched.ChannelType, int(time.Now().Unix()), channelCache); ok {
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
			visibleModels := make(map[string]struct{}, len(abilities))
			for _, ability := range filterVisibleAbilities(abilities, channelCache) {
				visibleModels[strings.ToLower(strings.TrimSpace(ability.Model))] = struct{}{}
			}

			tokenModels := strings.Split(modelsString, ",")
			modelNames := make([]string, 0, len(tokenModels))
			seen := make(map[string]struct{}, len(tokenModels))
			for _, rawModel := range tokenModels {
				modelName := strings.TrimSpace(rawModel)
				if modelName == "" {
					continue
				}
				key := strings.ToLower(modelName)
				if _, ok := visibleModels[key]; !ok {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				modelNames = append(modelNames, modelName)
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
