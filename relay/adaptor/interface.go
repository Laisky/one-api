package adaptor

import (
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
)

// ModelConfig represents pricing and configuration information for a model
// This structure consolidates both pricing (Ratio, CompletionRatio) and
// configuration (MaxTokens, etc.) to eliminate the need for separate ModelConfig
type ModelConfig struct {
	Ratio float64 `json:"ratio"`
	// CompletionRatio represents the output rate / input rate
	//
	// The upstream channel applies distinct pricing for cache‑hit and cache‑miss inputs,
	// while the output price remains the same, equal to Ratio * CompletionRatio.
	CompletionRatio float64 `json:"completion_ratio,omitempty"`
	// CachedInputRatio specifies price per cached input token.
	// If non-zero, it overrides Ratio for cached input tokens. Negative means free.
	CachedInputRatio float64 `json:"cached_input_ratio,omitempty"`
	// CacheWrite5mRatio specifies price per input token written to a 5-minute cache window.
	// If zero, falls back to normal input Ratio. Negative means free (not expected in production).
	CacheWrite5mRatio float64 `json:"cache_write_5m_ratio,omitempty"`
	// CacheWrite1hRatio specifies price per input token written to a 1-hour cache window.
	// If zero, falls back to normal input Ratio. Negative means free (not expected in production).
	CacheWrite1hRatio float64 `json:"cache_write_1h_ratio,omitempty"`
	// Tiers contains tiered pricing data. If present, the first tier is the base
	// Ratio/CompletionRatio/Cached* fields in this struct. Elements must be sorted
	// ascending by InputTokenThreshold and represent the 2nd+ tiers.
	Tiers []ModelRatioTier `json:"tiers,omitempty"`
	// MaxTokens represents the maximum token limit for this model on this channel
	// 0 means no limit (infinity)
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// Video holds per-second pricing metadata for video generation models.
	Video *VideoPricingConfig `json:"video,omitempty"`
	// Audio captures pricing metadata for audio prompt and completion billing.
	Audio *AudioPricingConfig `json:"audio,omitempty"`
	// Image captures pricing metadata for image prompt and render billing.
	Image *ImagePricingConfig `json:"image,omitempty"`
	// Embedding captures modality-specific pricing metadata for embedding requests.
	Embedding *EmbeddingPricingConfig `json:"embedding,omitempty"`
	// ContextLength is the total token context (input+output) the model supports.
	// 0 means unspecified — caller should fall back to a reasonable default.
	ContextLength int32 `json:"context_length,omitempty"`
	// MaxOutputTokens is the maximum tokens the model can emit per response.
	// 0 means unspecified — caller should fall back to ContextLength or a default.
	MaxOutputTokens int32 `json:"max_output_tokens,omitempty"`
	// InputModalities lists supported input modalities. Empty implies ["text"].
	// Valid OpenRouter values include "text", "image", "file".
	InputModalities []string `json:"input_modalities,omitempty"`
	// OutputModalities lists supported output modalities. Empty implies ["text"].
	OutputModalities []string `json:"output_modalities,omitempty"`
	// SupportedFeatures advertises capabilities. Subset of:
	// "tools", "json_mode", "structured_outputs", "logprobs", "web_search", "reasoning".
	SupportedFeatures []string `json:"supported_features,omitempty"`
	// SupportedSamplingParameters lists OpenAI-compatible sampling parameters this model accepts.
	// Empty means caller should use a default conservative set.
	SupportedSamplingParameters []string `json:"supported_sampling_parameters,omitempty"`
	// Quantization advertises numeric precision. Empty means unspecified.
	// Valid OpenRouter values: "int4","int8","fp4","fp6","fp8","fp16","bf16","fp32".
	Quantization string `json:"quantization,omitempty"`
	// HuggingFaceID identifies the model on HuggingFace if applicable. Empty if not on HF.
	HuggingFaceID string `json:"hugging_face_id,omitempty"`
	// Description is a short human-readable description (optional).
	Description string `json:"description,omitempty"`
}

// EmbeddingPricingConfig captures modality-specific pricing metadata for embedding requests.
// Token ratios are expressed per input token, while the USD-per-unit fields are used when
// the relay only has image counts, audio durations, or video frame counts available.
type EmbeddingPricingConfig struct {
	TextTokenRatio     float64 `json:"text_token_ratio,omitempty"`
	ImageTokenRatio    float64 `json:"image_token_ratio,omitempty"`
	AudioTokenRatio    float64 `json:"audio_token_ratio,omitempty"`
	VideoTokenRatio    float64 `json:"video_token_ratio,omitempty"`
	DocumentTokenRatio float64 `json:"document_token_ratio,omitempty"`
	UsdPerImage        float64 `json:"usd_per_image,omitempty"`
	UsdPerAudioSecond  float64 `json:"usd_per_audio_second,omitempty"`
	UsdPerVideoFrame   float64 `json:"usd_per_video_frame,omitempty"`
	UsdPerDocumentPage float64 `json:"usd_per_document_page,omitempty"`
}

// HasData reports whether the embedding configuration contains any pricing metadata.
func (cfg *EmbeddingPricingConfig) HasData() bool {
	if cfg == nil {
		return false
	}
	return cfg.TextTokenRatio != 0 || cfg.ImageTokenRatio != 0 || cfg.AudioTokenRatio != 0 ||
		cfg.VideoTokenRatio != 0 || cfg.DocumentTokenRatio != 0 || cfg.UsdPerImage != 0 ||
		cfg.UsdPerAudioSecond != 0 || cfg.UsdPerVideoFrame != 0 || cfg.UsdPerDocumentPage != 0
}

// Clone returns a copy of the embedding pricing configuration.
func (cfg *EmbeddingPricingConfig) Clone() *EmbeddingPricingConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	return &clone
}

// VideoPricingConfig captures pricing metadata for video generation requests.
// Pricing is expressed as a per-second USD cost that can be adjusted via resolution
// multipliers relative to the base resolution.
type VideoPricingConfig struct {
	// PerSecondUsd is the USD price per rendered second at the base resolution.
	PerSecondUsd float64 `json:"per_second_usd,omitempty"`
	// BaseResolution identifies the resolution treated as multiplier 1. Empty means unspecified.
	BaseResolution string `json:"base_resolution,omitempty"`
	// ResolutionMultipliers scales the base price for specific resolutions. Keys should be
	// normalized via normalizeResolutionKey and values must be positive.
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"`
}

// HasData reports whether the configuration contains any pricing information.
func (cfg *VideoPricingConfig) HasData() bool {
	if cfg == nil {
		return false
	}
	if cfg.PerSecondUsd > 0 {
		return true
	}
	return len(cfg.ResolutionMultipliers) > 0
}

// Clone returns a deep copy of the video pricing configuration.
func (cfg *VideoPricingConfig) Clone() *VideoPricingConfig {
	if cfg == nil {
		return nil
	}
	clone := &VideoPricingConfig{
		PerSecondUsd:   cfg.PerSecondUsd,
		BaseResolution: cfg.BaseResolution,
	}
	if len(cfg.ResolutionMultipliers) > 0 {
		clone.ResolutionMultipliers = make(map[string]float64, len(cfg.ResolutionMultipliers))
		maps.Copy(clone.ResolutionMultipliers, cfg.ResolutionMultipliers)
	}
	return clone
}

// EffectiveMultiplier resolves the multiplier for the supplied resolution.
// It first normalizes the key to handle orientation swaps (e.g., 720x1280 vs 1280x720)
// and falls back to the raw trimmed key if no normalized match is present.
func (cfg *VideoPricingConfig) EffectiveMultiplier(resolution string) float64 {
	if cfg == nil {
		return 1
	}
	normalized := normalizeResolutionKey(resolution)
	if normalized != "" && len(cfg.ResolutionMultipliers) > 0 {
		if multiplier, ok := cfg.ResolutionMultipliers[normalized]; ok && multiplier > 0 {
			return multiplier
		}
	}
	trimmed := strings.TrimSpace(strings.ToLower(resolution))
	if trimmed != "" && len(cfg.ResolutionMultipliers) > 0 {
		if multiplier, ok := cfg.ResolutionMultipliers[trimmed]; ok && multiplier > 0 {
			return multiplier
		}
	}
	if cfg.BaseResolution != "" {
		base := normalizeResolutionKey(cfg.BaseResolution)
		if base != "" && base == normalized {
			return 1
		}
	}
	return 1
}

func normalizeResolutionKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == 'x' || r == '*' || r == '\u00D7'
	})
	if len(parts) != 2 {
		return trimmed
	}
	width, err1 := strconv.Atoi(parts[0])
	height, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || width <= 0 || height <= 0 {
		return trimmed
	}
	if width < height {
		width, height = height, width
	}
	return strconv.Itoa(width) + "x" + strconv.Itoa(height)
}

// ModelRatioTier describes pricing for a specific input token tier. It overrides
// the base ModelConfig starting at InputTokenThreshold. Zero values for optional
// fields mean "inherit from base"; negative cached ratios mean free tokens.
type ModelRatioTier struct {
	// Base price for this tier (per input token)
	Ratio float64 `json:"ratio"`

	// Output‑to‑input multiplier for this tier (optional)
	CompletionRatio float64 `json:"completion_ratio,omitempty"`

	// Discount for cached input (optional)
	CachedInputRatio float64 `json:"cached_input_ratio,omitempty"`

	// Cache-write prices for this tier (optional)
	CacheWrite5mRatio float64 `json:"cache_write_5m_ratio,omitempty"`
	CacheWrite1hRatio float64 `json:"cache_write_1h_ratio,omitempty"`

	// The minimum input‑token count at which this tier becomes applicable
	InputTokenThreshold int `json:"input_token_threshold"`
}

// ToolPricingConfig describes the per-invocation pricing for a provider built-in tool.
// Prices can be expressed either as USD per call or precomputed quota units per call.
type ToolPricingConfig struct {
	// UsdPerCall represents the USD price per single invocation of the tool.
	// Leave zero when using quota-based pricing.
	UsdPerCall float64 `json:"usd_per_call,omitempty"`
	// QuotaPerCall overrides the per-invocation cost directly in quota units.
	// Zero means "not specified" unless the tool is intentionally free.
	QuotaPerCall int64 `json:"quota_per_call,omitempty"`
}

// ChannelToolConfig defines channel-scoped built-in tool policies and pricing metadata.
type ChannelToolConfig struct {
	// Whitelist enumerates provider-built tools permitted for this channel. Nil/empty allows all.
	Whitelist []string `json:"whitelist,omitempty"`
	// Pricing defines per-tool invocation pricing for the entire channel.
	Pricing map[string]ToolPricingConfig `json:"pricing,omitempty"`
}

// AudioPricingConfig captures pricing metadata for audio prompts and completions.
// PromptRatio converts audio prompt tokens to text-token billing units; CompletionRatio
// applies when upstream returns audio completions. Per-second fields allow direct
// billing of duration-based models.
type AudioPricingConfig struct {
	PromptRatio               float64 `json:"prompt_ratio,omitempty"`
	CompletionRatio           float64 `json:"completion_ratio,omitempty"`
	PromptTokensPerSecond     float64 `json:"prompt_tokens_per_second,omitempty"`
	CompletionTokensPerSecond float64 `json:"completion_tokens_per_second,omitempty"`
	UsdPerSecond              float64 `json:"usd_per_second,omitempty"`
}

// HasData reports whether the audio configuration carries any non-zero metadata.
func (cfg *AudioPricingConfig) HasData() bool {
	if cfg == nil {
		return false
	}
	return cfg.PromptRatio != 0 || cfg.CompletionRatio != 0 || cfg.PromptTokensPerSecond != 0 ||
		cfg.CompletionTokensPerSecond != 0 || cfg.UsdPerSecond != 0
}

// Clone returns a copy of the audio pricing configuration.
func (cfg *AudioPricingConfig) Clone() *AudioPricingConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	return &clone
}

// ImagePricingConfig captures prompt and render billing metadata for image models.
// Size and quality multipliers scale the base price; missing entries imply fallback to 1.0.
type ImagePricingConfig struct {
	PricePerImageUsd       float64                       `json:"price_per_image_usd,omitempty"`
	PromptRatio            float64                       `json:"prompt_ratio,omitempty"`
	DefaultSize            string                        `json:"default_size,omitempty"`
	DefaultQuality         string                        `json:"default_quality,omitempty"`
	PromptTokenLimit       int                           `json:"prompt_token_limit,omitempty"`
	MinImages              int                           `json:"min_images,omitempty"`
	MaxImages              int                           `json:"max_images,omitempty"`
	SizeMultipliers        map[string]float64            `json:"size_multipliers,omitempty"`
	QualityMultipliers     map[string]float64            `json:"quality_multipliers,omitempty"`
	QualitySizeMultipliers map[string]map[string]float64 `json:"quality_size_multipliers,omitempty"`
}

// HasData reports whether the image configuration contains any pricing metadata.
func (cfg *ImagePricingConfig) HasData() bool {
	if cfg == nil {
		return false
	}
	if cfg.PricePerImageUsd > 0 || cfg.PromptRatio > 0 || cfg.PromptTokenLimit > 0 || cfg.MinImages > 0 || cfg.MaxImages > 0 {
		return true
	}
	if len(cfg.SizeMultipliers) > 0 || len(cfg.QualityMultipliers) > 0 || len(cfg.QualitySizeMultipliers) > 0 {
		return true
	}
	return false
}

// Clone returns a deep copy of the image pricing configuration.
func (cfg *ImagePricingConfig) Clone() *ImagePricingConfig {
	if cfg == nil {
		return nil
	}
	clone := *cfg
	if len(cfg.SizeMultipliers) > 0 {
		clone.SizeMultipliers = make(map[string]float64, len(cfg.SizeMultipliers))
		for k, v := range cfg.SizeMultipliers {
			clone.SizeMultipliers[k] = v
		}
	}
	if len(cfg.QualityMultipliers) > 0 {
		clone.QualityMultipliers = make(map[string]float64, len(cfg.QualityMultipliers))
		for k, v := range cfg.QualityMultipliers {
			clone.QualityMultipliers[k] = v
		}
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		clone.QualitySizeMultipliers = make(map[string]map[string]float64, len(cfg.QualitySizeMultipliers))
		for quality, sizes := range cfg.QualitySizeMultipliers {
			inner := make(map[string]float64, len(sizes))
			for size, value := range sizes {
				inner[size] = value
			}
			clone.QualitySizeMultipliers[quality] = inner
		}
	}
	return &clone
}

// ToolingDefaultsProvider is implemented by adaptors that expose built-in tool defaults.
type ToolingDefaultsProvider interface {
	DefaultToolingConfig() ChannelToolConfig
}

type Adaptor interface {
	Init(meta *meta.Meta)
	GetRequestURL(meta *meta.Meta) (string, error)
	SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error
	ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error)
	ConvertImageRequest(c *gin.Context, request *model.ImageRequest) (any, error)
	ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error)
	DoRequest(c *gin.Context, meta *meta.Meta, requestBody io.Reader) (*http.Response, error)
	DoResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode)
	GetModelList() []string
	GetChannelName() string

	// Pricing methods - each adapter manages its own model pricing
	GetDefaultModelPricing() map[string]ModelConfig
	GetModelRatio(modelName string) float64
	GetCompletionRatio(modelName string) float64
}

// OCRAdaptor represents adaptors that can natively consume the dedicated OCR / layout-parsing DTO.
// Adaptors must implement this interface to accept /v1/layout_parsing requests; otherwise the
// controller will reject the call as unsupported.
type OCRAdaptor interface {
	ConvertOCRRequest(c *gin.Context, request *model.OCRRequest) (any, error)
	DoOCRResponse(c *gin.Context, resp *http.Response, meta *meta.Meta) (usage *model.Usage, err *model.ErrorWithStatusCode)
}

// RerankAdaptor represents adaptors that can natively consume the dedicated rerank DTO.
// Adaptors must implement this interface to accept /v1/rerank requests; otherwise the
// controller will reject the call as unsupported.
type RerankAdaptor interface {
	ConvertRerankRequest(c *gin.Context, request *model.RerankRequest) (any, error)
}

// DefaultPricingMethods provides default implementations for adapters without specific pricing
type DefaultPricingMethods struct{}

func (d *DefaultPricingMethods) GetDefaultModelPricing() map[string]ModelConfig {
	return make(map[string]ModelConfig) // Empty pricing map
}

func (d *DefaultPricingMethods) GetModelRatio(modelName string) float64 {
	// Fallback to a reasonable default
	return 2.5 * billingratio.MilliTokensUsd // 2.5 USD per million tokens in internal quota units
}

func (d *DefaultPricingMethods) GetCompletionRatio(modelName string) float64 {
	return 1.0 // Default completion ratio
}

// DefaultToolingConfig returns an empty tooling configuration so channels opt-in explicitly.
func (d *DefaultPricingMethods) DefaultToolingConfig() ChannelToolConfig {
	return ChannelToolConfig{}
}

func (d *DefaultPricingMethods) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	// Default implementation: not supported
	return nil, errors.New("Claude Messages API not supported by this adaptor")
}

// GetModelListFromPricing derives model list from pricing map keys
// This eliminates the need for separate ModelList variables
func GetModelListFromPricing(pricing map[string]ModelConfig) []string {
	models := make([]string, 0, len(pricing))
	for model := range pricing {
		models = append(models, model)
	}
	return models
}
