package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"
	"gorm.io/gorm"

	"github.com/Laisky/one-api/common/config"
	"github.com/Laisky/one-api/common/helper"
	"github.com/Laisky/one-api/common/identity"
	"github.com/Laisky/one-api/common/logger"
)

const (
	ChannelStatusUnknown          = 0
	ChannelStatusEnabled          = 1 // don't use 0, 0 is the default value!
	ChannelStatusManuallyDisabled = 2 // also don't use 0
	ChannelStatusAutoDisabled     = 3
)

type Channel struct {
	Id                 int     `json:"id"`
	UUID               string  `json:"uuid" gorm:"type:char(36);column:uuid"`
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
	HiddenModels       *string `json:"hidden_models" gorm:"type:text"`
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
	InferenceProfileArnMap *string         `json:"inference_profile_arn_map" gorm:"type:text"` // JSON string mapping model names to AWS Bedrock Inference Profile ARNs
	HiddenModelsProvided   bool            `json:"-" gorm:"-"`
	NullableFieldsProvided map[string]bool `json:"-" gorm:"-"`
}

var channelSortFields = map[string]string{
	"id":            "id",
	"name":          "name",
	"type":          "type",
	"status":        "status",
	"response_time": "response_time",
	"test_time":     "test_time",
	"priority":      "priority",
	"weight":        "weight",
	"used_quota":    "used_quota",
	"created_at":    "created_at",
	"updated_at":    "updated_at",
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
	// CustomHeaders are channel-owned upstream request headers. Values may use {{key}} to reference the channel API key.
	CustomHeaders map[string]string `json:"custom_headers,omitempty"`
	// SupportedEndpoints is a list of endpoint names that this channel supports.
	// When nil or empty, the channel uses default endpoints for its type.
	// Endpoint names: chat_completions, completions, embeddings, moderations,
	// images_generations, images_edits, audio_speech, audio_transcription,
	// audio_translation, rerank, response_api, claude_messages, realtime, videos.
	SupportedEndpoints []string `json:"supported_endpoints,omitempty"`
	// EndpointURLs maps an endpoint name (same vocabulary as SupportedEndpoints)
	// to a full upstream URL override for that endpoint. When a request's
	// endpoint has a non-empty override here, the relay layer sends the request
	// to this exact URL instead of the URL derived from BaseURL. This lets an
	// administrator point an individual, non-standard endpoint (for example a
	// rerank surface hosted on a different host/path) at a custom upstream URL.
	// When an endpoint is absent or its value is empty, the channel's default
	// BaseURL-derived URL is used. Applies to HTTP and WebSocket relay surfaces
	// (including the realtime endpoint); SDK-based channels (e.g. AWS Bedrock,
	// Vertex AI) ignore it.
	EndpointURLs map[string]string `json:"endpoint_urls,omitempty"`
}

type ModelConfig struct {
	MaxTokens int32 `json:"max_tokens,omitempty"`
	// Legacy image pricing field kept for backwards compatibility during migrations.
	ImagePriceUsd float64            `json:"image_price_usd,omitempty"`
	Image         *ImagePricingLocal `json:"image,omitempty"`
}

func GetAllChannels(startIdx int, num int, scope string, sortBy string, sortOrder string) ([]*Channel, error) {
	var channels []*Channel
	var err error

	orderClause := ValidateOrderClause(sortBy, sortOrder, channelSortFields, "id desc")

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
	if err != nil {
		return nil, errors.Wrap(err, "get all channels")
	}
	return channels, nil
}

func GetChannelCount() (count int64, err error) {
	err = DB.Model(&Channel{}).Count(&count).Error
	if err != nil {
		return 0, errors.Wrap(err, "count channels")
	}
	return count, nil
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
	orderClause := ValidateOrderClause(sortBy, sortOrder, channelSortFields, "id desc")

	db := DB.Omit("key")
	if scoped, matched := applyUUIDKeyword(db, keyword, "uuid"); matched {
		// A pasted UUID identifies exactly one channel; the LIKE arm cannot add matches.
		db = scoped
	} else {
		// The internal incremental id is deliberately not searchable; UUID is the
		// only external identifier for a channel.
		db = db.Where("name LIKE ?", keyword+"%")
	}
	err = db.Order(orderClause).Find(&channels).Error
	if err != nil {
		return nil, errors.Wrap(err, "search channels")
	}
	return channels, nil
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
		return nil, identity.Tag(
			errors.Wrapf(err, "failed to get channel by id=%d, selectAll=%t", id, selectAll),
			LookupChannelRef(context.Background(), id))
	}
	return &channel, nil
}

// BatchInsertChannels validates and atomically persists channels with their abilities.
// The channels parameter receives generated identifiers, and the function returns a
// wrapped error without leaving partially-created channel rows on failure.
func BatchInsertChannels(channels []Channel) error {
	if len(channels) == 0 {
		return nil
	}
	for i := range channels {
		if err := channels[i].NormalizeHiddenModels(); err != nil {
			return errors.Wrapf(err, "normalize hidden models for channel at index %d", i)
		}
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channels).Error; err != nil {
			return errors.Wrapf(err, "batch insert %d channels", len(channels))
		}
		for i := range channels {
			if err := addAbilitiesWithDB(tx, &channels[i]); err != nil {
				return errors.Wrapf(err, "add abilities for channel %d at batch index %d", channels[i].Id, i)
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "persist channel batch transaction")
	}

	groups := make([]string, 0, len(channels))
	for i := range channels {
		groups = append(groups, channels[i].Group)
	}
	InvalidateChannelModelCaches(groups...)
	return nil
}

func (channel *Channel) GetPriority() int64 {
	if channel.Priority == nil {
		return 0
	}
	return *channel.Priority
}

func (channel *Channel) GetWeight() uint {
	if channel.Weight == nil {
		return 0
	}
	return *channel.Weight
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
			append(channel.Ref().Zap(), zap.Error(err))...)
		return nil
	}
	return modelMapping
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

func (channel *Channel) Insert() error {
	if err := channel.NormalizeHiddenModels(); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to normalize hidden models for channel: name=%s, type=%d", channel.Name, channel.Type),
			channel.Ref())
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(channel).Error; err != nil {
			return errors.Wrapf(err, "insert channel: name=%s, type=%d", channel.Name, channel.Type)
		}
		if err := addAbilitiesWithDB(tx, channel); err != nil {
			return errors.Wrapf(err, "add abilities for channel: id=%d, name=%s", channel.Id, channel.Name)
		}
		return nil
	})
	if err != nil {
		return identity.Tag(errors.Wrap(err, "persist channel transaction"), channel.Ref())
	}
	InvalidateChannelModelCaches(channel.Group)
	return nil
}

func (channel *Channel) Update() error {
	if err := channel.NormalizeHiddenModels(); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to normalize hidden models for channel: id=%d, name=%s", channel.Id, channel.Name),
			channel.Ref())
	}
	// Validate/sync TestingModel with latest supported models
	clearTestingModel := false
	var existing Channel
	if channel.Id != 0 {
		if err := DB.Select("id", "models", "testing_model", "group").First(&existing, "id = ?", channel.Id).Error; err != nil {
			return identity.Tag(
				errors.Wrapf(err, "load existing channel %d before update", channel.Id),
				channel.Ref())
		}
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

	// Some nullable text fields are persisted as *string. GORM's struct-based
	// Updates() skips nil pointers, so collect explicitly provided values for a
	// map update inside the same transaction as the channel and ability rebuild.
	nullableColumnValues := map[string]*string{
		"model_mapping":             channel.ModelMapping,
		"model_configs":             channel.ModelConfigs,
		"system_prompt":             channel.SystemPrompt,
		"inference_profile_arn_map": channel.InferenceProfileArnMap,
	}
	forcedUpdates := make(map[string]any, len(channel.NullableFieldsProvided))
	cleared := make([]string, 0, len(channel.NullableFieldsProvided))
	for column, value := range nullableColumnValues {
		if !channel.NullableFieldsProvided[column] {
			continue
		}
		forcedUpdates[column] = value
		if value == nil {
			cleared = append(cleared, column)
		}
	}

	persisted := *channel
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&persisted).Omit("uuid").Updates(&persisted).Error; err != nil {
			return errors.Wrapf(err, "update channel: id=%d, name=%s", persisted.Id, persisted.Name)
		}
		if persisted.HiddenModelsProvided {
			if err := tx.Model(&Channel{}).Where("id = ?", persisted.Id).Update("hidden_models", persisted.HiddenModels).Error; err != nil {
				return errors.Wrapf(err, "update hidden models for channel: id=%d, name=%s", persisted.Id, persisted.Name)
			}
		}
		if len(forcedUpdates) > 0 {
			if err := tx.Model(&Channel{}).Where("id = ?", persisted.Id).Updates(forcedUpdates).Error; err != nil {
				return errors.Wrapf(err, "update nullable fields for channel: id=%d, name=%s", persisted.Id, persisted.Name)
			}
		}
		if clearTestingModel {
			if err := tx.Model(&Channel{}).Where("id = ?", persisted.Id).Update("testing_model", nil).Error; err != nil {
				return errors.Wrapf(err, "clear testing_model for channel: id=%d", persisted.Id)
			}
		}
		if err := tx.First(&persisted, "id = ?", persisted.Id).Error; err != nil {
			return errors.Wrapf(err, "reload channel %d before rebuilding abilities", persisted.Id)
		}
		if err := deleteAbilitiesWithDB(tx, persisted.Id); err != nil {
			return errors.Wrapf(err, "delete abilities for channel %d during update", persisted.Id)
		}
		if err := addAbilitiesWithDB(tx, &persisted); err != nil {
			return errors.Wrapf(err, "add abilities for channel %d during update", persisted.Id)
		}
		return nil
	})
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "persist channel %d update transaction", channel.Id),
			channel.Ref())
	}

	*channel = persisted
	if len(cleared) > 0 {
		logger.Logger.Debug("channel update cleared nullable fields",
			append(channel.Ref().Zap(), zap.Strings("cleared_fields", cleared))...)
	}
	InvalidateChannelModelCaches(existing.Group, channel.Group)
	return nil
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	err := DB.Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     helper.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		logger.Logger.Error("failed to update response time",
			append(channel.Ref().Zap(), zap.Error(err))...)
	}
}

func (channel *Channel) UpdateBalance(balance float64) {
	err := DB.Model(channel).Select("balance_updated_time", "balance").Updates(Channel{
		BalanceUpdatedTime: helper.GetTimestamp(),
		Balance:            balance,
	}).Error
	if err != nil {
		logger.Logger.Error("failed to update balance",
			append(channel.Ref().Zap(), zap.Error(err))...)
	}
}

func (channel *Channel) Delete() error {
	oldGroups := channel.Group
	if strings.TrimSpace(oldGroups) == "" && channel.Id != 0 {
		var existing Channel
		if err := DB.Select("id", "group").First(&existing, "id = ?", channel.Id).Error; err == nil {
			oldGroups = existing.Group
		}
	}
	if err := DB.Delete(channel).Error; err != nil {
		return identity.Tag(errors.Wrapf(err, "delete channel %d", channel.Id), channel.Ref())
	}
	if err := channel.DeleteAbilities(); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "delete abilities for channel %d", channel.Id),
			channel.Ref())
	}
	InvalidateChannelModelCaches(oldGroups)
	return nil
}

func (channel *Channel) LoadConfig() (ChannelConfig, error) {
	var cfg ChannelConfig
	if channel.Config == "" {
		return cfg, nil
	}
	err := json.Unmarshal([]byte(channel.Config), &cfg)
	if err != nil {
		return cfg, identity.Tag(
			errors.Wrapf(err, "unmarshal channel %d config", channel.Id),
			channel.Ref())
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
			append(channel.Ref().Zap(), zap.Error(err))...)
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
		return identity.Tag(
			errors.Wrapf(err, "marshal channel %d config", channel.Id),
			channel.Ref())
	}
	if len(data) == 0 || string(data) == "{}" {
		channel.Config = ""
		return nil
	}
	channel.Config = string(data)
	return nil
}

func UpdateChannelStatusById(id int, status int) {
	var channel Channel
	// uuid and name are selected alongside the group so the failure logs below can
	// name the channel without issuing a second query.
	_ = DB.Select("id", "uuid", "name", "group").First(&channel, "id = ?", id).Error
	if channel.Id == 0 {
		channel.Id = id
	}
	err := UpdateAbilityStatus(id, status == ChannelStatusEnabled)
	if err != nil {
		logger.Logger.Error("failed to update ability status",
			append(channel.Ref().Zap(), zap.Error(err), zap.Int("status", status))...)
	}
	err = DB.Model(&Channel{}).Where("id = ?", id).Update("status", status).Error
	if err != nil {
		logger.Logger.Error("failed to update channel status",
			append(channel.Ref().Zap(), zap.Error(err), zap.Int("status", status))...)
	}
	if err == nil {
		InvalidateChannelModelCaches(channel.Group)
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
			append(LookupChannelRef(context.Background(), id).Zap(),
				zap.Error(err),
				zap.Int64("quota", quota),
				zap.String("note", "billing completed successfully but channel usage statistics update failed"))...)
	}
}

func DeleteChannelByStatus(status int64) (int64, error) {
	var channels []Channel
	_ = DB.Select("group").Where("status = ?", status).Find(&channels).Error
	groups := make([]string, 0, len(channels))
	for _, channel := range channels {
		groups = append(groups, channel.Group)
	}
	result := DB.Where("status = ?", status).Delete(&Channel{})
	if result.Error == nil {
		InvalidateChannelModelCaches(groups...)
	}
	return result.RowsAffected, result.Error
}

func DeleteDisabledChannel() (int64, error) {
	var channels []Channel
	_ = DB.Select("group").Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Find(&channels).Error
	groups := make([]string, 0, len(channels))
	for _, channel := range channels {
		groups = append(groups, channel.Group)
	}
	result := DB.Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Delete(&Channel{})
	if result.Error == nil {
		InvalidateChannelModelCaches(groups...)
	}
	return result.RowsAffected, result.Error
}
