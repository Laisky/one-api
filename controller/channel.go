package controller

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"strconv"
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	glog "github.com/Laisky/go-utils/v6/log"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/helper"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/pricing"
)

type channelPayload struct {
	*model.Channel
	Tooling json.RawMessage `json:"tooling"`
}

func bindChannelPayload(c *gin.Context) (*model.Channel, json.RawMessage, error) {
	payload := channelPayload{Channel: &model.Channel{}}
	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, nil, err
	}
	return payload.Channel, payload.Tooling, nil
}

func parseToolingConfigPayload(raw json.RawMessage) (*model.ChannelToolingConfig, bool, error) {
	if raw == nil {
		return nil, false, nil
	}

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, true, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, true, nil
	}

	// Handle the common case where the tooling payload is provided as a JSON string.
	var toolingString string
	if err := json.Unmarshal(trimmed, &toolingString); err == nil {
		if strings.TrimSpace(toolingString) == "" {
			return nil, true, nil
		}
		if strings.EqualFold(strings.TrimSpace(toolingString), "null") {
			return nil, true, nil
		}

		var cfg model.ChannelToolingConfig
		if err := json.Unmarshal([]byte(toolingString), &cfg); err != nil {
			return nil, true, err
		}
		return &cfg, true, nil
	}

	// Otherwise, treat the payload as a JSON object.
	var cfg model.ChannelToolingConfig
	if err := json.Unmarshal(trimmed, &cfg); err != nil {
		return nil, true, err
	}
	return &cfg, true, nil
}

func convertAdaptorVideoPricing(cfg *adaptor.VideoPricingConfig) *model.VideoPricingLocal {
	if cfg == nil || !cfg.HasData() {
		return nil
	}
	local := &model.VideoPricingLocal{
		PerSecondUsd: cfg.PerSecondUsd,
	}
	if strings.TrimSpace(cfg.BaseResolution) != "" {
		local.BaseResolution = cfg.BaseResolution
	}
	if len(cfg.ResolutionMultipliers) > 0 {
		local.ResolutionMultipliers = make(map[string]float64, len(cfg.ResolutionMultipliers))
		maps.Copy(local.ResolutionMultipliers, cfg.ResolutionMultipliers)
	}
	return local
}

func buildChannelResponsePayload(lg glog.Logger, channel *model.Channel) any {
	response := struct {
		*model.Channel
		Tooling *string `json:"tooling,omitempty"`
	}{Channel: channel}

	if tooling := channel.GetToolingConfig(); tooling != nil {
		if data, err := json.Marshal(tooling); err == nil {
			toolingStr := string(data)
			response.Tooling = &toolingStr
		} else if lg != nil {
			lg.Error("failed to marshal tooling config", zap.Int("channel_id", channel.Id), zap.Error(err))
		}
	}

	return response
}

// GetAllChannels lists channel records with pagination and optional sorting parameters.
func GetAllChannels(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}

	// Get page size from query parameter, default to config value
	size, _ := strconv.Atoi(c.Query("size"))
	if size <= 0 {
		size = config.DefaultItemsPerPage
	}
	if size > config.MaxItemsPerPage {
		size = config.MaxItemsPerPage
	}

	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	channels, err := model.GetAllChannels(p*size, size, "limited", sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get total count for pagination
	totalCount, err := model.GetChannelCount()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channels,
		"total":   totalCount,
	})
}

// SearchChannels performs a keyword search across channels and returns the matching results.
func SearchChannels(c *gin.Context) {
	keyword := c.Query("keyword")
	sortBy := c.Query("sort")
	sortOrder := c.Query("order")
	if sortOrder == "" {
		sortOrder = "desc"
	}

	channels, err := model.SearchChannels(keyword, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    channels,
	})
}

// GetChannel retrieves a single channel by ID and returns its full configuration.
func GetChannel(c *gin.Context) {
	lg := gmw.GetLogger(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildChannelResponsePayload(lg, channel),
	})
}

// AddChannel creates one or more channels using the posted configuration payload.
func AddChannel(c *gin.Context) {
	channel, toolingRaw, err := bindChannelPayload(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Disallow empty channel name
	if strings.TrimSpace(channel.Name) == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Channel name is required",
		})
		return
	}

	// Validate inference profile ARN map if provided
	if channel.InferenceProfileArnMap != nil && *channel.InferenceProfileArnMap != "" {
		err = model.ValidateInferenceProfileArnMapJSON(*channel.InferenceProfileArnMap)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Invalid inference profile ARN map: " + err.Error(),
			})
			return
		}
	}

	if toolingCfg, provided, err := parseToolingConfigPayload(toolingRaw); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid tooling config: " + err.Error(),
		})
		return
	} else if provided {
		if err := channel.SetToolingConfig(toolingCfg); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to persist tooling config: " + err.Error(),
			})
			return
		}
	}

	channel.CreatedTime = helper.GetTimestamp()
	// Sanitize testing model at creation: only keep if present in models list
	if channel.TestingModel != nil {
		tm := strings.TrimSpace(*channel.TestingModel)
		if tm == "" {
			channel.TestingModel = nil
		} else {
			ok := false
			for name := range strings.SplitSeq(channel.Models, ",") {
				if strings.TrimSpace(name) == tm {
					ok = true
					break
				}
			}
			if !ok {
				channel.TestingModel = nil
			}
		}
	}
	keys := strings.Split(channel.Key, "\n")
	channels := make([]model.Channel, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			continue
		}
		localChannel := *channel
		localChannel.Key = key
		// Auto-populate default BaseURL on creation if blank and default exists
		if (localChannel.BaseURL == nil || *localChannel.BaseURL == "") && localChannel.Type >= 0 {
			// Defensive bounds check against channeltype.ChannelBaseURLs
			if localChannel.Type < len(channeltype.ChannelBaseURLs) {
				def := channeltype.ChannelBaseURLs[localChannel.Type]
				if strings.TrimSpace(def) != "" {
					v := strings.TrimRight(def, "/")
					localChannel.BaseURL = &v
				}
			}
		}
		channels = append(channels, localChannel)
	}
	err = model.BatchInsertChannels(channels)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteChannel removes the channel identified by the path parameter.
func DeleteChannel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	channel := model.Channel{Id: id}
	err := channel.Delete()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteDisabledChannel removes all channels currently marked as disabled and returns the affected row count.
func DeleteDisabledChannel(c *gin.Context) {
	rows, err := model.DeleteDisabledChannel()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    rows,
	})
}

// UpdateChannel updates the channel configuration or status based on the posted payload.
func UpdateChannel(c *gin.Context) {
	lg := gmw.GetLogger(c)
	statusOnly := c.Query("status_only")
	channel, toolingRaw, err := bindChannelPayload(c)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Validate inference profile ARN map if provided
	if channel.InferenceProfileArnMap != nil && *channel.InferenceProfileArnMap != "" {
		err = model.ValidateInferenceProfileArnMapJSON(*channel.InferenceProfileArnMap)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Invalid inference profile ARN map: " + err.Error(),
			})
			return
		}
	}

	if statusOnly != "" {
		// Only update status safely
		if channel.Id == 0 {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Channel id is required"})
			return
		}
		model.UpdateChannelStatusById(channel.Id, channel.Status)
		c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
		return
	}

	// Disallow empty name on full update
	if strings.TrimSpace(channel.Name) == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Channel name cannot be empty",
		})
		return
	}

	if toolingCfg, provided, err := parseToolingConfigPayload(toolingRaw); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid tooling config: " + err.Error(),
		})
		return
	} else if provided {
		if err := channel.SetToolingConfig(toolingCfg); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to persist tooling config: " + err.Error(),
			})
			return
		}
	}

	err = channel.Update()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    buildChannelResponsePayload(lg, channel),
	})
}

// GetChannelPricing returns the pricing configuration associated with the specified channel.
func GetChannelPricing(c *gin.Context) {
	lg := gmw.GetLogger(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channel, err := model.GetChannelById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Get from unified ModelConfigs only (after migration)
	modelRatio := channel.GetModelRatioFromConfigs()
	completionRatio := channel.GetCompletionRatioFromConfigs()

	// Also get the unified ModelConfigs
	modelConfigs := channel.GetModelPriceConfigs()
	tooling := channel.GetToolingConfig()

	// Debug logging to help identify data issues
	if len(modelConfigs) > 0 {
		var modelNames []string
		for modelName := range modelConfigs {
			modelNames = append(modelNames, modelName)
		}
		if lg != nil {
			lg.Info("Channel returning model configs", zap.Int("id", channel.Id), zap.Int("type", channel.Type), zap.Any("models", modelNames))
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"model_ratio":      modelRatio,
			"completion_ratio": completionRatio,
			"model_configs":    modelConfigs,
			"tooling":          tooling,
		},
	})
}

// UpdateChannelPricing replaces the channel pricing configuration using either legacy ratios or the unified model config format.
func UpdateChannelPricing(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	var request struct {
		ModelRatio      map[string]float64                `json:"model_ratio"`
		CompletionRatio map[string]float64                `json:"completion_ratio"`
		ModelConfigs    map[string]model.ModelConfigLocal `json:"model_configs"`
		Tooling         json.RawMessage                   `json:"tooling"`
	}

	err = c.ShouldBindJSON(&request)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	channel, err := model.GetChannelById(id, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Handle both old format (separate model_ratio and completion_ratio) and new format (unified model_configs)
	if len(request.ModelConfigs) > 0 {
		// New unified format - preferred approach
		err = channel.SetModelPriceConfigs(request.ModelConfigs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to set model configs: " + err.Error(),
			})
			return
		}
	} else if len(request.ModelRatio) > 0 || len(request.CompletionRatio) > 0 {
		// Old format - convert to unified format automatically
		modelConfigs := make(map[string]model.ModelConfigLocal)

		// Collect all model names from both ratios
		allModelNames := make(map[string]bool)
		for modelName := range request.ModelRatio {
			allModelNames[modelName] = true
		}
		for modelName := range request.CompletionRatio {
			allModelNames[modelName] = true
		}

		// Create ModelPriceLocal entries for each model
		for modelName := range allModelNames {
			config := model.ModelConfigLocal{}

			if request.ModelRatio != nil {
				if ratio, exists := request.ModelRatio[modelName]; exists {
					config.Ratio = ratio
				}
			}

			if request.CompletionRatio != nil {
				if completionRatio, exists := request.CompletionRatio[modelName]; exists {
					config.CompletionRatio = completionRatio
				}
			}

			// Only add if we have some pricing data
			if config.Ratio != 0 || config.CompletionRatio != 0 {
				modelConfigs[modelName] = config
			}
		}

		// Save to unified ModelConfigs only
		err = channel.SetModelPriceConfigs(modelConfigs)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to set model configs: " + err.Error(),
			})
			return
		}
	}

	if toolingCfg, provided, err := parseToolingConfigPayload(request.Tooling); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid tooling config: " + err.Error(),
		})
		return
	} else if provided {
		if err := channel.SetToolingConfig(toolingCfg); err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Failed to set tooling config: " + err.Error(),
			})
			return
		}
	}

	err = channel.Update()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// GetChannelDefaultPricing returns adapter-provided default pricing metadata for the supplied channel type.
func GetChannelDefaultPricing(c *gin.Context) {
	channelType, err := strconv.Atoi(c.Query("type"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Invalid channel type: " + err.Error(),
		})
		return
	}

	var (
		defaultPricing  map[string]adaptor.ModelConfig
		providerAdaptor adaptor.Adaptor
	)

	// OpenAI-compatible channels use global pricing so operators can mix models from
	// multiple providers without defining per-channel price maps.
	if channeltype.IsOpenAICompatible(channelType) {
		// Use global pricing manager to get pricing from all adapters
		defaultPricing = pricing.GetGlobalModelPricing()
	} else {
		// For specific channel types, use their adapter's default pricing
		// Convert channel type to API type first
		apiType := channeltype.ToAPIType(channelType)
		providerAdaptor = relay.GetAdaptor(apiType)
		if providerAdaptor == nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "Unsupported channel type",
			})
			return
		}
		defaultPricing = providerAdaptor.GetDefaultModelPricing()
	}

	// Separate model ratios and completion ratios for UI compatibility
	modelRatios := make(map[string]float64)
	completionRatios := make(map[string]float64)

	for model, price := range defaultPricing {
		modelRatios[model] = price.Ratio
		// Include all completion ratios, including 0 (which is valid pricing info)
		completionRatios[model] = price.CompletionRatio
	}

	// Create unified model configs format without tooling metadata
	modelConfigs := make(map[string]model.ModelConfigLocal)
	for modelName, price := range defaultPricing {
		modelConfigs[modelName] = model.ModelConfigLocal{
			Ratio:           price.Ratio,
			CompletionRatio: price.CompletionRatio,
			MaxTokens:       price.MaxTokens,
			Video:           convertAdaptorVideoPricing(price.Video),
		}
	}

	var toolingConfigJSON string
	if toolingProvider, ok := providerAdaptor.(adaptor.ToolingDefaultsProvider); ok {
		tooling := convertAdaptorTooling(toolingProvider.DefaultToolingConfig())
		if tooling != nil {
			data, err := json.Marshal(tooling)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"success": false,
					"message": "Failed to serialize tooling config: " + err.Error(),
				})
				return
			}
			toolingConfigJSON = string(data)
		}
	}

	// Convert to JSON
	modelRatioJSON, err := json.Marshal(modelRatios)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to serialize model ratios: " + err.Error(),
		})
		return
	}

	completionRatioJSON, err := json.Marshal(completionRatios)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to serialize completion ratios: " + err.Error(),
		})
		return
	}

	modelConfigsJSON, err := json.Marshal(modelConfigs)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "Failed to serialize model configs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"model_ratio":      string(modelRatioJSON),
			"completion_ratio": string(completionRatioJSON),
			"model_configs":    string(modelConfigsJSON),
			"tooling":          toolingConfigJSON,
		},
	})
}

// convertAdaptorTooling translates provider default tooling metadata into the
// channel-scoped DTO representation used by persistence and API responses.
func convertAdaptorTooling(cfg adaptor.ChannelToolConfig) *model.ChannelToolingConfig {
	if len(cfg.Whitelist) == 0 && len(cfg.Pricing) == 0 {
		return nil
	}
	tooling := &model.ChannelToolingConfig{}
	if len(cfg.Whitelist) > 0 {
		tooling.Whitelist = append([]string(nil), cfg.Whitelist...)
	}
	if len(cfg.Pricing) > 0 {
		tooling.Pricing = make(map[string]model.ToolPricingLocal, len(cfg.Pricing))
		for tool, price := range cfg.Pricing {
			tooling.Pricing[tool] = model.ToolPricingLocal{
				UsdPerCall:   price.UsdPerCall,
				QuotaPerCall: price.QuotaPerCall,
			}
		}
	}
	if len(tooling.Whitelist) == 0 && len(tooling.Pricing) == 0 {
		return nil
	}
	return tooling
}
