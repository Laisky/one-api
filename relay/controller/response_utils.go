package controller

import (
	"context"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/model"
	"github.com/songquanpeng/one-api/relay/adaptor/openai"
	"github.com/songquanpeng/one-api/relay/channeltype"
	metalib "github.com/songquanpeng/one-api/relay/meta"
	relaymodel "github.com/songquanpeng/one-api/relay/model"
)

// getChannelRatios gets channel model and completion ratios from unified ModelConfigs
func getChannelRatios(c *gin.Context) (map[string]float64, map[string]float64) {
	channel := c.MustGet(ctxkey.ChannelModel).(*model.Channel)

	// Only use unified ModelConfigs after migration
	modelRatios := channel.GetModelRatioFromConfigs()
	completionRatios := channel.GetCompletionRatioFromConfigs()

	return modelRatios, completionRatios
}

// getAndValidateResponseAPIRequest gets and validates Response API request
func getAndValidateResponseAPIRequest(c *gin.Context) (*openai.ResponseAPIRequest, error) {
	responseAPIRequest := &openai.ResponseAPIRequest{}
	err := common.UnmarshalBodyReusable(c, responseAPIRequest)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal Response API request")
	}

	// Basic validation
	if responseAPIRequest.Model == "" {
		return nil, errors.New("model is required")
	}

	// Either input or prompt is required, but not both
	hasInput := len(responseAPIRequest.Input) > 0
	hasPrompt := responseAPIRequest.Prompt != nil

	if !hasInput && !hasPrompt {
		return nil, errors.New("either input or prompt is required")
	}
	if hasInput && hasPrompt {
		return nil, errors.New("input and prompt are mutually exclusive - provide only one")
	}

	return responseAPIRequest, nil
}

// getResponseAPIPromptTokens estimates prompt tokens for Response API requests
func getResponseAPIPromptTokens(ctx context.Context, responseAPIRequest *openai.ResponseAPIRequest) int {
	// For now, use a simple estimation based on input content
	// This will be improved with proper token counting
	totalTokens := 0

	// Count tokens from input array (if present)
	for _, input := range responseAPIRequest.Input {
		switch v := input.(type) {
		case map[string]any:
			if content, ok := v["content"].(string); ok {
				// Simple estimation: ~4 characters per token
				totalTokens += len(content) / 4
			}
		case string:
			totalTokens += len(v) / 4
		}
	}

	// Count tokens from prompt template (if present)
	if responseAPIRequest.Prompt != nil {
		// Estimate tokens for prompt template ID (small fixed cost)
		totalTokens += 10

		// Count tokens from prompt variables
		for _, value := range responseAPIRequest.Prompt.Variables {
			switch v := value.(type) {
			case string:
				totalTokens += len(v) / 4
			case map[string]any:
				// For complex variables like input_file, add a fixed cost
				totalTokens += 20
			}
		}
	}

	// Add instruction tokens if present
	if responseAPIRequest.Instructions != nil {
		totalTokens += len(*responseAPIRequest.Instructions) / 4
	}

	// Minimum token count
	if totalTokens < 10 {
		totalTokens = 10
	}

	return totalTokens
}

// sanitizeResponseAPIRequest sanitizes Response API request by removing parameters not supported by reasoning models
func sanitizeResponseAPIRequest(request *openai.ResponseAPIRequest, channelType int) {
	if request == nil {
		return
	}
	modelName := strings.TrimSpace(strings.ToLower(request.Model))

	if isReasoningModel(modelName) {
		request.Temperature = nil
		request.TopP = nil
	}

	if request.Text != nil && request.Text.Format != nil && request.Text.Format.Schema != nil {
		if normalized, changed := openai.NormalizeStructuredJSONSchema(request.Text.Format.Schema, channelType); changed {
			request.Text.Format.Schema = normalized
		}
	}

	for idx := range request.Tools {
		tool := &request.Tools[idx]
		toolType := strings.ToLower(strings.TrimSpace(tool.Type))
		if (toolType == "web_search_preview" || toolType == "web_search_preview_reasoning" || toolType == "web_search_preview_non_reasoning") && channelType != channeltype.OpenAI {
			tool.Type = "web_search"
		}
		if tool.Parameters != nil {
			tool.Parameters, _ = openai.NormalizeStructuredJSONSchema(tool.Parameters, channelType)
		}
		if tool.Function != nil {
			if params, ok := tool.Function.Parameters.(map[string]any); ok && params != nil {
				if normalized, changed := openai.NormalizeStructuredJSONSchema(params, channelType); changed {
					tool.Function.Parameters = normalized
				}
			}
		}
	}
}

// sanitizeChatCompletionRequest sanitizes Chat Completion request by removing parameters not supported by reasoning models
func sanitizeChatCompletionRequest(request *relaymodel.GeneralOpenAIRequest) {
	if request == nil {
		return
	}
	modelName := strings.TrimSpace(strings.ToLower(request.Model))

	if isReasoningModel(modelName) {
		request.Temperature = nil
		request.TopP = nil
	}
}

// supportsNativeResponseAPI checks if the channel supports Response API natively
func supportsNativeResponseAPI(meta *metalib.Meta) bool {
	if meta == nil {
		return false
	}

	modelName := strings.TrimSpace(strings.ToLower(meta.ActualModelName))
	if modelName == "" {
		modelName = strings.TrimSpace(strings.ToLower(meta.OriginModelName))
	}
	if modelName != "" && openai.IsModelsOnlySupportedByChatCompletionAPI(modelName) {
		return false
	}

	if isDeepSeekModel(meta.ActualModelName) || isDeepSeekModel(meta.OriginModelName) {
		return false
	}

	switch meta.ChannelType {
	case channeltype.OpenAI:
		base := strings.TrimSpace(strings.ToLower(meta.BaseURL))
		if base == "" {
			return true
		}
		return strings.Contains(base, "api.openai.com")
	case channeltype.Azure:
		return openai.AzureRequiresResponseAPI(meta.ActualModelName)
	case channeltype.XAI:
		// XAI supports Response API natively
		return true
	case channeltype.OpenAICompatible:
		return channeltype.UseOpenAICompatibleResponseAPI(meta.Config.APIFormat)
	default:
		return false
	}
}

// isDeepSeekModel checks if the model is a DeepSeek model
func isDeepSeekModel(modelName string) bool {
	normalized := strings.TrimSpace(strings.ToLower(modelName))
	if normalized == "" {
		return false
	}
	return strings.HasPrefix(normalized, "deepseek")
}

// isReasoningModel checks if the model is a reasoning model
func isReasoningModel(modelName string) bool {
	if modelName == "" {
		return false
	}
	// Check for reasoning model prefixes (direct model names)
	if strings.HasPrefix(modelName, "gpt-5") ||
		strings.HasPrefix(modelName, "o1") ||
		strings.HasPrefix(modelName, "o3") ||
		strings.HasPrefix(modelName, "o4") ||
		strings.HasPrefix(modelName, "o-") {
		return true
	}
	// Also check for prefixed model names (e.g., "azure-gpt-5-nano", "vertex-o1-mini")
	// These are user-facing aliases that map to reasoning models.
	return strings.Contains(modelName, "-gpt-5") ||
		strings.Contains(modelName, "-o1-") ||
		strings.Contains(modelName, "-o3-") ||
		strings.Contains(modelName, "-o4-") ||
		strings.HasSuffix(modelName, "-o1") ||
		strings.HasSuffix(modelName, "-o3") ||
		strings.HasSuffix(modelName, "-o4")
}

// applyResponseAPIStreamParams applies stream parameters from query to meta
func applyResponseAPIStreamParams(c *gin.Context, meta *metalib.Meta) error {
	streamParam := c.Query("stream")
	if streamParam == "" {
		meta.IsStream = false
		return nil
	}

	stream, err := strconv.ParseBool(streamParam)
	if err != nil {
		return errors.Wrap(err, "parse stream query parameter")
	}
	meta.IsStream = stream
	return nil
}
