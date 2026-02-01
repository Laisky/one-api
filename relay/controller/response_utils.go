package controller

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
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

// getResponseAPIPromptTokens estimates prompt tokens for Response API requests.
func getResponseAPIPromptTokens(ctx context.Context, responseAPIRequest *openai.ResponseAPIRequest) int {
	if responseAPIRequest == nil {
		return 0
	}
	totalTokens := 0
	totalTokens += countResponseAPIInstructionsTokens(ctx, responseAPIRequest)
	totalTokens += countResponseAPIInputTokens(ctx, responseAPIRequest.Input, responseAPIRequest.Model)
	if responseAPIRequest.Prompt != nil {
		totalTokens += countResponseAPIPromptTemplateTokens(ctx, responseAPIRequest.Prompt, responseAPIRequest.Model)
	}

	if totalTokens < 10 {
		totalTokens = 10
	}
	return totalTokens
}

// countResponseAPIInstructionsTokens counts tokens in the instructions field.
// Parameters: ctx is the request context; request is the Response API request.
// Returns: the token count for instructions, or 0 if absent.
func countResponseAPIInstructionsTokens(ctx context.Context, request *openai.ResponseAPIRequest) int {
	if request == nil || request.Instructions == nil || strings.TrimSpace(*request.Instructions) == "" {
		return 0
	}
	return openai.CountTokenText(*request.Instructions, request.Model)
}

// countResponseAPIPromptTemplateTokens counts tokens for prompt templates and variables.
// Parameters: ctx is the request context; prompt is the prompt template; model is the target model name.
// Returns: the estimated token count for the prompt template.
func countResponseAPIPromptTemplateTokens(ctx context.Context, prompt *openai.ResponseAPIPrompt, model string) int {
	if prompt == nil {
		return 0
	}
	total := 0
	if prompt.Id != "" {
		total += openai.CountTokenText(prompt.Id, model)
	}
	for _, value := range prompt.Variables {
		total += countResponseAPIValueTokens(ctx, value, model)
	}
	return total
}

// countResponseAPIInputTokens counts tokens for Response API input items.
// Parameters: ctx is the request context; input is the Response API input list; model is the target model name.
// Returns: the estimated token count for the input payload.
func countResponseAPIInputTokens(ctx context.Context, input openai.ResponseAPIInput, model string) int {
	total := 0
	for _, item := range input {
		total += countResponseAPIInputItemTokens(ctx, item, model)
	}
	return total
}

// countResponseAPIInputItemTokens counts tokens for a single Response API input item.
// Parameters: ctx is the request context; item is the input element; model is the target model name.
// Returns: the estimated token count for the item.
func countResponseAPIInputItemTokens(ctx context.Context, item any, model string) int {
	switch v := item.(type) {
	case string:
		return openai.CountTokenText(v, model)
	case map[string]any:
		return countResponseAPIInputMapTokens(ctx, v, model)
	default:
		return countResponseAPIValueTokens(ctx, v, model)
	}
}

// countResponseAPIInputMapTokens counts tokens for map-shaped Response API input items.
// Parameters: ctx is the request context; itemMap is the input object; model is the target model name.
// Returns: the estimated token count for the input object.
func countResponseAPIInputMapTokens(ctx context.Context, itemMap map[string]any, model string) int {
	total := 0
	role, _ := itemMap["role"].(string)
	name, _ := itemMap["name"].(string)
	if role != "" {
		tokensPerMessage, tokensPerName := responseMessageTokenOverhead(model)
		total += tokensPerMessage
		total += openai.CountTokenText(role, model)
		if name != "" {
			total += tokensPerName
			total += openai.CountTokenText(name, model)
		}
	}

	if typeVal, ok := itemMap["type"].(string); ok {
		switch strings.ToLower(typeVal) {
		case "function_call":
			if nameVal, ok := itemMap["name"].(string); ok && nameVal != "" {
				total += openai.CountTokenText(nameVal, model)
			}
			if args, ok := itemMap["arguments"]; ok {
				total += countResponseAPIValueTokens(ctx, args, model)
			}
			return total
		case "function_call_output":
			if output, ok := itemMap["output"]; ok {
				total += countResponseAPIValueTokens(ctx, output, model)
			} else if content, ok := itemMap["content"]; ok {
				total += countResponseAPIValueTokens(ctx, content, model)
			}
			return total
		}
	}

	if content, ok := itemMap["content"]; ok {
		total += countResponseAPIContentTokens(ctx, content, model)
		return total
	}

	if typeVal, ok := itemMap["type"].(string); ok {
		partTokens := countResponseAPIContentPartTokens(ctx, itemMap, typeVal, model)
		if partTokens > 0 {
			total += partTokens
			return total
		}
	}

	total += countResponseAPIValueTokens(ctx, itemMap, model)
	return total
}

// countResponseAPIContentTokens counts tokens for a Response API content field.
// Parameters: ctx is the request context; content is the content payload; model is the target model name.
// Returns: the estimated token count for the content payload.
func countResponseAPIContentTokens(ctx context.Context, content any, model string) int {
	switch v := content.(type) {
	case string:
		return openai.CountTokenText(v, model)
	case []any:
		total := 0
		for _, raw := range v {
			if partMap, ok := raw.(map[string]any); ok {
				typeStr, _ := partMap["type"].(string)
				total += countResponseAPIContentPartTokens(ctx, partMap, typeStr, model)
				continue
			}
			total += countResponseAPIValueTokens(ctx, raw, model)
		}
		return total
	default:
		return countResponseAPIValueTokens(ctx, v, model)
	}
}

// countResponseAPIContentPartTokens counts tokens for a single content part item.
// Parameters: ctx is the request context; partMap is the content part map; partType is the declared type; model is the target model name.
// Returns: the estimated token count for the content part.
func countResponseAPIContentPartTokens(ctx context.Context, partMap map[string]any, partType string, model string) int {
	typeStr := strings.ToLower(strings.TrimSpace(partType))
	switch typeStr {
	case "input_text", "output_text":
		if text, ok := partMap["text"].(string); ok && text != "" {
			return openai.CountTokenText(text, model)
		}
	case "input_image":
		url, _ := partMap["image_url"].(string)
		detail, _ := partMap["detail"].(string)
		return countResponseAPIImageTokens(ctx, url, detail, model)
	case "input_audio":
		if inputAudio, ok := partMap["input_audio"].(map[string]any); ok {
			if data, ok := inputAudio["data"].(string); ok && data != "" {
				return countResponseAPIAudioTokens(ctx, data, model)
			}
		}
	}
	if text, ok := partMap["text"].(string); ok && text != "" {
		return openai.CountTokenText(text, model)
	}
	return countResponseAPIValueTokens(ctx, partMap, model)
}

// countResponseAPIImageTokens counts tokens for an input image.
// Parameters: ctx is the request context; url is the image URL; detail is the image detail level; model is the target model name.
// Returns: the estimated token count for the image input.
func countResponseAPIImageTokens(ctx context.Context, url string, detail string, model string) int {
	if url == "" {
		return 0
	}
	lg := gmw.GetLogger(ctx)
	tokens, err := openai.CountImageTokens(url, detail, model)
	if err != nil {
		isDataURL := strings.HasPrefix(url, "data:image/")
		b64Len := 0
		if isDataURL {
			if idx := strings.Index(url, ","); idx >= 0 && idx+1 < len(url) {
				b64Len = len(url[idx+1:])
			}
		}
		if lg != nil {
			lg.Debug("response api image token count failed",
				zap.Error(err),
				zap.String("model", model),
				zap.Bool("data_url", isDataURL),
				zap.Int("base64_len", b64Len),
				zap.String("detail", detail),
			)
		}
		return 0
	}
	return tokens
}

// countResponseAPIAudioTokens counts tokens for base64 audio inputs.
// Parameters: ctx is the request context; base64Data is the audio payload; model is the target model name.
// Returns: the estimated token count for the audio input.
func countResponseAPIAudioTokens(ctx context.Context, base64Data string, model string) int {
	lg := gmw.GetLogger(ctx)
	tokens, err := openai.CountInputAudioTokens(ctx, base64Data, model)
	if err != nil {
		if lg != nil {
			lg.Debug("response api audio token count failed",
				zap.Error(err),
				zap.String("model", model),
			)
		}
		return 0
	}
	return tokens
}

// countResponseAPIValueTokens counts tokens for arbitrary values by serializing to JSON when needed.
// Parameters: ctx is the request context; value is the payload; model is the target model name.
// Returns: the estimated token count for the value.
func countResponseAPIValueTokens(ctx context.Context, value any, model string) int {
	lg := gmw.GetLogger(ctx)
	switch v := value.(type) {
	case string:
		return openai.CountTokenText(v, model)
	case nil:
		return 0
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			if lg != nil {
				lg.Debug("response api value token count fallback",
					zap.Error(err),
					zap.String("model", model),
				)
			}
			return 0
		}
		return openai.CountTokenText(string(raw), model)
	}
}

// responseMessageTokenOverhead returns per-message overhead values for chat-like inputs.
// Parameters: model is the target model name.
// Returns: tokensPerMessage and tokensPerName to approximate chat-style overhead.
func responseMessageTokenOverhead(model string) (int, int) {
	if model == "gpt-3.5-turbo-0301" {
		return 4, -1
	}
	return 3, 1
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
