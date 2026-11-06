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
	"github.com/Laisky/one-api/relay/channeltype"
)

// ChannelTestingModelSkip is the sentinel testing_model value that excludes a
// channel from health checking altogether — the automatic sweep and the list-page
// Test button both report it as skipped instead of probing it.
//
// It is stored in the testing_model column rather than a separate flag because the
// administrator chooses it from the same per-channel selector as the model names,
// alongside the auto-select ("cheapest") option. It is deliberately not a legal
// model name, and it is never sent upstream. The admin UI offers the same literal,
// so changing it is a breaking wire change.
const ChannelTestingModelSkip = "__skip__"

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
	// Group is the comma-separated list of every group the channel serves. Loaded
	// channels carry the complete list even when the channels.group column holds only
	// a prefix of it; see GroupTruncated and ChannelGroup.
	Group string `json:"group" gorm:"type:varchar(32);default:'default'"`
	// GroupTruncated is true when channels.group holds only a leading part of the
	// channel's groups because the whole list does not fit the column, so channel_groups
	// alone holds all of them; false when the column holds the whole list. It is nil for
	// a row no relation-aware writer has stored (one written before the relation existed,
	// or by an older binary), whose column is complete. It deliberately has no column
	// default, so existing rows read back NULL and startup reconciliation finds them.
	GroupTruncated *bool   `json:"-" gorm:"column:group_truncated;index"`
	UsedQuota      int64   `json:"used_quota" gorm:"bigint;default:0"`
	ModelMapping   *string `json:"model_mapping" gorm:"type:text"`
	Priority       *int64  `json:"priority" gorm:"bigint;default:0"`
	Config         string  `json:"config"`
	SystemPrompt   *string `json:"system_prompt" gorm:"type:text"`
	RateLimit      *int    `json:"ratelimit" gorm:"column:ratelimit;default:0"`
	// Preferred testing model for this channel (optional).
	// If empty or nil, the system auto-selects the cheapest chat-format model at test
	// time. Set it to ChannelTestingModelSkip to exclude the channel from testing.
	// Every write path stores it through Channel.NormalizeTestingModel.
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
	return GetAllChannelsWithContext(context.Background(), startIdx, num, scope, sortBy, sortOrder)
}

// GetAllChannelsWithContext returns channels with database cancellation bound
// to ctx.
//
// Parameters:
//   - ctx: lifecycle and deadline scope for the query.
//   - startIdx, num, scope, sortBy, sortOrder: pagination and filtering inputs.
//
// Return values:
//   - []*Channel: matching channels.
//   - error: wrapped query failure.
func GetAllChannelsWithContext(ctx context.Context, startIdx int, num int, scope string, sortBy string, sortOrder string) ([]*Channel, error) {
	var channels []*Channel
	var err error

	orderClause := ValidateOrderClause(sortBy, sortOrder, channelSortFields, "id desc")

	switch scope {
	case "all":
		if num > 0 {
			// Apply pagination when num > 0
			err = DB.WithContext(ctx).Order(orderClause).Limit(num).Offset(startIdx).Find(&channels).Error
		} else {
			// Return all channels when num = 0 (backward compatibility)
			err = DB.WithContext(ctx).Order(orderClause).Find(&channels).Error
		}
	case "disabled":
		err = DB.WithContext(ctx).Order(orderClause).Where("status = ? or status = ?", ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled).Find(&channels).Error
	default:
		err = DB.WithContext(ctx).Order(orderClause).Limit(num).Offset(startIdx).Omit("key").Find(&channels).Error
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

	db := applyChannelSearch(DB.Omit("key"), keyword)
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
		channels[i].TestingModel = channels[i].NormalizeTestingModel(channels[i].TestingModel)
		if err := validateJinaChannelConfiguration(&channels[i]); err != nil {
			return errors.Wrapf(err, "validate channel at batch index %d", i)
		}
	}

	groupLists := make([][]string, len(channels))
	for i := range channels {
		groupLists[i] = assignedChannelGroups(&channels[i])
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		restores := make([]func(), len(channels))
		for i := range channels {
			restores[i] = stageChannelGroupColumn(&channels[i], groupLists[i])
		}
		err := tx.Create(&channels).Error
		for _, restore := range restores {
			restore()
		}
		if err != nil {
			return errors.Wrapf(err, "batch insert %d channels", len(channels))
		}
		ids := make([]int, len(channels))
		for i := range channels {
			ids[i] = channels[i].Id
		}
		if err := insertNewChannelGroupsWithDB(tx, ids, groupLists); err != nil {
			return errors.Wrapf(err, "write groups for %d channels", len(channels))
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
	return channel.GetModelMappingWithContext(context.Background())
}

// GetModelMappingWithContext parses the channel model mapping with a
// context-aware logger. Parameters: ctx carries request correlation when the
// mapping is read during an HTTP request. Returns: the mapping or nil when it
// is empty or malformed.
func (channel *Channel) GetModelMappingWithContext(ctx context.Context) map[string]string {
	if channel.ModelMapping == nil || *channel.ModelMapping == "" || *channel.ModelMapping == "{}" {
		return nil
	}
	modelMapping := make(map[string]string)
	err := json.Unmarshal([]byte(*channel.ModelMapping), &modelMapping)
	if err != nil {
		logger.FromContext(ctx).Error("failed to unmarshal model mapping for channel",
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
			cheapestName, cheapestRatio = name, r
			initialized = true
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
	channel.TestingModel = channel.NormalizeTestingModel(channel.TestingModel)
	if err := validateJinaChannelConfiguration(channel); err != nil {
		return errors.Wrap(err, "validate new channel")
	}
	groups := assignedChannelGroups(channel)
	err := DB.Transaction(func(tx *gorm.DB) error {
		restore := stageChannelGroupColumn(channel, groups)
		err := tx.Create(channel).Error
		restore()
		if err != nil {
			return errors.Wrapf(err, "insert channel: name=%s, type=%d", channel.Name, channel.Type)
		}
		if err := insertNewChannelGroupsWithDB(tx, []int{channel.Id}, [][]string{groups}); err != nil {
			return errors.Wrapf(err, "write groups for channel: id=%d, name=%s", channel.Id, channel.Name)
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
	return channel.UpdateWithContext(context.Background())
}

// UpdateWithContext persists a channel update and binds safe diagnostics to ctx.
// Parameters: ctx carries request logging and channel is the channel to persist.
// Returns: a wrapped validation or persistence error.
func (channel *Channel) UpdateWithContext(ctx context.Context) error {
	if err := channel.NormalizeHiddenModels(); err != nil {
		return identity.Tag(
			errors.Wrapf(err, "failed to normalize hidden models for channel: id=%d, name=%s", channel.Id, channel.Name),
			channel.Ref())
	}
	// A client disconnect must not abort a half-applied update, so the write is detached
	// from cancellation, but it stays bounded: the caller's deadline or
	// channelWriteTimeout, whichever is sooner, also caps row-lock waits.
	writeCtx, cancel := detachedChannelWriteContext(ctx)
	defer cancel()
	var existing Channel
	if channel.Id != 0 {
		if err := DB.WithContext(writeCtx).Select("id", "group").First(&existing, "id = ?", channel.Id).Error; err != nil {
			return identity.Tag(
				errors.Wrapf(err, "load existing channel %d before update", channel.Id),
				channel.Ref())
		}
	}
	// testing_model has merge-patch semantics (omitted keeps, null clears) and is
	// resolved against the locked row inside the transaction, never written by the
	// struct update. A non-nil pointer is an explicit write, so callers that reload a
	// row and save it back must clear TestingModel unless they mean to change it.
	requestedTestingModel := channel.TestingModel
	testingModelProvided := requestedTestingModel != nil || channel.NullableFieldsProvided[ChannelTestingModelField]

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

	// A submission that names no group keeps the stored groups: the struct update skips
	// the empty column and the relation is left alone.
	requestedGroups := channel.GetGroupNames()
	persisted := *channel
	var testingModel testingModelResolution
	err := DB.WithContext(writeCtx).Transaction(func(tx *gorm.DB) error {
		if err := lockChannelRow(tx, persisted.Id); err != nil {
			return errors.Wrapf(err, "lock channel %d for update", persisted.Id)
		}
		var upstreamBefore Channel
		if err := tx.Select(abilityUpstreamColumns).First(&upstreamBefore, "id = ?", persisted.Id).Error; err != nil {
			return errors.Wrapf(err, "read upstream configuration of channel %d before update", persisted.Id)
		}
		// The reload below gives persisted its whole list back, so the staged column
		// copy needs no restore.
		persisted.Group, persisted.GroupTruncated = "", nil
		if len(requestedGroups) > 0 {
			stageChannelGroupColumn(&persisted, requestedGroups)
		}
		if err := tx.Model(&persisted).Omit("uuid", ChannelTestingModelField).Updates(&persisted).Error; err != nil {
			return errors.Wrapf(err, "update channel: id=%d, name=%s", persisted.Id, persisted.Name)
		}
		if len(requestedGroups) > 0 {
			if err := replaceChannelGroupsWithDB(tx, persisted.Id, requestedGroups); err != nil {
				return errors.Wrapf(err, "write groups for channel %d", persisted.Id)
			}
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
		resolution, err := persistResolvedTestingModel(tx, persisted.Id, requestedTestingModel, testingModelProvided)
		if err != nil {
			return errors.Wrap(err, "persist resolved testing_model")
		}
		testingModel = resolution
		if err := tx.First(&persisted, "id = ?", persisted.Id).Error; err != nil {
			return errors.Wrapf(err, "reload channel %d before rebuilding abilities", persisted.Id)
		}
		// Validate the effective row, not an incomplete update DTO. A type-only
		// change or inherited unsafe endpoint must roll back the whole update.
		if err := validateJinaChannelConfiguration(&persisted); err != nil {
			return errors.Wrap(err, "validate updated channel")
		}
		// Keep the relay-time suspensions of retained abilities unless this write changed
		// the upstream they were measured against.
		if err := syncAbilitiesWithDB(tx, &persisted, !sameAbilityUpstream(&upstreamBefore, &persisted)); err != nil {
			return errors.Wrapf(err, "sync abilities for channel %d during update", persisted.Id)
		}
		return nil
	})
	if err != nil {
		return identity.Tag(
			errors.Wrapf(err, "persist channel %d update transaction", channel.Id),
			channel.Ref())
	}

	*channel = persisted
	lg := logger.FromContext(ctx)
	if len(cleared) > 0 {
		lg.Debug("channel update cleared nullable fields",
			append(channel.Ref().Zap(), zap.Strings("cleared_fields", cleared))...)
	}
	if testingModel.written {
		lg.Debug("channel update resolved testing_model",
			append(channel.Ref().Zap(),
				zap.Bool("testing_model_provided", testingModelProvided),
				zap.String("requested_testing_model", derefStr(requestedTestingModel)),
				zap.String("previous_testing_model", derefStr(testingModel.previous)),
				zap.String("persisted_testing_model", derefStr(testingModel.persisted)))...)
	}
	InvalidateChannelModelCachesWithContext(ctx, existing.Group, channel.Group)
	return nil
}

func (channel *Channel) UpdateResponseTime(responseTime int64) {
	channel.UpdateResponseTimeWithContext(context.Background(), responseTime)
}

// UpdateResponseTimeWithContext stores channel probe latency and binds failures to ctx.
// Parameters: ctx carries request logging and responseTime is the measured latency in milliseconds.
// Returns: none; persistence failures are logged with channel identity.
func (channel *Channel) UpdateResponseTimeWithContext(ctx context.Context, responseTime int64) {
	err := DB.WithContext(ctx).Model(channel).Select("response_time", "test_time").Updates(Channel{
		TestTime:     helper.GetTimestamp(),
		ResponseTime: int(responseTime),
	}).Error
	if err != nil {
		logger.FromContext(ctx).Error("failed to update response time",
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

// Delete removes the channel together with its abilities and channel_groups rows in one
// transaction, then refreshes the model-list caches of the groups it served.
//
// Return values:
//   - error: a wrapped read or delete failure; nothing is removed when it is non-nil.
func (channel *Channel) Delete() error {
	oldGroups := channel.Group
	if strings.TrimSpace(oldGroups) == "" && channel.Id != 0 {
		var existing Channel
		err := DB.Select("id", "group").First(&existing, "id = ?", channel.Id).Error
		switch {
		case err == nil:
			oldGroups = existing.Group
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return identity.Tag(errors.Wrapf(err, "read groups of channel %d before deletion", channel.Id), channel.Ref())
		}
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := deleteAbilitiesWithDB(tx, channel.Id); err != nil {
			return errors.Wrapf(err, "delete abilities for channel %d", channel.Id)
		}
		if err := deleteChannelGroupsWithDB(tx, channel.Id); err != nil {
			return errors.Wrapf(err, "delete groups for channel %d", channel.Id)
		}
		if err := tx.Delete(channel).Error; err != nil {
			return errors.Wrapf(err, "delete channel %d", channel.Id)
		}
		return nil
	})
	if err != nil {
		return identity.Tag(errors.Wrapf(err, "delete channel %d transaction", channel.Id), channel.Ref())
	}
	InvalidateChannelModelCaches(oldGroups)
	return nil
}

// LoadConfig decodes stored channel settings and applies provider-specific URL
// security policy, including when Config is empty and only BaseURL is present.
func (channel *Channel) LoadConfig() (ChannelConfig, error) {
	var cfg ChannelConfig
	if channel.Config != "" {
		if err := json.Unmarshal([]byte(channel.Config), &cfg); err != nil {
			return cfg, identity.Tag(
				errors.Wrapf(err, "unmarshal channel %d config", channel.Id),
				channel.Ref())
		}
	}
	if channel.Type == channeltype.Jina {
		if err := channeltype.ValidateJinaURLs(channel.GetBaseURL(), cfg.EndpointURLs); err != nil {
			return cfg, identity.Tag(errors.Wrap(err, "validate Jina channel configuration"), channel.Ref())
		}
	}
	return cfg, nil
}

// GetSupportedEndpoints returns the effective supported endpoints for this channel.
// If the channel has custom endpoints configured, those are returned.
// Otherwise, the default endpoints for the channel type are returned.
func (channel *Channel) GetSupportedEndpoints() []string {
	return channel.GetSupportedEndpointsWithContext(context.Background())
}

// GetSupportedEndpointsWithContext returns configured endpoints while retaining
// request correlation in ctx. Parameters: ctx carries the request logger when
// called from middleware. Returns: configured endpoint names or nil for defaults.
func (channel *Channel) GetSupportedEndpointsWithContext(ctx context.Context) []string {
	cfg, err := channel.LoadConfig()
	if err != nil {
		logger.FromContext(ctx).Error("failed to load channel config for endpoints",
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
	UpdateChannelStatusByIdWithContext(context.Background(), id, status)
}

// UpdateChannelStatusByIdWithContext updates channel ability/status state with request-scoped diagnostics.
// It delegates to SetChannelStatusWithContext so the status and the ability routing
// flags change in one locked transaction; the separate statements it used to issue let
// a concurrent status change leave a disabled channel whose abilities still routed traffic.
// Relay error paths call it after the request has finished, so, as before, the write
// ignores ctx's cancellation and deadline; channelWriteTimeout still bounds lock waits.
// Parameters: ctx carries request logging, id identifies the channel, and status is the desired channel status.
// Returns: none; persistence failures are logged with channel identity.
func UpdateChannelStatusByIdWithContext(ctx context.Context, id int, status int) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), channelWriteTimeout)
	defer cancel()
	if err := SetChannelStatusWithContext(writeCtx, id, status); err != nil {
		logger.FromContext(ctx).Error("failed to update channel status",
			append(LookupChannelRef(ctx, id).Zap(), zap.Error(err), zap.Int("status", status))...)
	}
}

func UpdateChannelUsedQuota(id int, quota int64) {
	UpdateChannelUsedQuotaWithContext(context.Background(), id, quota)
}

// UpdateChannelUsedQuotaWithContext updates channel usage while preserving
// request correlation in ctx. Parameters: ctx carries the request logger, id
// identifies the channel, and quota is the consumed amount. Returns: none;
// persistence failures are logged with the context logger.
func UpdateChannelUsedQuotaWithContext(ctx context.Context, id int, quota int64) {
	if config.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeChannelUsedQuota, id, quota)
		return
	}
	updateChannelUsedQuota(ctx, id, quota)
}

// updateChannelUsedQuota increments channel usage, retrying only failed SQLite busy writes within ctx.
func updateChannelUsedQuota(ctx context.Context, id int, quota int64) {
	db := DB
	err := runWithSQLiteBusyRetryForDB(ctx, db, func() error {
		return errors.WithStack(db.WithContext(ctx).Model(&Channel{}).Where("id = ?", id).
			Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error)
	})
	if err != nil {
		logger.FromContext(ctx).Error("failed to update channel used quota - channel statistics may be inaccurate",
			append(LookupChannelRef(ctx, id).Zap(),
				zap.Error(err),
				zap.Int64("quota", quota),
				zap.String("note", "billing completed successfully but channel usage statistics update failed"))...)
	}
}

// DeleteChannelByStatus removes every channel whose status equals status, with its
// abilities and channel_groups rows, and returns how many channels it removed.
//
// Parameters:
//   - status: the channel status to delete.
//
// Return values:
//   - int64: the number of channels removed.
//   - error: a wrapped failure; nothing is removed when it is non-nil.
func DeleteChannelByStatus(status int64) (int64, error) {
	removed, err := deleteChannelsWithStatus(context.Background(), int(status))
	if err != nil {
		return 0, errors.Wrapf(err, "delete channels with status %d", status)
	}
	return removed, nil
}

// DeleteDisabledChannel removes every automatically or manually disabled channel, with
// its abilities and channel_groups rows, and returns how many channels it removed.
//
// Return values:
//   - int64: the number of channels removed.
//   - error: a wrapped failure; nothing is removed when it is non-nil.
func DeleteDisabledChannel() (int64, error) {
	removed, err := deleteChannelsWithStatus(context.Background(), ChannelStatusAutoDisabled, ChannelStatusManuallyDisabled)
	if err != nil {
		return 0, errors.Wrap(err, "delete disabled channels")
	}
	return removed, nil
}
