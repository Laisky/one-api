package openai

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v6"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	imgutil "github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/alibailian"
	"github.com/songquanpeng/one-api/relay/adaptor/baiduv2"
	"github.com/songquanpeng/one-api/relay/adaptor/doubao"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/songquanpeng/one-api/relay/adaptor/minimax"
	"github.com/songquanpeng/one-api/relay/adaptor/novita"
	"github.com/songquanpeng/one-api/relay/billing/ratio"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

type Adaptor struct {
	// failed to inline image URL; sending original URL upstream
	// logger.Logger.Warn("failed to inline image URL; sending original URL upstream",
	//     zap.String("url", url),
	//     zap.Error(err))
	ChannelType int
}

// webSearchCallUSDPerThousand returns the USD cost per 1000 calls for the given model name
//
//   - https://openai.com/api/pricing/
//   - https://platform.openai.com/docs/pricing#built-in-tools
func webSearchCallUSDPerThousand(modelName string) float64 {
	lower := normalizedModelName(modelName)

	if isModelSupportedReasoning(lower) {
		return 10.0
	}

	if isWebSearchPreviewModel(lower) {
		return 25.0
	}

	if strings.Contains(lower, "-web-search") || strings.Contains(lower, "-search") {
		return 10.0
	}

	return 25.0
}

func normalizedModelName(modelName string) string {
	return strings.ToLower(strings.TrimSpace(modelName))
}

func isWebSearchPreviewModel(lower string) bool {
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "search-preview") || strings.Contains(lower, "web-search-preview")
}

func webSearchCallQuotaPerInvocation(modelName string) int64 {
	usd := webSearchCallUSDPerThousand(modelName)
	if usd <= 0 {
		return 0
	}
	return int64(math.Ceil(usd / 1000.0 * ratio.QuotaPerUsd))
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.ChannelType = meta.ChannelType
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.ChannelType {
	case channeltype.Azure:
		// Azure requires a deployment name (model) in the URL.
		if strings.TrimSpace(meta.ActualModelName) == "" {
			return "", errors.Errorf("azure request url build failed: empty ActualModelName for path %q", meta.RequestURLPath)
		}

		defaultVersion := meta.Config.APIVersion

		// https://learn.microsoft.com/en-us/azure/ai-services/openai/how-to/reasoning?tabs=python#api--feature-support
		if strings.HasPrefix(meta.ActualModelName, "o1") ||
			strings.HasPrefix(meta.ActualModelName, "o3") {
			defaultVersion = "2024-12-01-preview"
		}

		if meta.Mode == relaymode.ImagesGenerations {
			// https://learn.microsoft.com/en-us/azure/ai-services/openai/dall-e-quickstart?tabs=dalle3%2Ccommand-line&pivots=rest-api
			// https://{resource_name}.openai.azure.com/openai/deployments/dall-e-3/images/generations?api-version=2024-03-01-preview
			fullRequestURL := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s", meta.BaseURL, meta.ActualModelName, defaultVersion)
			return fullRequestURL, nil
		}

		// https://learn.microsoft.com/en-us/azure/cognitive-services/openai/chatgpt-quickstart?pivots=rest-api&tabs=command-line#rest-api
		requestURL := strings.Split(meta.RequestURLPath, "?")[0]
		requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, defaultVersion)
		task := strings.TrimPrefix(requestURL, "/v1/")
		model_ := meta.ActualModelName
		// https://github.com/songquanpeng/one-api/issues/2235
		// model_ = strings.Replace(model_, ".", "", -1)
		//https://github.com/songquanpeng/one-api/issues/1191
		// {your endpoint}/openai/deployments/{your azure_model}/chat/completions?api-version={api_version}
		requestURL = fmt.Sprintf("/openai/deployments/%s/%s", model_, task)
		return GetFullRequestURL(meta.BaseURL, requestURL, meta.ChannelType), nil
	case channeltype.Minimax:
		return minimax.GetRequestURL(meta)
	case channeltype.Doubao:
		return doubao.GetRequestURL(meta)
	case channeltype.Novita:
		return novita.GetRequestURL(meta)
	case channeltype.BaiduV2:
		return baiduv2.GetRequestURL(meta)
	case channeltype.AliBailian:
		return alibailian.GetRequestURL(meta)
	case channeltype.GeminiOpenAICompatible:
		return geminiOpenaiCompatible.GetRequestURL(meta)
	default:
		// Handle Claude Messages requests - check if model should use Response API
		requestPath := meta.RequestURLPath
		if idx := strings.Index(requestPath, "?"); idx >= 0 {
			requestPath = requestPath[:idx]
		}
		if requestPath == "/v1/messages" {
			// For OpenAI channels, check if the model should use Response API
			if meta.ChannelType == channeltype.OpenAI &&
				!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) &&
				!meta.ResponseAPIFallback {
				responseAPIPath := "/v1/responses"
				return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
			}
			// Otherwise, use ChatCompletion endpoint
			chatCompletionsPath := "/v1/chat/completions"
			return GetFullRequestURL(meta.BaseURL, chatCompletionsPath, meta.ChannelType), nil
		}

		if meta.ChannelType == channeltype.OpenAI &&
			(meta.Mode == relaymode.ChatCompletions || meta.Mode == relaymode.ClaudeMessages) &&
			!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) &&
			!meta.ResponseAPIFallback {
			responseAPIPath := "/v1/responses"
			return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
		}

		return GetFullRequestURL(meta.BaseURL, meta.RequestURLPath, meta.ChannelType), nil
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Request, meta *meta.Meta) error {
	adaptor.SetupCommonRequestHeader(c, req, meta)
	if meta.ChannelType == channeltype.Azure {
		req.Header.Set("api-key", meta.APIKey)
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+meta.APIKey)
	if meta.ChannelType == channeltype.OpenRouter {
		req.Header.Set("HTTP-Referer", "https://github.com/Laisky/one-api")
		req.Header.Set("X-Title", "One API")
	}
	return nil
}

func (a *Adaptor) ConvertRequest(c *gin.Context, relayMode int, request *model.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	meta := meta.GetByContext(c)
	// Add debug info for conversion path
	// This helps diagnose cases where model name may be missing or conversion selects Response API unexpectedly.
	// Note: use request.Model for origin field and meta.ActualModelName for resolved model.
	if config.DebugEnabled {
		// avoid heavy logs by omitting full request body here
	}

	// Handle direct Response API requests
	if relayMode == relaymode.ResponseAPI {
		// Apply transformations (e.g., image URL -> base64) and pass through
		if err := a.applyRequestTransformations(meta, request); err != nil {
			return nil, errors.Wrap(err, "apply request transformations for Response API")
		}
		logConvertedRequest(c, meta, relayMode, request)
		return request, nil
	}

	// Apply existing transformations for other modes before determining conversion strategy
	if err := a.applyRequestTransformations(meta, request); err != nil {
		return nil, errors.Wrap(err, "apply request transformations")
	}

	if (relayMode == relaymode.ChatCompletions || relayMode == relaymode.ClaudeMessages) &&
		meta.ChannelType == channeltype.OpenAI &&
		!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) &&
		!meta.ResponseAPIFallback {
		responseAPIRequest := ConvertChatCompletionToResponseAPI(request)
		logConvertedRequest(c, meta, relayMode, responseAPIRequest)
		return responseAPIRequest, nil
	}

	logConvertedRequest(c, meta, relayMode, request)
	return request, nil
}

// isModelSupportedReasoning checks if the model supports reasoning features
func isModelSupportedReasoning(modelName string) bool {
	switch {
	case strings.HasPrefix(modelName, "o"),
		strings.HasPrefix(modelName, "gpt-5") && !strings.HasPrefix(modelName, "gpt-5-chat"):
		return true
	default:
		return false
	}
}

// isWebSearchModel returns true when the upstream OpenAI model uses the web search surface
// and therefore rejects parameters like temperature/top_p.
func isWebSearchModel(modelName string) bool {
	return strings.Contains(modelName, "-search") || strings.Contains(modelName, "-search-preview")
}

func isDeepResearchModel(modelName string) bool {
	return strings.Contains(modelName, "deep-research")
}

func defaultReasoningEffortForModel(modelName string) string {
	if isDeepResearchModel(modelName) {
		return "medium"
	}
	return "high"
}

func isReasoningEffortAllowedForModel(modelName, effort string) bool {
	if effort == "" {
		return false
	}
	if isDeepResearchModel(modelName) {
		return effort == "medium"
	}
	switch effort {
	case "low", "medium", "high":
		return true
	default:
		return false
	}
}

func normalizeReasoningEffortForModel(modelName string, effort *string) *string {
	defaultEffort := defaultReasoningEffortForModel(modelName)
	if effort == nil {
		return stringRef(defaultEffort)
	}
	normalized := strings.ToLower(strings.TrimSpace(*effort))
	if !isReasoningEffortAllowedForModel(modelName, normalized) {
		return stringRef(defaultEffort)
	}
	return stringRef(normalized)
}

func stringRef(value string) *string {
	return &value
}

func generalToolSummary(tools []model.Tool) (bool, []string) {
	if len(tools) == 0 {
		return false, nil
	}
	hasWebSearch := false
	types := make([]string, 0, len(tools))
	for _, tool := range tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeName == "" && tool.Function != nil {
			typeName = "function"
		}
		if typeName == "" {
			typeName = "unknown"
		}
		types = append(types, typeName)
		if typeName == "web_search" {
			hasWebSearch = true
		}
	}
	return hasWebSearch, types
}

func responseAPIToolSummary(tools []ResponseAPITool) (bool, []string) {
	if len(tools) == 0 {
		return false, nil
	}
	hasWebSearch := false
	types := make([]string, 0, len(tools))
	for _, tool := range tools {
		typeName := strings.ToLower(strings.TrimSpace(tool.Type))
		if typeName == "" {
			typeName = "unknown"
		}
		types = append(types, typeName)
		if strings.HasPrefix(typeName, "web_search") {
			hasWebSearch = true
		}
	}
	return hasWebSearch, types
}

func logConvertedRequest(c *gin.Context, metaInfo *meta.Meta, relayMode int, payload any) {
	if c == nil {
		return
	}
	lg := gmw.GetLogger(c)
	if lg == nil {
		return
	}
	fields := []zap.Field{
		zap.Int("relay_mode", relayMode),
	}
	if metaInfo != nil {
		fields = append(fields,
			zap.String("model", metaInfo.ActualModelName),
			zap.Int("channel_id", metaInfo.ChannelId),
		)
	}
	switch req := payload.(type) {
	case *ResponseAPIRequest:
		hasWebSearch, toolTypes := responseAPIToolSummary(req.Tools)
		fields = append(fields,
			zap.String("payload_type", "response_api"),
			zap.Int("input_items", len(req.Input)),
			zap.Int("tool_count", len(req.Tools)),
			zap.Bool("has_web_search_tool", hasWebSearch),
		)
		if len(toolTypes) > 0 {
			fields = append(fields, zap.Strings("tool_types", toolTypes))
		}
		if req.Reasoning != nil && req.Reasoning.Effort != nil {
			fields = append(fields, zap.String("reasoning_effort", *req.Reasoning.Effort))
		}
		if req.MaxOutputTokens != nil {
			fields = append(fields, zap.Int("max_output_tokens", *req.MaxOutputTokens))
		}
	case *model.GeneralOpenAIRequest:
		hasWebSearch, toolTypes := generalToolSummary(req.Tools)
		fields = append(fields,
			zap.String("payload_type", "chat_completions"),
			zap.Int("message_count", len(req.Messages)),
			zap.Int("tool_count", len(req.Tools)),
			zap.Bool("has_web_search_tool", hasWebSearch),
		)
		if len(toolTypes) > 0 {
			fields = append(fields, zap.Strings("tool_types", toolTypes))
		}
		if req.ReasoningEffort != nil {
			fields = append(fields, zap.String("reasoning_effort", *req.ReasoningEffort))
		}
		if req.MaxCompletionTokens != nil {
			fields = append(fields, zap.Int("max_completion_tokens", *req.MaxCompletionTokens))
		}
	default:
		if payload != nil {
			fields = append(fields, zap.String("payload_type", fmt.Sprintf("%T", payload)))
		}
	}

	lg.Debug("prepared upstream request payload", fields...)
}

func ensureWebSearchTool(request *model.GeneralOpenAIRequest) {
	for _, tool := range request.Tools {
		if strings.EqualFold(tool.Type, "web_search") {
			return
		}
	}

	request.Tools = append(request.Tools, model.Tool{Type: "web_search"})
}

// applyRequestTransformations applies the existing request transformations
func (a *Adaptor) applyRequestTransformations(meta *meta.Meta, request *model.GeneralOpenAIRequest) error {
	if meta != nil {
		meta.EnsureActualModelName(request.Model)
	}

	switch meta.ChannelType {
	case channeltype.OpenRouter:
		includeReasoning := true
		request.IncludeReasoning = &includeReasoning
		if request.Provider == nil || request.Provider.Sort == "" &&
			config.OpenrouterProviderSort != "" {
			if request.Provider == nil {
				request.Provider = &model.RequestProvider{}
			}

			request.Provider.Sort = config.OpenrouterProviderSort
		}
	default:
	}

	if config.EnforceIncludeUsage && request.Stream {
		// always return usage in stream mode
		if request.StreamOptions == nil {
			request.StreamOptions = &model.StreamOptions{}
		}
		request.StreamOptions.IncludeUsage = true
	}

	if request.MaxTokens != 0 {
		// Copy value before zeroing MaxTokens. Previous code took the address of
		// request.MaxTokens then set it to 0, so the pointer observed 0 and was
		// replaced by the default (e.g. 2048). This preserved user intent.
		tmpMaxTokens := request.MaxTokens
		request.MaxCompletionTokens = &tmpMaxTokens
		request.MaxTokens = 0
	}

	// Set default max tokens if not set or invalid, since MaxCompletionTokens cannot be 0
	if request.MaxCompletionTokens == nil || *request.MaxCompletionTokens <= 0 {
		defaultMaxCompletionTokens := config.DefaultMaxToken
		request.MaxCompletionTokens = &defaultMaxCompletionTokens
	}

	actualModel := meta.ActualModelName
	if strings.TrimSpace(actualModel) == "" {
		actualModel = request.Model
	}

	// o1/o3/o4/gpt-5 do not support system prompt/temperature variations
	if isModelSupportedReasoning(actualModel) {
		targetsResponseAPI := meta.Mode == relaymode.ResponseAPI ||
			(meta.ChannelType == channeltype.OpenAI && !IsModelsOnlySupportedByChatCompletionAPI(actualModel))

		if targetsResponseAPI {
			request.Temperature = nil
		} else {
			temperature := float64(1)
			request.Temperature = &temperature // Only the default (1) value is supported
		}

		request.TopP = nil
		request.ReasoningEffort = normalizeReasoningEffortForModel(actualModel, request.ReasoningEffort)

		request.Messages = func(raw []model.Message) (filtered []model.Message) {
			for i := range raw {
				if raw[i].Role != "system" {
					filtered = append(filtered, raw[i])
				}
			}

			return
		}(request.Messages)
	}

	// web search models do not support system prompt/max_tokens/temperature overrides
	if isWebSearchModel(actualModel) {
		request.Temperature = nil
		request.TopP = nil
		request.PresencePenalty = nil
		request.N = nil
		request.FrequencyPenalty = nil
	}

	modelName := actualModel

	if isDeepResearchModel(modelName) {
		ensureWebSearchTool(request)
	}

	if request.WebSearchOptions != nil {
		ensureWebSearchTool(request)
	}

	if request.Stream && !config.EnforceIncludeUsage &&
		strings.HasSuffix(request.Model, "-audio") {
		// TODO: Since it is not clear how to implement billing in stream mode,
		// it is temporarily not supported
		return errors.New("set ENFORCE_INCLUDE_USAGE=true to enable stream mode for gpt-4o-audio")
	}

	// Transform image URLs in messages to base64 data URLs to ensure upstream receives embedded data
	for i := range request.Messages {
		parts := request.Messages[i].ParseContent()
		if len(parts) == 0 {
			continue
		}
		changed := false
		for pi := range parts {
			if parts[pi].Type == model.ContentTypeImageURL && parts[pi].ImageURL != nil {
				url := parts[pi].ImageURL.Url
				if url != "" && !strings.HasPrefix(url, "data:image/") {
					if dataURL, err := toDataURL(url); err == nil && dataURL != "" {
						parts[pi].ImageURL.Url = dataURL
						changed = true
					}
				}
			}
		}
		if changed {
			// Replace message content with the normalized parts
			request.Messages[i].Content = parts
		}
	}

	return nil
}

func (a *Adaptor) ConvertImageRequest(_ *gin.Context, request *model.ImageRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	return request, nil
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, request *model.ClaudeRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}

	// Convert Claude Messages API request to OpenAI Chat Completions format
	openaiRequest := &model.GeneralOpenAIRequest{
		Model:       request.Model,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream != nil && *request.Stream,
		Stop:        request.StopSequences,
		Thinking:    request.Thinking,
	}

	// Use MaxCompletionTokens instead of MaxTokens for ClaudeRequest conversion
	if request.MaxTokens != 0 {
		originalMaxTokens := request.MaxTokens
		openaiRequest.MaxCompletionTokens = &originalMaxTokens
		openaiRequest.MaxTokens = 0
	}

	// Convert system prompt
	if request.System != nil {
		switch system := request.System.(type) {
		case string:
			if system != "" {
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: system,
				})
			}
		case []any:
			// For structured system content, extract text parts
			var systemParts []string
			for _, block := range system {
				if blockMap, ok := block.(map[string]any); ok {
					if text, exists := blockMap["text"]; exists {
						if textStr, ok := text.(string); ok {
							systemParts = append(systemParts, textStr)
						}
					}
				}
			}
			if len(systemParts) > 0 {
				systemText := strings.Join(systemParts, "\n")
				openaiRequest.Messages = append(openaiRequest.Messages, model.Message{
					Role:    "system",
					Content: systemText,
				})
			}
		}
	}

	// Convert messages
	for _, msg := range request.Messages {
		openaiMessage := model.Message{
			Role: msg.Role,
		}

		// Convert content based on type
		switch content := msg.Content.(type) {
		case string:
			// Simple string content
			openaiMessage.Content = content
		case []any:
			// Structured content blocks - convert to OpenAI format
			var contentParts []model.MessageContent
			for _, block := range content {
				if blockMap, ok := block.(map[string]any); ok {
					if blockType, exists := blockMap["type"]; exists {
						switch blockType {
						case "text":
							if text, exists := blockMap["text"]; exists {
								if textStr, ok := text.(string); ok {
									contentParts = append(contentParts, model.MessageContent{
										Type: "text",
										Text: &textStr,
									})
								}
							}
						case "image":
							if source, exists := blockMap["source"]; exists {
								if sourceMap, ok := source.(map[string]any); ok {
									imageURL := model.ImageURL{}
									// Support base64 source
									if mediaType, exists := sourceMap["media_type"]; exists {
										if data, exists := sourceMap["data"]; exists {
											if dataStr, ok := data.(string); ok {
												imageURL.Url = fmt.Sprintf("data:%s;base64,%s", mediaType, dataStr)
											}
										}
									}
									// Support URL source -> fetch and inline
									if srcType, ok := sourceMap["type"].(string); ok && srcType == "url" {
										if u, ok := sourceMap["url"].(string); ok && u != "" {
											if dataURL, err := toDataURL(u); err == nil && dataURL != "" {
												imageURL.Url = dataURL
											}
										}
									}
									contentParts = append(contentParts, model.MessageContent{
										Type:     "image_url",
										ImageURL: &imageURL,
									})
								}
							}
						}
					}
				}
			}
			if len(contentParts) > 0 {
				openaiMessage.Content = contentParts
			}
		default:
			// Fallback: convert to string
			if contentBytes, err := json.Marshal(content); err == nil {
				openaiMessage.Content = string(contentBytes)
			}
		}

		openaiRequest.Messages = append(openaiRequest.Messages, openaiMessage)
	}

	// Convert tools
	for _, tool := range request.Tools {
		openaiTool := model.Tool{
			Type: "function",
			Function: &model.Function{
				Name:        tool.Name,
				Description: tool.Description,
			},
		}

		// Convert input schema
		if tool.InputSchema != nil {
			if schemaMap, ok := tool.InputSchema.(map[string]any); ok {
				openaiTool.Function.Parameters = schemaMap
			}
		}

		openaiRequest.Tools = append(openaiRequest.Tools, openaiTool)
	}

	// Convert tool choice
	if request.ToolChoice != nil {
		openaiRequest.ToolChoice = request.ToolChoice
	}

	// Mark this as a Claude Messages conversion for response handling
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	c.Set(ctxkey.OriginalClaudeRequest, request)

	// For OpenAI adaptor, check if we should convert to Response API format
	meta := meta.GetByContext(c)
	if meta.ChannelType == channeltype.OpenAI && !IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) &&
		!meta.ResponseAPIFallback {
		// Apply transformations first
		if err := a.applyRequestTransformations(meta, openaiRequest); err != nil {
			return nil, errors.Wrap(err, "apply request transformations for Claude conversion")
		}

		// Convert to Response API format
		responseAPIRequest := ConvertChatCompletionToResponseAPI(openaiRequest)

		// Store the converted request in context to detect it later in DoResponse
		c.Set(ctxkey.ConvertedRequest, responseAPIRequest)

		return responseAPIRequest, nil
	}

	// For non-OpenAI channels or models that only support ChatCompletion API,
	// return the OpenAI request directly
	return openaiRequest, nil
}

// getImageFromURLFn is injectable for tests
var getImageFromURLFn = imgutil.GetImageFromUrl

// toDataURL downloads an image and returns a data URL string
func toDataURL(url string) (string, error) {
	mime, data, err := getImageFromURLFn(url)
	if err != nil {
		return "", errors.Wrap(err, "get image from url")
	}
	if mime == "" {
		mime = "image/jpeg"
	}
	return fmt.Sprintf("data:%s;base64,%s", mime, data), nil
}

func (a *Adaptor) DoRequest(c *gin.Context,
	meta *meta.Meta,
	requestBody io.Reader) (*http.Response, error) {
	return adaptor.DoRequestHelper(a, c, meta, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context,
	resp *http.Response,
	meta *meta.Meta) (usage *model.Usage,
	err *model.ErrorWithStatusCode) {
	if meta.IsStream {
		var responseText string
		// Handle different streaming modes
		switch meta.Mode {
		case relaymode.ResponseAPI:
			// Direct Response API streaming - pass through without conversion
			err, responseText, usage = ResponseAPIDirectStreamHandler(c, resp, meta.Mode)
		default:
			// Check if we need to handle Response API streaming response for ChatCompletion
			if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
				if _, ok := vi.(*ResponseAPIRequest); ok {
					// This is a Response API streaming response that needs conversion
					err, responseText, usage = ResponseAPIStreamHandler(c, resp, meta.Mode)
				} else {
					// Regular streaming response
					err, responseText, usage = StreamHandler(c, resp, meta.Mode)
				}
			} else {
				// Regular streaming response
				err, responseText, usage = StreamHandler(c, resp, meta.Mode)
			}
		}

		if usage == nil || usage.TotalTokens == 0 {
			usage = ResponseText2Usage(responseText, meta.ActualModelName, meta.PromptTokens)
		}
		if usage.TotalTokens != 0 && usage.PromptTokens == 0 { // some channels don't return prompt tokens & completion tokens
			usage.PromptTokens = meta.PromptTokens
			usage.CompletionTokens = usage.TotalTokens - meta.PromptTokens
		}
	} else {
		switch meta.Mode {
		case relaymode.ImagesGenerations,
			relaymode.ImagesEdits:
			err, usage = ImageHandler(c, resp)
		// case relaymode.ImagesEdits:
		// err, usage = ImagesEditsHandler(c, resp)
		case relaymode.ResponseAPI:
			// For direct Response API requests, pass through the response directly
			// without conversion back to ChatCompletion format
			err, usage = ResponseAPIDirectHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
		case relaymode.ChatCompletions:
			// Check if we need to convert Response API response back to ChatCompletion format
			if vi, ok := c.Get(ctxkey.ConvertedRequest); ok {
				if _, ok := vi.(*ResponseAPIRequest); ok {
					// This is a Response API response that needs conversion
					err, usage = ResponseAPIHandler(c, resp, meta.PromptTokens, meta.ActualModelName)
				} else {
					// Regular ChatCompletion request
					err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
				}
			} else {
				// Regular ChatCompletion request
				err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
			}
		default:
			err, usage = Handler(c, resp, meta.PromptTokens, meta.ActualModelName)
		}
	}

	if errCost := applyWebSearchToolCost(c, &usage, meta); errCost != nil {
		return nil, errCost
	}

	// Handle Claude Messages response conversion
	if isClaudeConversion, exists := c.Get(ctxkey.ClaudeMessagesConversion); exists && isClaudeConversion.(bool) {
		claudeResp, convertErr := a.convertToClaudeResponse(c, resp)
		if convertErr != nil {
			return nil, convertErr
		}

		// Replace the original response with the converted Claude response
		// We need to update the response in the context so the controller can use it
		c.Set(ctxkey.ConvertedResponse, claudeResp)

		// For Claude Messages conversion, we don't return usage separately
		// The usage is included in the Claude response body, so return nil usage
		return nil, nil
	}

	return
}

func applyWebSearchToolCost(c *gin.Context, usage **model.Usage, meta *meta.Meta) *model.ErrorWithStatusCode {
	if usage == nil || meta == nil {
		return nil
	}

	ensureUsage := func() *model.Usage {
		if *usage == nil {
			*usage = &model.Usage{}
		}
		return *usage
	}

	modelName := meta.ActualModelName
	perCallQuota := webSearchCallQuotaPerInvocation(modelName)

	if callCountAny, ok := c.Get(ctxkey.WebSearchCallCount); ok {
		count := 0
		switch v := callCountAny.(type) {
		case int:
			count = v
		case int32:
			count = int(v)
		case int64:
			count = int(v)
		case float64:
			count = int(v)
		}
		if count < 0 {
			count = 0
		}
		if count > 0 {
			usagePtr := ensureUsage()
			enforceWebSearchTokenPolicy(usagePtr, modelName, count)
			if perCallQuota > 0 {
				usagePtr.ToolsCost += int64(count) * perCallQuota
			}
		}
	}

	return nil
}

func enforceWebSearchTokenPolicy(usage *model.Usage, modelName string, callCount int) {
	if usage == nil || callCount <= 0 {
		return
	}

	_ = modelName // model retained for possible future policy tuning
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
}

// convertToClaudeResponse converts OpenAI response format to Claude Messages format
func (a *Adaptor) convertToClaudeResponse(c *gin.Context, resp *http.Response) (*http.Response, *model.ErrorWithStatusCode) {
	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	resp.Body.Close()

	// Check if it's a streaming response
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		// Handle streaming response conversion
		return a.convertStreamingToClaudeResponse(c, resp, body)
	}

	// Handle non-streaming response conversion
	return a.convertNonStreamingToClaudeResponse(c, resp, body)
}

// convertNonStreamingToClaudeResponse converts a non-streaming OpenAI response to Claude format
func (a *Adaptor) convertNonStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	// First try to parse as Response API format
	var responseAPIResp ResponseAPIResponse
	if err := json.Unmarshal(body, &responseAPIResp); err == nil && responseAPIResp.Object == "response" {
		// This is a Response API response, convert it to Claude format
		return a.ConvertResponseAPIToClaudeResponse(c, resp, &responseAPIResp)
	}

	// Fall back to ChatCompletion API format
	var openaiResp TextResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		// If it's an error response, pass it through
		newResp := &http.Response{
			StatusCode: resp.StatusCode,
			Header:     resp.Header,
			Body:       io.NopCloser(bytes.NewReader(body)),
		}
		return newResp, nil
	}

	// Convert to Claude Messages format
	claudeResp := model.ClaudeResponse{
		ID:      openaiResp.Id,
		Type:    "message",
		Role:    "assistant",
		Model:   openaiResp.Model,
		Content: []model.ClaudeContent{},
		Usage: model.ClaudeUsage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		},
		StopReason: "end_turn",
	}

	// Convert choices to content
	for _, choice := range openaiResp.Choices {
		if choice.Message.Content != nil {
			switch content := choice.Message.Content.(type) {
			case string:
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type: "text",
					Text: content,
				})
			case []model.MessageContent:
				// Handle structured content
				for _, part := range content {
					if part.Type == "text" && part.Text != nil {
						claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
							Type: "text",
							Text: *part.Text,
						})
					}
				}
			}
		}

		// Handle tool calls
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				var input json.RawMessage
				if toolCall.Function.Arguments != nil {
					if argsStr, ok := toolCall.Function.Arguments.(string); ok {
						input = json.RawMessage(argsStr)
					} else if argsBytes, err := json.Marshal(toolCall.Function.Arguments); err == nil {
						input = json.RawMessage(argsBytes)
					}
				}
				claudeResp.Content = append(claudeResp.Content, model.ClaudeContent{
					Type:  "tool_use",
					ID:    toolCall.Id,
					Name:  toolCall.Function.Name,
					Input: input,
				})
			}
		}

		// Set stop reason based on finish reason
		switch choice.FinishReason {
		case "stop":
			claudeResp.StopReason = "end_turn"
		case "length":
			claudeResp.StopReason = "max_tokens"
		case "tool_calls":
			claudeResp.StopReason = "tool_use"
		case "content_filter":
			claudeResp.StopReason = "stop_sequence"
		}
	}

	// Marshal the Claude response
	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
	}

	// Create new response with Claude format
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(claudeBody)),
	}

	// Copy headers but update content type
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(claudeBody)))

	return newResp, nil
}

// ConvertResponseAPIToClaudeResponse converts a Response API response to Claude Messages format
func (a *Adaptor) ConvertResponseAPIToClaudeResponse(c *gin.Context, resp *http.Response, responseAPIResp *ResponseAPIResponse) (*http.Response, *model.ErrorWithStatusCode) {
	// Convert to Claude Messages format
	claudeResp := model.ClaudeResponse{
		ID:         responseAPIResp.Id,
		Type:       "message",
		Role:       "assistant",
		Model:      responseAPIResp.Model,
		Content:    []model.ClaudeContent{},
		StopReason: "end_turn",
	}

	// Convert usage if present
	if responseAPIResp.Usage != nil {
		claudeResp.Usage = model.ClaudeUsage{
			InputTokens:  responseAPIResp.Usage.InputTokens,
			OutputTokens: responseAPIResp.Usage.OutputTokens,
		}
	}

	// Convert output items to Claude content
	for _, outputItem := range responseAPIResp.Output {
		if outputItem.Type == "reasoning" {
			// Convert reasoning content to Claude thinking format
			for _, summary := range outputItem.Summary {
				if summary.Type == "summary_text" && summary.Text != "" {
					claudeContent := model.ClaudeContent{
						Type:     "thinking",
						Thinking: summary.Text,
					}
					claudeResp.Content = append(claudeResp.Content, claudeContent)
				}
			}
		} else if outputItem.Type == "message" && outputItem.Role == "assistant" {
			// Convert message content
			for _, content := range outputItem.Content {
				if content.Type == "output_text" && content.Text != "" {
					claudeContent := model.ClaudeContent{
						Type: "text",
						Text: content.Text,
					}
					claudeResp.Content = append(claudeResp.Content, claudeContent)
				}
			}
		}
	}

	// Marshal the Claude response
	claudeBody, err := json.Marshal(claudeResp)
	if err != nil {
		return nil, ErrorWrapper(err, "marshal_claude_response_failed", http.StatusInternalServerError)
	}

	// Create new response with Claude format
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(claudeBody)),
	}

	// Copy headers but update content type
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}
	newResp.Header.Set("Content-Type", "application/json")
	newResp.Header.Set("Content-Length", fmt.Sprintf("%d", len(claudeBody)))

	return newResp, nil
}

// convertStreamingToClaudeResponse converts a streaming OpenAI response to Claude format
func (a *Adaptor) convertStreamingToClaudeResponse(c *gin.Context, resp *http.Response, body []byte) (*http.Response, *model.ErrorWithStatusCode) {
	// For streaming responses, we need to convert each SSE event
	// This is more complex and would require parsing SSE events and converting them
	// For now, we'll create a simple streaming converter

	reader, writer := io.Pipe()

	go func() {
		defer writer.Close()

		// Parse SSE events from the body and convert them
		scanner := bufio.NewScanner(bytes.NewReader(body))
		for scanner.Scan() {
			line := scanner.Text()

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				if data == "[DONE]" {
					// Send Claude-style done event
					writer.Write([]byte("event: message_stop\n"))
					writer.Write([]byte("data: {\"type\":\"message_stop\"}\n\n"))
					break
				}

				// Parse OpenAI streaming chunk
				var chunk ChatCompletionsStreamResponse
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					continue
				}

				// Convert to Claude streaming format
				if len(chunk.Choices) > 0 {
					choice := chunk.Choices[0]
					if choice.Delta.Content != nil {
						claudeChunk := map[string]interface{}{
							"type":  "content_block_delta",
							"index": 0,
							"delta": map[string]interface{}{
								"type": "text_delta",
								"text": choice.Delta.Content,
							},
						}

						claudeData, _ := json.Marshal(claudeChunk)
						writer.Write([]byte("event: content_block_delta\n"))
						writer.Write([]byte(fmt.Sprintf("data: %s\n\n", claudeData)))
					}
				}
			}
		}
	}()

	// Create new streaming response
	newResp := &http.Response{
		StatusCode: resp.StatusCode,
		Header:     make(http.Header),
		Body:       reader,
	}

	// Copy headers
	for k, v := range resp.Header {
		newResp.Header[k] = v
	}

	return newResp, nil
}

func (a *Adaptor) GetModelList() []string {
	return adaptor.GetModelListFromPricing(ModelRatios)
}

func (a *Adaptor) GetChannelName() string {
	channelName, _ := GetCompatibleChannelMeta(a.ChannelType)
	return channelName
}

// Pricing methods - OpenAI adapter manages its own model pricing
func (a *Adaptor) GetDefaultModelPricing() map[string]adaptor.ModelConfig {
	return ModelRatios
}

func (a *Adaptor) GetModelRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.Ratio
	}
	// Fallback to global pricing for unknown models
	return ratio.GetModelRatio(modelName, a.ChannelType)
}

func (a *Adaptor) GetCompletionRatio(modelName string) float64 {
	pricing := a.GetDefaultModelPricing()
	if price, exists := pricing[modelName]; exists {
		return price.CompletionRatio
	}
	// Fallback to global pricing for unknown models
	return ratio.GetCompletionRatio(modelName, a.ChannelType)
}
