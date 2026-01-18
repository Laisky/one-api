package model

import (
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	// MySQL driver error inspection (for robust error code detection)
	mysql_driver "github.com/go-sql-driver/mysql"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/common/logger"
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
)

type Channel struct {
	Id                 int     `json:"id"`
	Type               int     `json:"type" gorm:"default:0"`
	Key                string  `json:"key" gorm:"type:text"`
	Status             int     `json:"status" gorm:"default:1"`
	Name               string  `json:"name" gorm:"index"`
	Weight             *uint   `json:"weight" gorm:"default:0"`
	CreatedTime        int64   `json:"created_time" gorm:"bigint"`
	TestTime           int64   `json:"test_time" gorm:"bigint"`
	ResponseTime       int     `json:"response_time"` // in milliseconds
	BaseURL            *string `json:"base_url" gorm:"column:base_url;default:''"`
	Other              *string `json:"other"`   // DEPRECATED: please save config to field Config
	Balance            float64 `json:"balance"` // in USD
	BalanceUpdatedTime int64   `json:"balance_updated_time" gorm:"bigint"`
	Models             string  `json:"models"`
	ModelConfigs       *string `json:"model_configs" gorm:"type:text"`
	Group              string  `json:"group" gorm:"type:varchar(32);default:'default'"`
	UsedQuota          int64   `json:"used_quota" gorm:"bigint;default:0"`
	ModelMapping       *string `json:"model_mapping" gorm:"type:text"`
	Priority           *int64  `json:"priority" gorm:"bigint;default:0"`
	Config             string  `json:"config"`
	SystemPrompt       *string `json:"system_prompt" gorm:"type:text"`
	RateLimit          *int    `json:"ratelimit" gorm:"column:ratelimit;default:0"`
	// Preferred testing model for this channel (optional)
	// If empty or nil, the system will auto-select the cheapest supported model at test time.
	TestingModel *string `json:"testing_model" gorm:"column:testing_model;type:varchar(255)"`
	// Channel-specific pricing tables
	// DEPRECATED: Use ModelConfigs instead. These fields are kept for backward compatibility and migration.
	ModelRatio      *string `json:"model_ratio" gorm:"type:text"`      // DEPRECATED: JSON string of model pricing ratios
	CompletionRatio *string `json:"completion_ratio" gorm:"type:text"` // DEPRECATED: JSON string of completion pricing ratios
	CreatedAt       int64   `json:"created_at" gorm:"bigint;autoCreateTime:milli"`
	UpdatedAt       int64   `json:"updated_at" gorm:"bigint;autoUpdateTime:milli"`
	// AWS-specific configuration
	InferenceProfileArnMap *string `json:"inference_profile_arn_map" gorm:"type:text"` // JSON string mapping model names to AWS Bedrock Inference Profile ARNs
}

type ChannelConfig struct {
	Region            string                `json:"region,omitempty"`
	SK                string                `json:"sk,omitempty"`
	AK                string                `json:"ak,omitempty"`
	UserID            string                `json:"user_id,omitempty"`
	APIVersion        string                `json:"api_version,omitempty"`
	LibraryID         string                `json:"library_id,omitempty"`
	Plugin            string                `json:"plugin,omitempty"`
	VertexAIProjectID string                `json:"vertex_ai_project_id,omitempty"`
	VertexAIADC       string                `json:"vertex_ai_adc,omitempty"`
	AuthType          string                `json:"auth_type,omitempty"`
	APIFormat         string                `json:"api_format,omitempty"`
	Tooling           *ChannelToolingConfig `json:"tooling,omitempty"`
	MCPToolBlacklist  []string              `json:"mcp_tool_blacklist,omitempty"`
	// SupportedEndpoints is a list of endpoint names that this channel supports.
	// When nil or empty, the channel uses default endpoints for its type.
	// Endpoint names: chat_completions, completions, embeddings, moderations,
	// images_generations, images_edits, audio_speech, audio_transcription,
	// audio_translation, rerank, response_api, claude_messages, realtime, videos.
	SupportedEndpoints []string `json:"supported_endpoints,omitempty"`
}

type ModelConfig struct {
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// Legacy image pricing field kept for backwards compatibility during migrations.
	ImagePriceUsd float64            `json:"image_price_usd,omitempty"`
	Image         *ImagePricingLocal `json:"image,omitempty"`
}

// ModelConfigLocal represents the local definition of ModelConfig to avoid import cycles
// This should match the structure in relay/adaptor/interface.go
type ModelConfigLocal struct {
	Ratio           float64            `json:"ratio"`
	CompletionRatio float64            `json:"completion_ratio,omitempty"`
	MaxTokens       int32              `json:"max_tokens,omitempty"`
	Video           *VideoPricingLocal `json:"video,omitempty"`
	Audio           *AudioPricingLocal `json:"audio,omitempty"`
	Image           *ImagePricingLocal `json:"image,omitempty"`
}

// VideoPricingLocal represents channel-scoped video pricing metadata stored alongside model configs.
type VideoPricingLocal struct {
	PerSecondUsd          float64            `json:"per_second_usd,omitempty"`
	BaseResolution        string             `json:"base_resolution,omitempty"`
	ResolutionMultipliers map[string]float64 `json:"resolution_multipliers,omitempty"`
}

// AudioPricingLocal mirrors adaptor.AudioPricingConfig for persistence without creating import cycles.
type AudioPricingLocal struct {
	PromptRatio               float64 `json:"prompt_ratio,omitempty"`
	CompletionRatio           float64 `json:"completion_ratio,omitempty"`
	PromptTokensPerSecond     float64 `json:"prompt_tokens_per_second,omitempty"`
	CompletionTokensPerSecond float64 `json:"completion_tokens_per_second,omitempty"`
	UsdPerSecond              float64 `json:"usd_per_second,omitempty"`
}

// ImagePricingLocal mirrors adaptor.ImagePricingConfig for persistence.
type ImagePricingLocal struct {
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

// ToolPricingLocal is the channel-scoped representation of adaptor.ToolPricingConfig.
// Administrators can express pricing either in USD per call or direct quota units.
type ToolPricingLocal struct {
	UsdPerCall   float64 `json:"usd_per_call,omitempty"`
	QuotaPerCall int64   `json:"quota_per_call,omitempty"`
}

// normalizeToolWhitelist trims, deduplicates, and validates the provided tooling whitelist.
func normalizeToolWhitelist(list []string) ([]string, error) {
	if len(list) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(list))
	normalized := make([]string, 0, len(list))
	for _, raw := range list {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, errors.New("tool whitelist cannot contain empty entries")
		}
		lower := strings.ToLower(trimmed)
		if _, exists := seen[lower]; exists {
			continue
		}
		seen[lower] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// normalizeToolPricingMap validates provider pricing definitions and removes empty entries.
func normalizeToolPricingMap(pricing map[string]ToolPricingLocal) (map[string]ToolPricingLocal, error) {
	if len(pricing) == 0 {
		return nil, nil
	}
	normalized := make(map[string]ToolPricingLocal, len(pricing))
	for rawName, value := range pricing {
		name := strings.TrimSpace(rawName)
		if name == "" {
			return nil, errors.New("tool pricing cannot contain empty tool names")
		}
		if value.UsdPerCall < 0 {
			return nil, errors.Errorf("tool %s usd_per_call cannot be negative", name)
		}
		if value.QuotaPerCall < 0 {
			return nil, errors.Errorf("tool %s quota_per_call cannot be negative", name)
		}
		normalized[name] = value
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// normalizeChannelToolingConfig applies whitelist/pricing validation to a channel tooling configuration.
func normalizeChannelToolingConfig(cfg *ChannelToolingConfig) (*ChannelToolingConfig, error) {
	if cfg == nil {
		return nil, nil
	}
	normalized := &ChannelToolingConfig{}
	list, err := normalizeToolWhitelist(cfg.Whitelist)
	if err != nil {
		return nil, err
	}
	if len(list) > 0 {
		normalized.Whitelist = list
	}
	pricing, err := normalizeToolPricingMap(cfg.Pricing)
	if err != nil {
		return nil, err
	}
	if len(pricing) > 0 {
		normalized.Pricing = pricing
	}
	if len(normalized.Whitelist) == 0 && len(normalized.Pricing) == 0 {
		return nil, nil
	}
	return normalized, nil
}

// cloneChannelToolingConfig produces a deep copy of the tooling configuration so callers can mutate safely.
func cloneChannelToolingConfig(cfg *ChannelToolingConfig) *ChannelToolingConfig {
	if cfg == nil {
		return nil
	}
	clone := &ChannelToolingConfig{}
	if len(cfg.Whitelist) > 0 {
		clone.Whitelist = append([]string(nil), cfg.Whitelist...)
	}
	if len(cfg.Pricing) > 0 {
		clone.Pricing = make(map[string]ToolPricingLocal, len(cfg.Pricing))
		maps.Copy(clone.Pricing, cfg.Pricing)
	}
	return clone
}

// normalizeModelConfigLocal trims whitespace and validates numeric fields.
func normalizeModelConfigLocal(cfg ModelConfigLocal) (ModelConfigLocal, error) {
	video, err := normalizeVideoPricingLocal(cfg.Video)
	if err != nil {
		return ModelConfigLocal{}, err
	}
	audio, err := normalizeAudioPricingLocal(cfg.Audio)
	if err != nil {
		return ModelConfigLocal{}, err
	}
	image, err := normalizeImagePricingLocal(cfg.Image)
	if err != nil {
		return ModelConfigLocal{}, err
	}

	normalized := ModelConfigLocal{
		Ratio:           cfg.Ratio,
		CompletionRatio: cfg.CompletionRatio,
		MaxTokens:       cfg.MaxTokens,
	}
	if video != nil {
		normalized.Video = video
	}
	if audio != nil {
		normalized.Audio = audio
	}
	if image != nil {
		normalized.Image = image
	}
	return normalized, nil
}

// ChannelToolingConfig captures channel-scoped built-in tool policy and pricing.
type ChannelToolingConfig struct {
	Whitelist []string                    `json:"whitelist,omitempty"`
	Pricing   map[string]ToolPricingLocal `json:"pricing,omitempty"`
}

// Migration control & state
var (
	channelFieldMigrationOnce sync.Once
	channelFieldMigrated      atomic.Bool // true after successful schema migration
)

// isMySQLDataTooLongErr checks whether an error is a MySQL "data too long" error (code 1406)
func isMySQLDataTooLongErr(err error) bool {
	if err == nil {
		return false
	}
	if merr, ok := err.(*mysql_driver.MySQLError); ok {
		if merr.Number == 1406 { // ER_DATA_TOO_LONG
			return true
		}
	}
	// fallback substring match (defensive for wrapped errors)
	if strings.Contains(err.Error(), "Data too long for column") {
		return true
	}
	return false
}

func GetAllChannels(startIdx int, num int, scope string, sortBy string, sortOrder string) ([]*Channel, error) {
	var channels []*Channel
	var err error

	// Default sorting
	orderClause := "id desc"
	if sortBy != "" {
		if sortOrder == "asc" {
			orderClause = sortBy + " asc"
		} else {
			orderClause = sortBy + " desc"
		}
	}

	switch scope {
	case "all":
		if num > 0 {
			// Apply pagination when num > 0
			err = DB.Order(orderClause).Limit(num).Offset(startIdx).Find(&channels).Error
		} else {
			// Return all channels when num = 0 (backward compatibility)
			err = DB.Order(orderClause).Find(&channels).Error
		}
	case "disabled":
		err = DB.Order(orderClause).Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Find(&channels).Error
	default:
		err = DB.Order(orderClause).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
	}
	return channels, err
}

func GetChannelCount() (count int64, err error) {
	err = DB.Model(&Channel{}).Count(&count).Error
	return count, err
}

// GetAllEnabledChannels returns all channels with status = ChannelStatusEnabled
func GetAllEnabledChannels() ([]*Channel, error) {
	var channels []*Channel
	if err := DB.Where("status = ?", ChannelStatusEnabled).Find(&channels).Error; err != nil {
		return nil, errors.Wrap(err, "query enabled channels")
	}
	return channels, nil
}

// GetEnabledChannelsVersionSignature returns a signature string that changes whenever
// the set of enabled channels (or their configurations) is modified.
func GetEnabledChannelsVersionSignature() (string, error) {
	type snapshot struct {
		Count        int64 `gorm:"column:count"`
		MaxUpdatedAt int64 `gorm:"column:max_updated_at"`
	}
	var result snapshot
	if err := DB.Model(&Channel{}).
		Where("status = ?", ChannelStatusEnabled).
		Select("COUNT(*) AS count, COALESCE(MAX(updated_at), 0) AS max_updated_at").
		Scan(&result).Error; err != nil {
		return "", errors.Wrap(err, "query enabled channel version")
	}
	return fmt.Sprintf("%d:%d", result.Count, result.MaxUpdatedAt), nil
}

func SearchChannels(keyword string, sortBy string, sortOrder string) (channels []*Channel, err error) {
	// Default sorting
	orderClause := "id desc"
	if sortBy != "" {
		if sortOrder == "asc" {
			orderClause = sortBy + " asc"
		} else {
			orderClause = sortBy + " desc"
		}
	}

	err = DB.Omit("key").Where("id = ? or name LIKE ?", helper.String2Int(keyword), keyword+"%").Order(orderClause).Find(&channels).Error
	return channels, err
}

func GetChannelById(id int, selectAll bool) (*Channel, error) {
	channel := Channel{Id: id}
	var err error
	if selectAll {
		err = DB.First(&channel, "id = ?", id).Error
	} else {
		err = DB.Omit("key").First(&channel, "id = ?", id).Error
	}
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get channel by id=%d, selectAll=%t", id, selectAll)
	}
	return &channel, nil
}

func BatchInsertChannels(channels []Channel) error {
	err := DB.Create(&channels).Error
	if err != nil {
		return errors.Wrapf(err, "failed to batch insert %d channels", len(channels))
	}
	for i, channel_ := range channels {
		err = channel_.AddAbilities()
		if err != nil {
			return errors.Wrapf(err, "failed to add abilities for channel %d (index %d) during batch insert", channel_.Id, i)
		}
	}
	InitChannelCache()
	return nil
}

func (channel *Channel) GetPriority() int64 {
	if channel.Priority == nil {
		return 0
	}
	return *channel.Priority
}

func (channel *Channel) GetBaseURL() string {
	if channel.BaseURL == nil {
		return ""
	}
	return *channel.BaseURL
}

// GetDefaultBaseURL returns the default base URL for the channel type based on built-in mapping.
// Returns empty string if unknown.
func (channel *Channel) GetDefaultBaseURL() string {
	// Import lazily to avoid circulars; mirror relay/channeltype mapping here via function in callers.
	return "" // kept simple; callers should use relay/channeltype.ChannelBaseURLs when needed
}

func (channel *Channel) GetModelMapping() map[string]string {
	if channel.ModelMapping == nil || *channel.ModelMapping == "" || *channel.ModelMapping == "{}" {
		return nil
	}
	modelMapping := make(map[string]string)
	err := json.Unmarshal([]byte(*channel.ModelMapping), &modelMapping)
	if err != nil {
		logger.Logger.Error("failed to unmarshal model mapping for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return modelMapping
}

// GetSupportedModelNames returns the list of model names the channel currently supports
// based on the comma-separated Models field.
func (channel *Channel) GetSupportedModelNames() []string {
	models := strings.TrimSpace(channel.Models)
	if models == "" {
		return nil
	}
	parts := strings.Split(models, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SupportsModel reports whether the channel allows the provided model name.
// When the channel has no explicit supported models configured (empty list),
// the channel is treated as supporting all models.
func (channel *Channel) SupportsModel(modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return true
	}
	supported := channel.GetSupportedModelNames()
	if len(supported) == 0 {
		return true
	}
	for _, name := range supported {
		if strings.EqualFold(name, modelName) {
			return true
		}
	}
	if mapping := channel.GetModelMapping(); mapping != nil {
		if mapped := strings.TrimSpace(mapping[modelName]); mapped != "" {
			for _, name := range supported {
				if strings.EqualFold(name, mapped) {
					return true
				}
			}
		}
	}
	return false
}

// GetCheapestSupportedModel returns the cheapest model among the channel's currently
// supported models using channel-specific ModelConfigs ratios when available.
// Returns empty string if none found.
func (channel *Channel) GetCheapestSupportedModel() string {
	names := channel.GetSupportedModelNames()
	if len(names) == 0 {
		return ""
	}
	// Use unified ModelConfigs to get ratio if available
	configs := channel.GetModelPriceConfigs()
	var (
		cheapestName  string
		cheapestRatio float64 = 0
		initialized   bool
	)
	for _, name := range names {
		var r float64
		if cfg, ok := configs[name]; ok {
			r = cfg.Ratio
		} else {
			// fallback to old per-field ratios if still present
			if mr := channel.GetModelRatio(); mr != nil {
				r = mr[name]
			}
		}
		// only consider positive ratios; if zero, still consider but at lowest weight
		if !initialized {
			cheapestName, cheapestRatio, initialized = name, r, true
			continue
		}
		if r < cheapestRatio {
			cheapestName, cheapestRatio = name, r
		}
	}
	return cheapestName
}

// GetModelConfig returns the model configuration for a specific model
// DEPRECATED: Use GetModelPriceConfig() instead. This method is kept for backward compatibility.
func (channel *Channel) GetModelConfig(modelName string) *ModelConfig {
	// Only use unified ModelConfigs after migration
	priceConfig := channel.GetModelPriceConfig(modelName)
	if priceConfig != nil {
		// Convert ModelPriceLocal to ModelConfig for backward compatibility
		legacy := &ModelConfig{MaxTokens: priceConfig.MaxTokens}
		if priceConfig.Image != nil {
			legacy.ImagePriceUsd = priceConfig.Image.PricePerImageUsd
			legacy.Image = priceConfig.Image
		}
		return legacy
	}

	return nil
}

// MigrateModelConfigsToModelPrice migrates existing ModelConfigs data from the old format
// (map[string]ModelConfig) to the new format (map[string]ModelPriceLocal)
// This handles cases where contributors have already applied the PR changes locally
func (channel *Channel) MigrateModelConfigsToModelPrice() error {
	if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
		return nil // Nothing to migrate
	}

	// Validate JSON format first
	var rawData any
	if err := json.Unmarshal([]byte(*channel.ModelConfigs), &rawData); err != nil {
		return errors.Wrapf(err, "invalid JSON in ModelConfigs for channel %d", channel.Id)
	}

	// Check if the JSON is null, array, or string (invalid types)
	switch rawData.(type) {
	case nil:
		return errors.Errorf("ModelConfigs cannot be parsed: null value for channel %d", channel.Id)
	case []any:
		return errors.Errorf("ModelConfigs cannot be parsed: array value for channel %d", channel.Id)
	case string:
		return errors.Errorf("ModelConfigs cannot be parsed: string value for channel %d", channel.Id)
	}

	// Try to unmarshal as the new format first
	var newFormatConfigs map[string]ModelConfigLocal
	err := json.Unmarshal([]byte(*channel.ModelConfigs), &newFormatConfigs)
	if err == nil {
		// Validate the new format data
		if err := channel.validateModelPriceConfigs(newFormatConfigs); err != nil {
			return errors.Wrapf(err, "invalid ModelPriceLocal data for channel %d", channel.Id)
		}

		// Check if it has pricing data (already in new format)
		hasPricingData := false
		for _, config := range newFormatConfigs {
			if config.Ratio != 0 || config.CompletionRatio != 0 {
				hasPricingData = true
				break
			}
		}

		if hasPricingData {
			logger.Logger.Info("Channel ModelConfigs already in new format with pricing data",
				zap.Int("channel_id", channel.Id))
			return nil
		}

		logger.Logger.Info("Channel ModelConfigs in new format but needs pricing migration",
			zap.Int("channel_id", channel.Id))
	}

	// Try to unmarshal as the old format (map[string]ModelConfig)
	var oldFormatConfigs map[string]ModelConfig
	err = json.Unmarshal([]byte(*channel.ModelConfigs), &oldFormatConfigs)
	if err != nil {
		return errors.Wrapf(err, "ModelConfigs cannot be parsed in either format for channel %d", channel.Id)
	}

	// Validate old format data
	for modelName, config := range oldFormatConfigs {
		if modelName == "" {
			return errors.Errorf("empty model name found in ModelConfigs for channel %d", channel.Id)
		}
		if config.MaxTokens < 0 {
			return errors.Errorf("negative MaxTokens for model %s in channel %d", modelName, channel.Id)
		}
	}

	// Convert old format to new format
	migratedConfigs := make(map[string]ModelConfigLocal)

	// Get existing ModelRatio and CompletionRatio for this channel
	modelRatios := channel.GetModelRatio()
	completionRatios := channel.GetCompletionRatio()

	// Collect all model names from all sources
	allModelNames := make(map[string]bool)
	for modelName := range oldFormatConfigs {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}
	for modelName := range modelRatios {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}
	for modelName := range completionRatios {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}

	// Process all models from all sources
	for modelName := range allModelNames {
		newConfig := ModelConfigLocal{}

		// Start with MaxTokens from old config if available
		if oldConfig, exists := oldFormatConfigs[modelName]; exists {
			newConfig.MaxTokens = oldConfig.MaxTokens
			if oldConfig.Image != nil {
				normalizedImage, err := normalizeImagePricingLocal(oldConfig.Image)
				if err != nil {
					return errors.Wrapf(err, "invalid legacy image pricing for model %s", modelName)
				}
				newConfig.Image = normalizedImage
			} else if oldConfig.ImagePriceUsd > 0 {
				newConfig.Image = &ImagePricingLocal{PricePerImageUsd: oldConfig.ImagePriceUsd}
			}
		}

		// Add pricing information if available
		if modelRatios != nil {
			if ratio, exists := modelRatios[modelName]; exists {
				if ratio < 0 {
					return errors.Errorf("negative ratio for model %s: %f", modelName, ratio)
				}
				if ratio > 0 {
					newConfig.Ratio = ratio
				}
			}
		}
		if completionRatios != nil {
			if completionRatio, exists := completionRatios[modelName]; exists {
				if completionRatio < 0 {
					return errors.Errorf("negative completion ratio for model %s: %f", modelName, completionRatio)
				}
				if completionRatio > 0 {
					newConfig.CompletionRatio = completionRatio
				}
			}
		}

		migratedConfigs[modelName] = newConfig
	}

	// Validate migrated data
	if err := channel.validateModelPriceConfigs(migratedConfigs); err != nil {
		return errors.Wrapf(err, "migration produced invalid data for channel %d", channel.Id)
	}

	// Save the migrated data back to ModelConfigs
	jsonBytes, err := json.Marshal(migratedConfigs)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal migrated data for channel %d", channel.Id)
	}

	jsonStr := string(jsonBytes)
	channel.ModelConfigs = &jsonStr

	logger.Logger.Info("Successfully migrated ModelConfigs from old format to new format",
		zap.Int("channel_id", channel.Id),
		zap.Int("model_count", len(migratedConfigs)))
	return nil
}

// validateModelPriceConfigs validates the structure and values of ModelPriceLocal configurations
func (channel *Channel) validateModelPriceConfigs(configs map[string]ModelConfigLocal) error {
	if configs == nil {
		return nil
	}

	for modelName, config := range configs {
		// Validate model name
		if modelName == "" {
			return errors.New("empty model name found")
		}

		// Validate ratio values
		if config.Ratio < 0 {
			return errors.Errorf("negative ratio for model %s: %f", modelName, config.Ratio)
		}
		if config.CompletionRatio < 0 {
			return errors.Errorf("negative completion ratio for model %s: %f", modelName, config.CompletionRatio)
		}

		// Validate MaxTokens
		if config.MaxTokens < 0 {
			return errors.Errorf("negative MaxTokens for model %s: %d", modelName, config.MaxTokens)
		}

		hasVideoData, err := validateVideoPricingLocal(config.Video, modelName)
		if err != nil {
			return err
		}
		hasAudioData, err := validateAudioPricingLocal(config.Audio, modelName)
		if err != nil {
			return err
		}
		hasImageData, err := validateImagePricingLocal(config.Image, modelName)
		if err != nil {
			return err
		}

		// Validate that at least one field has meaningful data
		if config.Ratio == 0 && config.CompletionRatio == 0 && config.MaxTokens == 0 && !hasVideoData && !hasAudioData && !hasImageData {
			return errors.Errorf("model %s has no meaningful configuration data", modelName)
		}
	}

	return nil
}

func normalizeVideoPricingLocal(cfg *VideoPricingLocal) (*VideoPricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}

	if cfg.PerSecondUsd < 0 {
		return nil, errors.New("video per_second_usd cannot be negative")
	}

	normalized := &VideoPricingLocal{
		PerSecondUsd: cfg.PerSecondUsd,
	}
	if strings.TrimSpace(cfg.BaseResolution) != "" {
		normalized.BaseResolution = normalizeVideoResolutionKey(cfg.BaseResolution)
	}

	if len(cfg.ResolutionMultipliers) > 0 {
		normalized.ResolutionMultipliers = make(map[string]float64, len(cfg.ResolutionMultipliers))
		for rawKey, value := range cfg.ResolutionMultipliers {
			key := normalizeVideoResolutionKey(rawKey)
			if key == "" {
				return nil, errors.Errorf("video resolution multiplier key cannot be empty for '%s'", rawKey)
			}
			if value <= 0 {
				return nil, errors.Errorf("video resolution multiplier for %s must be positive", rawKey)
			}
			normalized.ResolutionMultipliers[key] = value
		}
	}

	return normalized, nil
}

func validateVideoPricingLocal(cfg *VideoPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PerSecondUsd < 0 {
		return false, errors.Errorf("video per_second_usd cannot be negative for model %s", modelName)
	}
	for key, value := range cfg.ResolutionMultipliers {
		if strings.TrimSpace(key) == "" {
			return false, errors.Errorf("video resolution multiplier key cannot be empty for model %s", modelName)
		}
		if value <= 0 {
			return false, errors.Errorf("video resolution multiplier for %s must be positive (model %s)", key, modelName)
		}
	}
	return hasVideoPricingData(cfg), nil
}

func hasVideoPricingData(cfg *VideoPricingLocal) bool {
	if cfg == nil {
		return false
	}
	if cfg.PerSecondUsd > 0 {
		return true
	}
	return len(cfg.ResolutionMultipliers) > 0
}

func validateAudioPricingLocal(cfg *AudioPricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PromptRatio < 0 {
		return false, errors.Errorf("audio prompt_ratio cannot be negative for model %s", modelName)
	}
	if cfg.CompletionRatio < 0 {
		return false, errors.Errorf("audio completion_ratio cannot be negative for model %s", modelName)
	}
	if cfg.PromptTokensPerSecond < 0 {
		return false, errors.Errorf("audio prompt_tokens_per_second cannot be negative for model %s", modelName)
	}
	if cfg.CompletionTokensPerSecond < 0 {
		return false, errors.Errorf("audio completion_tokens_per_second cannot be negative for model %s", modelName)
	}
	if cfg.UsdPerSecond < 0 {
		return false, errors.Errorf("audio usd_per_second cannot be negative for model %s", modelName)
	}
	return hasAudioPricingData(cfg), nil
}

func hasAudioPricingData(cfg *AudioPricingLocal) bool {
	if cfg == nil {
		return false
	}
	return cfg.PromptRatio != 0 || cfg.CompletionRatio != 0 || cfg.PromptTokensPerSecond != 0 ||
		cfg.CompletionTokensPerSecond != 0 || cfg.UsdPerSecond != 0
}

func validateImagePricingLocal(cfg *ImagePricingLocal, modelName string) (bool, error) {
	if cfg == nil {
		return false, nil
	}
	if cfg.PricePerImageUsd < 0 {
		return false, errors.Errorf("image price_per_image_usd cannot be negative for model %s", modelName)
	}
	if cfg.PromptRatio < 0 {
		return false, errors.Errorf("image prompt_ratio cannot be negative for model %s", modelName)
	}
	if cfg.PromptTokenLimit < 0 {
		return false, errors.Errorf("image prompt_token_limit cannot be negative for model %s", modelName)
	}
	if cfg.MinImages < 0 {
		return false, errors.Errorf("image min_images cannot be negative for model %s", modelName)
	}
	if cfg.MaxImages < 0 {
		return false, errors.Errorf("image max_images cannot be negative for model %s", modelName)
	}
	if cfg.MinImages > 0 && cfg.MaxImages > 0 && cfg.MinImages > cfg.MaxImages {
		return false, errors.Errorf("image min_images cannot exceed max_images for model %s", modelName)
	}
	if len(cfg.SizeMultipliers) > 0 {
		for key, value := range cfg.SizeMultipliers {
			if strings.TrimSpace(key) == "" {
				return false, errors.Errorf("image size multiplier key cannot be empty for model %s", modelName)
			}
			if value <= 0 {
				return false, errors.Errorf("image size multiplier for %s (%s) must be positive", modelName, key)
			}
		}
	}
	if len(cfg.QualityMultipliers) > 0 {
		for key, value := range cfg.QualityMultipliers {
			if strings.TrimSpace(key) == "" {
				return false, errors.Errorf("image quality multiplier key cannot be empty for model %s", modelName)
			}
			if value <= 0 {
				return false, errors.Errorf("image quality multiplier for %s (%s) must be positive", modelName, key)
			}
		}
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		for quality, sizeMap := range cfg.QualitySizeMultipliers {
			if strings.TrimSpace(quality) == "" {
				return false, errors.Errorf("image quality-size multiplier quality cannot be empty for model %s", modelName)
			}
			for size, value := range sizeMap {
				if strings.TrimSpace(size) == "" {
					return false, errors.Errorf("image quality-size multiplier size cannot be empty for model %s quality %s", modelName, quality)
				}
				if value <= 0 {
					return false, errors.Errorf("image quality-size multiplier for %s (%s/%s) must be positive", modelName, quality, size)
				}
			}
		}
	}
	return hasImagePricingData(cfg), nil
}

func hasImagePricingData(cfg *ImagePricingLocal) bool {
	if cfg == nil {
		return false
	}
	if cfg.PricePerImageUsd > 0 || cfg.PromptRatio > 0 || cfg.PromptTokenLimit > 0 {
		return true
	}
	if cfg.MinImages > 0 || cfg.MaxImages > 0 {
		return true
	}
	return len(cfg.SizeMultipliers) > 0 || len(cfg.QualityMultipliers) > 0 || len(cfg.QualitySizeMultipliers) > 0
}

func normalizeVideoResolutionKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	parts := strings.FieldsFunc(trimmed, func(r rune) bool {
		return r == 'x' || r == '*' || r == '×'
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

func normalizeAudioPricingLocal(cfg *AudioPricingLocal) (*AudioPricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.PromptRatio < 0 {
		return nil, errors.New("audio prompt_ratio cannot be negative")
	}
	if cfg.CompletionRatio < 0 {
		return nil, errors.New("audio completion_ratio cannot be negative")
	}
	if cfg.PromptTokensPerSecond < 0 {
		return nil, errors.New("audio prompt_tokens_per_second cannot be negative")
	}
	if cfg.CompletionTokensPerSecond < 0 {
		return nil, errors.New("audio completion_tokens_per_second cannot be negative")
	}
	if cfg.UsdPerSecond < 0 {
		return nil, errors.New("audio usd_per_second cannot be negative")
	}
	normalized := &AudioPricingLocal{
		PromptRatio:               cfg.PromptRatio,
		CompletionRatio:           cfg.CompletionRatio,
		PromptTokensPerSecond:     cfg.PromptTokensPerSecond,
		CompletionTokensPerSecond: cfg.CompletionTokensPerSecond,
		UsdPerSecond:              cfg.UsdPerSecond,
	}
	return normalized, nil
}

func normalizeImagePricingLocal(cfg *ImagePricingLocal) (*ImagePricingLocal, error) {
	if cfg == nil {
		return nil, nil
	}
	if cfg.PricePerImageUsd < 0 {
		return nil, errors.New("image price_per_image_usd cannot be negative")
	}
	if cfg.PromptRatio < 0 {
		return nil, errors.New("image prompt_ratio cannot be negative")
	}
	if cfg.PromptTokenLimit < 0 {
		return nil, errors.New("image prompt_token_limit cannot be negative")
	}
	if cfg.MinImages < 0 {
		return nil, errors.New("image min_images cannot be negative")
	}
	if cfg.MaxImages < 0 {
		return nil, errors.New("image max_images cannot be negative")
	}
	if cfg.MinImages > 0 && cfg.MaxImages > 0 && cfg.MinImages > cfg.MaxImages {
		return nil, errors.New("image min_images cannot exceed max_images")
	}
	normalized := &ImagePricingLocal{
		PricePerImageUsd: cfg.PricePerImageUsd,
		PromptRatio:      cfg.PromptRatio,
		PromptTokenLimit: cfg.PromptTokenLimit,
		MinImages:        cfg.MinImages,
		MaxImages:        cfg.MaxImages,
	}
	if trimmed := strings.TrimSpace(cfg.DefaultSize); trimmed != "" {
		normalized.DefaultSize = normalizeImageSizeKey(trimmed)
	}
	if trimmedQuality := strings.TrimSpace(cfg.DefaultQuality); trimmedQuality != "" {
		normalized.DefaultQuality = normalizeImageQualityKey(trimmedQuality)
	}
	if len(cfg.SizeMultipliers) > 0 {
		normalized.SizeMultipliers = make(map[string]float64, len(cfg.SizeMultipliers))
		for raw, value := range cfg.SizeMultipliers {
			key := normalizeImageSizeKey(raw)
			if key == "" {
				return nil, errors.Errorf("image size multiplier key cannot be empty (input: %q)", raw)
			}
			if value <= 0 {
				return nil, errors.Errorf("image size multiplier for %s must be positive", raw)
			}
			normalized.SizeMultipliers[key] = value
		}
	}
	if len(cfg.QualityMultipliers) > 0 {
		normalized.QualityMultipliers = make(map[string]float64, len(cfg.QualityMultipliers))
		for raw, value := range cfg.QualityMultipliers {
			key := normalizeImageQualityKey(raw)
			if key == "" {
				return nil, errors.Errorf("image quality multiplier key cannot be empty (input: %q)", raw)
			}
			if value <= 0 {
				return nil, errors.Errorf("image quality multiplier for %s must be positive", raw)
			}
			normalized.QualityMultipliers[key] = value
		}
	}
	if len(cfg.QualitySizeMultipliers) > 0 {
		normalized.QualitySizeMultipliers = make(map[string]map[string]float64, len(cfg.QualitySizeMultipliers))
		for rawQuality, sizeMap := range cfg.QualitySizeMultipliers {
			qualityKey := normalizeImageQualityKey(rawQuality)
			if qualityKey == "" {
				return nil, errors.Errorf("image quality-size multiplier quality cannot be empty (input: %q)", rawQuality)
			}
			if len(sizeMap) == 0 {
				continue
			}
			normalizedSizes := make(map[string]float64, len(sizeMap))
			for rawSize, value := range sizeMap {
				sizeKey := normalizeImageSizeKey(rawSize)
				if sizeKey == "" {
					return nil, errors.Errorf("image quality-size multiplier size cannot be empty (quality: %s)", rawQuality)
				}
				if value <= 0 {
					return nil, errors.Errorf("image quality-size multiplier for %s/%s must be positive", rawQuality, rawSize)
				}
				normalizedSizes[sizeKey] = value
			}
			if len(normalizedSizes) > 0 {
				normalized.QualitySizeMultipliers[qualityKey] = normalizedSizes
			}
		}
	}
	return normalized, nil
}

func normalizeImageSizeKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	trimmed = strings.ReplaceAll(trimmed, "×", "x")
	trimmed = strings.ReplaceAll(trimmed, "*", "x")
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	return trimmed
}

func normalizeImageQualityKey(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}

func migrateLegacyImagePriceInConfigs(rawJSON string) (string, bool, error) {
	trimmed := strings.TrimSpace(rawJSON)
	if trimmed == "" || trimmed == "{}" {
		return rawJSON, false, nil
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		return "", false, errors.Wrap(err, "decode legacy image pricing map")
	}
	changed := false
	for modelName, rawCfg := range payload {
		cfgMap, ok := rawCfg.(map[string]any)
		if !ok || cfgMap == nil {
			continue
		}
		legacyValue, hasLegacy := cfgMap["image_price_usd"]
		if !hasLegacy {
			continue
		}
		price, ok := floatFromAny(legacyValue)
		delete(cfgMap, "image_price_usd")
		if !ok || price <= 0 {
			payload[modelName] = cfgMap
			changed = true
			continue
		}
		imageValue, hasImage := cfgMap["image"]
		var imageMap map[string]any
		if hasImage {
			imageMap, _ = imageValue.(map[string]any)
		}
		if imageMap == nil {
			imageMap = make(map[string]any)
		}
		if existing, exists := imageMap["price_per_image_usd"]; !exists {
			imageMap["price_per_image_usd"] = price
		} else if existingPrice, ok := floatFromAny(existing); ok && existingPrice <= 0 {
			imageMap["price_per_image_usd"] = price
		}
		cfgMap["image"] = imageMap
		payload[modelName] = cfgMap
		changed = true
	}
	if !changed {
		return rawJSON, false, nil
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return "", false, errors.Wrap(err, "encode normalized image pricing")
	}
	return string(normalized), true, nil
}

func floatFromAny(value any) (float64, bool) {
	switch v := value.(type) {
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case uint32:
		return float64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// GetModelPriceConfigs returns the channel-specific model price configurations in the new unified format
func (channel *Channel) GetModelPriceConfigs() map[string]ModelConfigLocal {
	if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
		return nil
	}

	modelPriceConfigs := make(map[string]ModelConfigLocal)
	err := json.Unmarshal([]byte(*channel.ModelConfigs), &modelPriceConfigs)
	if err != nil {
		logger.Logger.Error("failed to unmarshal model price configs for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}

	return modelPriceConfigs
}

// SetModelPriceConfigs sets the channel-specific model price configurations in the new unified format
func (channel *Channel) SetModelPriceConfigs(modelPriceConfigs map[string]ModelConfigLocal) error {
	if len(modelPriceConfigs) == 0 {
		channel.ModelConfigs = nil
		return nil
	}

	cleaned := make(map[string]ModelConfigLocal, len(modelPriceConfigs))
	for rawName, cfg := range modelPriceConfigs {
		trimmedName := strings.TrimSpace(rawName)
		if trimmedName == "" {
			return errors.New("model name cannot be empty")
		}
		normalized, err := normalizeModelConfigLocal(cfg)
		if err != nil {
			return errors.Wrapf(err, "normalize model config for %s", rawName)
		}
		cleaned[trimmedName] = normalized
	}

	// Validate the configurations before setting
	if err := channel.validateModelPriceConfigs(cleaned); err != nil {
		return errors.Wrap(err, "invalid model price configurations")
	}

	jsonBytes, err := json.Marshal(cleaned)
	if err != nil {
		return errors.Wrap(err, "failed to marshal model price configurations")
	}

	jsonStr := string(jsonBytes)
	channel.ModelConfigs = &jsonStr
	return nil
}

// GetToolingConfig returns the channel-level tooling policy configuration, if any.
func (channel *Channel) GetToolingConfig() *ChannelToolingConfig {
	cfg, err := channel.LoadConfig()
	if err != nil {
		logger.Logger.Error("failed to load channel config for tooling",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return cloneChannelToolingConfig(cfg.Tooling)
}

// SetToolingConfig updates the channel-level tooling configuration stored in the config JSON blob.
func (channel *Channel) SetToolingConfig(tooling *ChannelToolingConfig) error {
	normalized, err := normalizeChannelToolingConfig(tooling)
	if err != nil {
		return errors.Wrap(err, "invalid tooling configuration")
	}
	cfg, err := channel.LoadConfig()
	if err != nil {
		return err
	}
	cfg.Tooling = cloneChannelToolingConfig(normalized)
	return channel.storeConfig(cfg)
}

// GetModelPriceConfig returns the price configuration for a specific model
func (channel *Channel) GetModelPriceConfig(modelName string) *ModelConfigLocal {
	configs := channel.GetModelPriceConfigs()
	if configs == nil {
		return nil
	}

	if config, exists := configs[modelName]; exists {
		return &config
	}

	return nil
}

// GetModelRatioFromConfigs extracts model ratios from the unified ModelConfigs
func (channel *Channel) GetModelRatioFromConfigs() map[string]float64 {
	configs := channel.GetModelPriceConfigs()
	if configs == nil {
		return nil
	}

	modelRatios := make(map[string]float64)
	for modelName, config := range configs {
		if config.Ratio != 0 {
			modelRatios[modelName] = config.Ratio
		}
	}

	if len(modelRatios) == 0 {
		return nil
	}

	return modelRatios
}

// GetCompletionRatioFromConfigs extracts completion ratios from the unified ModelConfigs
func (channel *Channel) GetCompletionRatioFromConfigs() map[string]float64 {
	configs := channel.GetModelPriceConfigs()
	if configs == nil {
		return nil
	}

	completionRatios := make(map[string]float64)
	for modelName, config := range configs {
		if config.CompletionRatio != 0 {
			completionRatios[modelName] = config.CompletionRatio
		}
	}

	if len(completionRatios) == 0 {
		return nil
	}

	return completionRatios
}

func (channel *Channel) GetInferenceProfileArnMap() map[string]string {
	if channel.InferenceProfileArnMap == nil || *channel.InferenceProfileArnMap == "" || *channel.InferenceProfileArnMap == "{}" {
		return nil
	}
	arnMap := make(map[string]string)
	err := json.Unmarshal([]byte(*channel.InferenceProfileArnMap), &arnMap)
	if err != nil {
		logger.Logger.Error("failed to unmarshal inference profile ARN map for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return arnMap
}

func (channel *Channel) SetInferenceProfileArnMap(arnMap map[string]string) error {
	if len(arnMap) == 0 {
		channel.InferenceProfileArnMap = nil
		return nil
	}

	// Validate that keys and values are not empty
	for key, value := range arnMap {
		if key == "" || value == "" {
			return errors.New("inference profile ARN map cannot contain empty keys or values")
		}
	}

	jsonBytes, err := json.Marshal(arnMap)
	if err != nil {
		return errors.Wrap(err, "marshal inference profile ARN map")
	}
	jsonStr := string(jsonBytes)
	channel.InferenceProfileArnMap = &jsonStr
	return nil
}

// ValidateInferenceProfileArnMapJSON validates a JSON string for inference profile ARN mapping
func ValidateInferenceProfileArnMapJSON(jsonStr string) error {
	if jsonStr == "" {
		return nil // Empty is allowed
	}

	var arnMap map[string]string
	err := json.Unmarshal([]byte(jsonStr), &arnMap)
	if err != nil {
		return errors.Errorf("invalid JSON format: %v", err)
	}

	// Validate that keys and values are not empty
	for key, value := range arnMap {
		if key == "" {
			return errors.New("inference profile ARN map cannot contain empty keys")
		}
		if value == "" {
			return errors.New("inference profile ARN map cannot contain empty values")
		}
	}

	return nil
}

func (channel *Channel) Insert() error {
	err := DB.Create(channel).Error
	if err != nil {
		return errors.Wrapf(err, "failed to insert channel: name=%s, type=%d", channel.Name, channel.Type)
	}
	err = channel.AddAbilities()
	if err != nil {
		return errors.Wrapf(err, "failed to add abilities for channel: id=%d, name=%s", channel.Id, channel.Name)
	}
	InitChannelCache()
	return nil
}

func (channel *Channel) Update() error {
	// Validate/sync TestingModel with latest supported models
	clearTestingModel := false
	var existing Channel
	if channel.Id != 0 {
		_ = DB.Select("id", "models", "testing_model").First(&existing, "id = ?", channel.Id).Error
	}
	// Determine models to validate against: new value if provided, else existing
	modelsForValidation := channel.Models
	if strings.TrimSpace(modelsForValidation) == "" {
		modelsForValidation = existing.Models
	}
	// Helper to check containment
	contains := func(listCSV, name string) bool {
		for n := range strings.SplitSeq(listCSV, ",") {
			if strings.TrimSpace(n) == name {
				return true
			}
		}
		return false
	}
	if channel.TestingModel != nil {
		tm := strings.TrimSpace(*channel.TestingModel)
		if tm == "" {
			clearTestingModel = true
			channel.TestingModel = nil
		} else if !contains(modelsForValidation, tm) {
			// requested value not supported by current models
			clearTestingModel = true
			channel.TestingModel = nil
		}
	} else if existing.TestingModel != nil && *existing.TestingModel != "" {
		// No explicit testing_model provided in payload, but existing one may become invalid due to models change
		if !contains(modelsForValidation, *existing.TestingModel) {
			clearTestingModel = true
		}
	}

	err := DB.Model(channel).Updates(channel).Error
	if err != nil {
		return errors.Wrapf(err, "failed to update channel: id=%d, name=%s", channel.Id, channel.Name)
	}
	DB.Model(channel).First(channel, "id = ?", channel.Id)
	if clearTestingModel {
		if err := DB.Model(channel).Where("id = ?", channel.Id).Update("testing_model", nil).Error; err != nil {
			return errors.Wrapf(err, "failed to clear testing_model for channel: id=%d", channel.Id)
		}
		// refresh field after manual clear
		channel.TestingModel = nil
	}
	err = channel.UpdateAbilities()
	if err != nil {
		return errors.Wrapf(err, "failed to update abilities for channel: id=%d, name=%s", channel.Id, channel.Name)
	}
	InitChannelCache()
	return nil
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     helper.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		logger.Logger.Error("failed to update response time", zap.Error(err))
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: helper.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		logger.Logger.Error("failed to update balance", zap.Error(err))
	}
}

func (channel *Channel) Delete() error {
	if err := DB.Delete(channel).Error; err != nil {
		return errors.Wrapf(err, "delete channel %d", channel.Id)
	}
	if err := channel.DeleteAbilities(); err != nil {
		return errors.Wrapf(err, "delete abilities for channel %d", channel.Id)
	}
	InitChannelCache()
	return nil
}

func (channel *Channel) LoadConfig() (ChannelConfig, error) {
	var cfg ChannelConfig
	if channel.Config == "" {
		return cfg, nil
	}
	err := json.Unmarshal([]byte(channel.Config), &cfg)
	if err != nil {
		return cfg, errors.Wrapf(err, "unmarshal channel %d config", channel.Id)
	}
	return cfg, nil
}

// GetSupportedEndpoints returns the effective supported endpoints for this channel.
// If the channel has custom endpoints configured, those are returned.
// Otherwise, the default endpoints for the channel type are returned.
func (channel *Channel) GetSupportedEndpoints() []string {
	cfg, err := channel.LoadConfig()
	if err != nil {
		logger.Logger.Error("failed to load channel config for endpoints",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	if len(cfg.SupportedEndpoints) > 0 {
		return cfg.SupportedEndpoints
	}
	// Return nil to indicate default endpoints should be used
	// The caller should use channeltype.DefaultEndpointNamesForChannelType(channel.Type)
	return nil
}

// storeConfig persists the provided ChannelConfig into the serialized Config
// column, clearing the field when the configuration is effectively empty.
func (channel *Channel) storeConfig(cfg ChannelConfig) error {
	data, err := json.Marshal(cfg)
	if err != nil {
		return errors.Wrapf(err, "marshal channel %d config", channel.Id)
	}
	if len(data) == 0 || string(data) == "{}" {
		channel.Config = ""
		return nil
	}
	channel.Config = string(data)
	return nil
}

// GetModelRatio returns the channel-specific model ratio map
// DEPRECATED: Use GetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) GetModelRatio() map[string]float64 {
	if channel.ModelRatio == nil || *channel.ModelRatio == "" || *channel.ModelRatio == "{}" {
		return nil
	}
	modelRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(*channel.ModelRatio), &modelRatio)
	if err != nil {
		logger.Logger.Error("failed to unmarshal model ratio for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return modelRatio
}

// GetCompletionRatio returns the channel-specific completion ratio map
// DEPRECATED: Use GetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) GetCompletionRatio() map[string]float64 {
	if channel.CompletionRatio == nil || *channel.CompletionRatio == "" || *channel.CompletionRatio == "{}" {
		return nil
	}
	completionRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(*channel.CompletionRatio), &completionRatio)
	if err != nil {
		logger.Logger.Error("failed to unmarshal completion ratio for channel",
			zap.Int("channel_id", channel.Id),
			zap.Error(err))
		return nil
	}
	return completionRatio
}

// SetModelRatio sets the channel-specific model ratio map
// DEPRECATED: Use SetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) SetModelRatio(modelRatio map[string]float64) error {
	if len(modelRatio) == 0 {
		channel.ModelRatio = nil
		return nil
	}
	jsonBytes, err := json.Marshal(modelRatio)
	if err != nil {
		return errors.Wrap(err, "marshal channel model ratio")
	}
	jsonStr := string(jsonBytes)
	channel.ModelRatio = &jsonStr
	return nil
}

// SetCompletionRatio sets the channel-specific completion ratio map
// DEPRECATED: Use SetModelPriceConfigs() instead. This method is kept for backward compatibility.
func (channel *Channel) SetCompletionRatio(completionRatio map[string]float64) error {
	if len(completionRatio) == 0 {
		channel.CompletionRatio = nil
		return nil
	}
	jsonBytes, err := json.Marshal(completionRatio)
	if err != nil {
		return errors.Wrap(err, "marshal channel completion ratio")
	}
	jsonStr := string(jsonBytes)
	channel.CompletionRatio = &jsonStr
	return nil
}

func UpdateChannelStatusById(id int, status int) {
	err := UpdateAbilityStatus(id, status == ChannelStatusEnabled)
	if err != nil {
		logger.Logger.Error("failed to update ability status", zap.Error(err))
	}
	err = DB.Model(&Channel{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		logger.Logger.Error("failed to update channel status", zap.Error(err))
	}
	if err == nil {
		InitChannelCache()
	}
}

func UpdateChannelUsedQuota(id int, quota int64) {
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		return
	}
	updateChannelUsedQuota(id, quota)
}

func updateChannelUsedQuota(id int, quota int64) {
	err := DB.Model(&Channel{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error
	if err != nil {
		logger.Logger.Error("failed to update channel used quota - channel statistics may be inaccurate",
			zap.Error(err),
			zap.Int("channelId", id),
			zap.Int64("quota", quota),
			zap.String("note", "billing completed successfully but channel usage statistics update failed"))
	}
}

func DeleteChannelByStatus(status int64) (int64, error) {
	result := DB.Where("status = ?", status).Delete(&Channel{})
	if result.Error == nil {
		InitChannelCache()
	}
	return result.RowsAffected, result.Error
}

func DeleteDisabledChannel() (int64, error) {
	result := DB.Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Delete(&Channel{})
	if result.Error == nil {
		InitChannelCache()
	}
	return result.RowsAffected, result.Error
}

// MigrateHistoricalPricingToModelConfigs migrates historical ModelRatio and CompletionRatio data
// into the new unified ModelConfigs format for a single channel
func (channel *Channel) MigrateHistoricalPricingToModelConfigs() error {
	// Validate channel
	if channel == nil {
		return errors.New("channel is nil")
	}

	// Get existing ModelRatio and CompletionRatio data with validation
	var modelRatios map[string]float64
	var completionRatios map[string]float64
	var migrationErrors []string

	// Safely get ModelRatio
	if channel.ModelRatio != nil && *channel.ModelRatio != "" && *channel.ModelRatio != "{}" {
		if err := json.Unmarshal([]byte(*channel.ModelRatio), &modelRatios); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("invalid ModelRatio JSON: %s", err.Error()))
		} else {
			// Validate ModelRatio values
			for modelName, ratio := range modelRatios {
				if modelName == "" {
					migrationErrors = append(migrationErrors, "empty model name in ModelRatio")
				}
				if ratio < 0 {
					migrationErrors = append(migrationErrors, fmt.Sprintf("negative ratio for model %s: %f", modelName, ratio))
				}
			}
		}
	}

	// Safely get CompletionRatio
	if channel.CompletionRatio != nil && *channel.CompletionRatio != "" && *channel.CompletionRatio != "{}" {
		if err := json.Unmarshal([]byte(*channel.CompletionRatio), &completionRatios); err != nil {
			migrationErrors = append(migrationErrors, fmt.Sprintf("invalid CompletionRatio JSON: %s", err.Error()))
		} else {
			// Validate CompletionRatio values
			for modelName, ratio := range completionRatios {
				if modelName == "" {
					migrationErrors = append(migrationErrors, "empty model name in CompletionRatio")
				}
				if ratio < 0 {
					migrationErrors = append(migrationErrors, fmt.Sprintf("negative completion ratio for model %s: %f", modelName, ratio))
				}
			}
		}
	}

	// Report validation errors but continue with valid data
	if len(migrationErrors) > 0 {
		logger.Logger.Error("Channel has validation errors in historical data",
			zap.Int("channel_id", channel.Id),
			zap.Any("errors", migrationErrors))
		// Don't return error - continue with valid data
	}

	// Skip if no valid historical data to migrate
	if len(modelRatios) == 0 && len(completionRatios) == 0 {
		return nil
	}

	// Check if ModelConfigs already has unified data
	existingConfigs := channel.GetModelPriceConfigs()
	if len(existingConfigs) > 0 {
		// Check if existing configs have pricing data (not just MaxTokens)
		hasPricingData := false
		for _, config := range existingConfigs {
			if config.Ratio != 0 || config.CompletionRatio != 0 {
				hasPricingData = true
				break
			}
		}

		if hasPricingData {
			logger.Logger.Info("Channel already has pricing data in ModelConfigs, skipping historical migration",
				zap.Int("channel_id", channel.Id))
			return nil
		}

		// Merge historical pricing with existing MaxTokens data
		logger.Logger.Info("Channel has MaxTokens data, merging with historical pricing",
			zap.Int("channel_id", channel.Id))
	} else {
		existingConfigs = make(map[string]ModelConfigLocal)
	}

	// Collect all valid model names from both ratios and existing configs
	allModelNames := make(map[string]bool)
	for modelName, ratio := range modelRatios {
		// Skip invalid entries
		if modelName != "" && ratio >= 0 {
			allModelNames[modelName] = true
		}
	}
	for modelName, ratio := range completionRatios {
		// Skip invalid entries
		if modelName != "" && ratio >= 0 {
			allModelNames[modelName] = true
		}
	}
	for modelName := range existingConfigs {
		if modelName != "" {
			allModelNames[modelName] = true
		}
	}

	// Create unified ModelConfigs from all data sources
	modelConfigs := make(map[string]ModelConfigLocal)
	for modelName := range allModelNames {
		config := ModelConfigLocal{}

		// Start with existing config if available
		if existingConfig, exists := existingConfigs[modelName]; exists {
			config = existingConfig
		}

		// Add/override pricing data from historical sources (only valid data)
		if modelRatios != nil {
			if ratio, exists := modelRatios[modelName]; exists && ratio >= 0 {
				config.Ratio = ratio
			}
		}

		if completionRatios != nil {
			if completionRatio, exists := completionRatios[modelName]; exists && completionRatio >= 0 {
				config.CompletionRatio = completionRatio
			}
		}

		// Add if we have any data (pricing or MaxTokens)
		if config.Ratio != 0 || config.CompletionRatio != 0 || config.MaxTokens != 0 {
			modelConfigs[modelName] = config
		}
	}

	// Save the migrated data to ModelConfigs
	if len(modelConfigs) > 0 {
		// Log the models being migrated for debugging
		var modelNames []string
		for modelName := range modelConfigs {
			modelNames = append(modelNames, modelName)
		}
		logger.Logger.Info("Channel migrating models",
			zap.Int("channel_id", channel.Id),
			zap.Int("type", channel.Type),
			zap.Strings("models", modelNames))

		err := channel.SetModelPriceConfigs(modelConfigs)
		if err != nil {
			logger.Logger.Error("Failed to set migrated ModelConfigs for channel",
				zap.Int("channel_id", channel.Id),
				zap.Error(err))
			return errors.Wrapf(err, "set migrated model configs for channel %d", channel.Id)
		}

		logger.Logger.Info("Successfully migrated historical pricing data to ModelConfigs",
			zap.Int("channel_id", channel.Id),
			zap.Int("model_count", len(modelConfigs)))
	}

	return nil
}

// MigrateChannelFieldsToText migrates ModelConfigs and ModelMapping fields from varchar(1024) to text type.
//
// Background:
// The original varchar(1024) length was insufficient for complex model configurations, especially when:
// - Multiple models are configured with detailed pricing information (ratio, completion_ratio, max_tokens)
// - Long model names or complex mapping values are used
// - Channel-specific configurations grow beyond the 1024 character limit
//
// This migration is essential because:
// 1. Modern AI models have longer names and more complex configurations
// 2. Users need to configure pricing for dozens of models per channel
// 3. JSON serialization of comprehensive model configs easily exceeds 1024 chars
// 4. Truncated configurations lead to data loss and system errors
//
// The migration is designed to be:
// - Idempotent: Can be run multiple times safely
// - Database-agnostic: Supports MySQL, PostgreSQL, and SQLite
// - Data-preserving: All existing data is maintained during the migration
// - Transaction-safe: Uses database transactions to ensure data integrity
//
// This function should be called during application startup before any channel operations.
func MigrateChannelFieldsToText() error {
	// Ensure only executed once even if called from multiple goroutines
	var runErr error
	channelFieldMigrationOnce.Do(func() {
		logger.Logger.Info("Starting migration of ModelConfigs and ModelMapping fields to TEXT type")

		// Skip if we already migrated in this process
		if channelFieldMigrated.Load() {
			logger.Logger.Info("Channel field migration already completed in this process - skipping")
			return
		}

		needsMigration, err := checkIfFieldMigrationNeeded()
		if err != nil {
			runErr = errors.Wrap(err, "failed to check migration status")
			return
		}

		if !needsMigration {
			logger.Logger.Info("ModelConfigs and ModelMapping fields are already TEXT type - no migration needed")
			channelFieldMigrated.Store(true)
			return
		}

		logger.Logger.Info("Column type migration required - proceeding with migration")
		runErr = performFieldMigration()
	})
	return runErr
}

// performFieldMigration executes the actual database schema changes to migrate fields to TEXT type.
// This function uses database transactions to ensure data integrity and provides detailed error handling.
func performFieldMigration() error {
	// Use transaction for data integrity - ensures all-or-nothing migration
	tx := DB.Begin()
	if tx.Error != nil {
		return errors.Wrap(tx.Error, "failed to start transaction")
	}

	// Ensure transaction is properly handled in case of panic or error
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			logger.Logger.Error("Column migration panicked, rolled back",
				zap.Any("panic", r))
		}
	}()

	// Perform database-specific column type changes
	var err error
	if common.UsingMySQL.Load() {
		err = performMySQLFieldMigration(tx)
	} else if common.UsingPostgreSQL.Load() {
		err = performPostgreSQLFieldMigration(tx)
	} else {
		// This should not happen due to the check in checkIfFieldMigrationNeeded,
		// but we handle it for safety
		tx.Rollback()
		return errors.New("unsupported database type for field migration")
	}

	if err != nil {
		tx.Rollback()
		return errors.Wrap(err, "perform field migration")
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return errors.Wrap(err, "failed to commit migration")
	}

	logger.Logger.Info("Successfully migrated ModelConfigs and ModelMapping columns to TEXT type")
	return nil
}

// performMySQLFieldMigration performs the MySQL-specific column type migration.
func performMySQLFieldMigration(tx *gorm.DB) error {
	logger.Logger.Info("Performing MySQL field migration")

	// MySQL: Use MODIFY COLUMN to change type while preserving data.
	// Do NOT set DEFAULT '' on TEXT columns (not allowed for TEXT/BLOB in MySQL).
	err := tx.Exec("ALTER TABLE channels MODIFY COLUMN model_configs TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_configs column")
	}

	err = tx.Exec("ALTER TABLE channels MODIFY COLUMN model_mapping TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_mapping column")
	}

	channelFieldMigrated.Store(true)
	logger.Logger.Info("MySQL field migration completed successfully")
	return nil
}

// performPostgreSQLFieldMigration performs the PostgreSQL-specific column type migration.
func performPostgreSQLFieldMigration(tx *gorm.DB) error {
	logger.Logger.Info("Performing PostgreSQL field migration")

	// PostgreSQL: Use ALTER COLUMN TYPE to change column type
	err := tx.Exec("ALTER TABLE channels ALTER COLUMN model_configs TYPE TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_configs column")
	}

	err = tx.Exec("ALTER TABLE channels ALTER COLUMN model_mapping TYPE TEXT").Error
	if err != nil {
		return errors.Wrap(err, "failed to migrate model_mapping column")
	}

	channelFieldMigrated.Store(true)
	logger.Logger.Info("PostgreSQL field migration completed successfully")
	return nil
}

// checkIfFieldMigrationNeeded checks if ModelConfigs and ModelMapping fields need to be migrated to TEXT type.
// This function provides idempotency by checking the current column types in the database.
// Returns true if migration is needed, false if fields are already TEXT type.
func checkIfFieldMigrationNeeded() (bool, error) {
	if common.UsingMySQL.Load() {
		// First check if the channels table exists at all
		var tableExists int
		err := DB.Raw(`SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels'`).
			Scan(&tableExists).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check if channels table exists in MySQL")
		}

		// If table doesn't exist, no migration needed - AutoMigrate will create it correctly
		if tableExists == 0 {
			logger.Logger.Info("Channels table does not exist - no field migration needed")
			return false, nil
		}

		// Check MySQL column types for both fields
		var modelConfigsType, modelMappingType string

		// Check model_configs column type
		err = DB.Raw(`SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels' AND COLUMN_NAME = 'model_configs'`).
			Scan(&modelConfigsType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_configs column type in MySQL")
		}

		// Check model_mapping column type
		err = DB.Raw(`SELECT DATA_TYPE FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'channels' AND COLUMN_NAME = 'model_mapping'`).
			Scan(&modelMappingType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_mapping column type in MySQL")
		}

		logger.Logger.Info("Detected MySQL column types for migration check",
			zap.String("model_configs_type", modelConfigsType),
			zap.String("model_mapping_type", modelMappingType))

		// If columns don't exist, no migration needed - AutoMigrate will create them correctly
		if modelConfigsType == "" || modelMappingType == "" {
			logger.Logger.Info("One or more columns do not exist - no field migration needed")
			return false, nil
		}

		// Migration needed unless both columns already some kind of TEXT.* (varchar is insufficient)
		isTextType := func(tp string) bool { return strings.Contains(tp, "text") }
		need := !(isTextType(modelConfigsType) && isTextType(modelMappingType))
		return need, nil

	} else if common.UsingPostgreSQL.Load() {
		// First check if the channels table exists at all
		var tableExists int
		err := DB.Raw(`SELECT COUNT(*) FROM information_schema.tables
			WHERE table_name = 'channels'`).
			Scan(&tableExists).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check if channels table exists in PostgreSQL")
		}

		// If table doesn't exist, no migration needed - AutoMigrate will create it correctly
		if tableExists == 0 {
			logger.Logger.Info("Channels table does not exist - no field migration needed")
			return false, nil
		}

		// Check PostgreSQL column types for both fields
		var modelConfigsType, modelMappingType string

		// Check model_configs column type
		err = DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = 'channels' AND column_name = 'model_configs'`).
			Scan(&modelConfigsType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_configs column type in PostgreSQL")
		}

		// Check model_mapping column type
		err = DB.Raw(`SELECT data_type FROM information_schema.columns
			WHERE table_name = 'channels' AND column_name = 'model_mapping'`).
			Scan(&modelMappingType).Error
		if err != nil {
			return false, errors.Wrap(err, "failed to check model_mapping column type in PostgreSQL")
		}

		// If columns don't exist, no migration needed - AutoMigrate will create them correctly
		if modelConfigsType == "" || modelMappingType == "" {
			logger.Logger.Info("One or more columns do not exist - no field migration needed")
			return false, nil
		}

		// Migration needed if either field is still character varying (varchar)
		return modelConfigsType == "character varying" || modelMappingType == "character varying", nil

	} else if common.UsingSQLite.Load() {
		// SQLite is flexible with column types and doesn't enforce strict typing
		// TEXT and VARCHAR are treated the same way, so no migration is needed
		logger.Logger.Info("SQLite detected - column type migration not required (SQLite is flexible with text types)")
		return false, nil

	} else {
		// Unknown database type - assume no migration needed to be safe
		logger.Logger.Info("Unknown database type detected - skipping column type migration")
		return false, nil
	}
}

// MigrateAllChannelModelConfigs migrates all channels' ModelConfigs from old format to new format
// and also migrates historical ModelRatio/CompletionRatio data to the new unified format
// This should be called during application startup to handle existing data
func MigrateAllChannelModelConfigs() error {
	logger.Logger.Info("Starting migration of all channel ModelConfigs and historical pricing data")

	var channels []*Channel
	err := DB.Find(&channels).Error
	if err != nil {
		return errors.Wrap(err, "failed to fetch channels")
	}

	if len(channels) == 0 {
		logger.Logger.Info("No channels found for migration")
		return nil
	}

	migratedCount := 0
	historicalMigratedCount := 0
	errorCount := 0
	var migrationErrors []string

	for _, channel := range channels {
		channelUpdated := false
		originalModelConfigs := ""
		if channel.ModelConfigs != nil {
			originalModelConfigs = *channel.ModelConfigs
		}

		// First, migrate existing ModelConfigs from old format to new format (PR format -> unified format)
		if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
			err := channel.MigrateModelConfigsToModelPrice()
			if err != nil {
				logger.Logger.Error("Failed to migrate ModelConfigs for channel",
					zap.Int("channel_id", channel.Id),
					zap.Error(err))
				errorMsg := getMigrationErrorContext(err, channel.Id, "ModelConfigs format migration")
				migrationErrors = append(migrationErrors, errorMsg)
				errorCount++
				continue
			}
			channelUpdated = true
			migratedCount++
		}

		// Second, migrate historical ModelRatio/CompletionRatio data to ModelConfigs
		err := channel.MigrateHistoricalPricingToModelConfigs()
		if err != nil {
			logger.Logger.Error("Failed to migrate historical pricing for channel",
				zap.Int("channel_id", channel.Id),
				zap.Error(err))
			errorMsg := getMigrationErrorContext(err, channel.Id, "historical pricing migration")
			migrationErrors = append(migrationErrors, errorMsg)
			errorCount++
			continue
		}

		// Check if historical migration actually created ModelConfigs data
		if channel.ModelConfigs != nil && *channel.ModelConfigs != "" && *channel.ModelConfigs != "{}" {
			if !channelUpdated { // Only count if it wasn't already counted in the first migration
				historicalMigratedCount++
				channelUpdated = true
			}
		}

		// Save the migrated channel back to database if any changes were made
		if channelUpdated {
			// Validate the final result before saving
			finalConfigs := channel.GetModelPriceConfigs()
			if err := channel.validateModelPriceConfigs(finalConfigs); err != nil {
				logger.Logger.Error("Migration validation failed for channel",
					zap.Int("channel_id", channel.Id),
					zap.Error(err))
				errorMsg := getMigrationErrorContext(err, channel.Id, "validation")
				migrationErrors = append(migrationErrors, errorMsg)
				errorCount++
				// Restore original data
				if originalModelConfigs != "" {
					channel.ModelConfigs = &originalModelConfigs
				} else {
					channel.ModelConfigs = nil
				}
				continue
			}

			saveErr := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error
			if saveErr != nil {
				// Detect MySQL column size overflow and attempt on-the-fly migration+retry
				if common.UsingMySQL.Load() && isMySQLDataTooLongErr(saveErr) {
					logger.Logger.Warn("Detected model_configs length overflow, attempting column type migration to TEXT and retry",
						zap.Int("channel_id", channel.Id))
					if migErr := performMySQLFieldMigration(DB); migErr != nil {
						logger.Logger.Error("On-demand MySQL column migration failed",
							zap.Int("channel_id", channel.Id),
							zap.Error(migErr))
						errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d after overflow & migration attempt: %s", channel.Id, saveErr.Error())
						migrationErrors = append(migrationErrors, errorMsg)
						errorCount++
						continue
					}
					// Retry save after migration
					if retryErr := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error; retryErr != nil {
						logger.Logger.Error("Retry save after column migration still failed",
							zap.Int("channel_id", channel.Id),
							zap.Error(retryErr))
						errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d after retry: %s", channel.Id, retryErr.Error())
						migrationErrors = append(migrationErrors, errorMsg)
						errorCount++
						continue
					}
					logger.Logger.Info("Retry save after on-demand column migration succeeded",
						zap.Int("channel_id", channel.Id))
				} else {
					logger.Logger.Error("Failed to save migrated ModelConfigs for channel",
						zap.Int("channel_id", channel.Id),
						zap.Error(saveErr))
					errorMsg := fmt.Sprintf("Failed to save migrated ModelConfigs for channel %d: %s", channel.Id, saveErr.Error())
					migrationErrors = append(migrationErrors, errorMsg)
					errorCount++
					continue
				}
			}
		}
	}

	// If more than 50% of channels failed, return error to prevent silent data loss
	if len(channels) > 0 {
		failureRate := float64(errorCount) / float64(len(channels))
		if failureRate > 0.5 {
			return errors.Errorf("migration failed for %d/%d channels (%.1f%%)",
				errorCount, len(channels), failureRate*100)
		}
	}

	// Log final results
	if migratedCount > 0 {
		logger.Logger.Info("Successfully migrated ModelConfigs format", zap.Int("migrated_count", migratedCount))
	}
	if historicalMigratedCount > 0 {
		logger.Logger.Info("Successfully migrated historical pricing data", zap.Int("historical_migrated_count", historicalMigratedCount))
	}
	if errorCount > 0 {
		logger.Logger.Error("Migration completed with errors", zap.Int("error_count", errorCount))
		for _, errMsg := range migrationErrors {
			logger.Logger.Error("Migration error", zap.String("error", errMsg))
		}
	}
	if migratedCount == 0 && historicalMigratedCount == 0 && errorCount == 0 {
		logger.Logger.Info("No channels required data migration")
	}

	return nil
}

// MigrateChannelLegacyImagePricing normalizes legacy per-image pricing fields into Image configs.
func MigrateChannelLegacyImagePricing() error {
	logger.Logger.Info("Starting migration of legacy channel image pricing")
	var channels []*Channel
	if err := DB.Find(&channels).Error; err != nil {
		return errors.Wrap(err, "fetch channels for image pricing migration")
	}
	migrated := 0
	for _, channel := range channels {
		if channel.ModelConfigs == nil || *channel.ModelConfigs == "" || *channel.ModelConfigs == "{}" {
			continue
		}
		updated, changed, err := migrateLegacyImagePriceInConfigs(*channel.ModelConfigs)
		if err != nil {
			logger.Logger.Error("failed to normalize image pricing for channel",
				zap.Int("channel_id", channel.Id),
				zap.Error(err))
			continue
		}
		if !changed {
			continue
		}
		original := *channel.ModelConfigs
		channel.ModelConfigs = &updated
		configs := channel.GetModelPriceConfigs()
		if err := channel.validateModelPriceConfigs(configs); err != nil {
			logger.Logger.Error("validated migrated image pricing failed",
				zap.Int("channel_id", channel.Id),
				zap.Error(err))
			channel.ModelConfigs = &original
			continue
		}
		if err := DB.Model(channel).Update("model_configs", channel.ModelConfigs).Error; err != nil {
			logger.Logger.Error("failed to persist migrated image pricing",
				zap.Int("channel_id", channel.Id),
				zap.Error(err))
			channel.ModelConfigs = &original
			continue
		}
		migrated++
	}
	if migrated > 0 {
		logger.Logger.Info("Legacy channel image pricing migration completed", zap.Int("channels_migrated", migrated))
	} else {
		logger.Logger.Info("No legacy channel image pricing entries required migration")
	}
	return nil
}

// getMigrationErrorContext provides additional context for migration errors
func getMigrationErrorContext(err error, channelID int, operation string) string {
	if err == nil {
		return ""
	}

	context := fmt.Sprintf("Channel %d %s failed", channelID, operation)

	// Add specific guidance for common errors
	errStr := err.Error()
	switch {
	case strings.Contains(errStr, "Data too long"):
		context += " - Column size insufficient, consider running field migration"
	case strings.Contains(errStr, "invalid character"):
		context += " - Invalid JSON format in configuration data"
	case strings.Contains(errStr, "connection"):
		context += " - Database connection issue, check connectivity"
	case strings.Contains(errStr, "syntax error"):
		context += " - SQL syntax error, check database compatibility"
	case strings.Contains(errStr, "duplicate"):
		context += " - Duplicate key constraint violation"
	default:
		context += fmt.Sprintf(" - %s", err.Error())
	}

	return context
}
