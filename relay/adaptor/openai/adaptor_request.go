package openai

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Laisky/errors/v2"
	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/songquanpeng/one-api/common/config"
	"github.com/songquanpeng/one-api/common/ctxkey"
	"github.com/songquanpeng/one-api/common/image"
	"github.com/songquanpeng/one-api/relay/adaptor"
	"github.com/songquanpeng/one-api/relay/adaptor/alibailian"
	"github.com/songquanpeng/one-api/relay/adaptor/baiduv2"
	"github.com/songquanpeng/one-api/relay/adaptor/doubao"
	"github.com/songquanpeng/one-api/relay/adaptor/geminiOpenaiCompatible"
	"github.com/songquanpeng/one-api/relay/adaptor/minimax"
	"github.com/songquanpeng/one-api/relay/adaptor/novita"
	"github.com/songquanpeng/one-api/relay/adaptor/openai_compatible"
	"github.com/songquanpeng/one-api/relay/channeltype"
	"github.com/songquanpeng/one-api/relay/meta"
	"github.com/songquanpeng/one-api/relay/model"
	"github.com/songquanpeng/one-api/relay/relaymode"
)

// azureRequiresResponseAPI returns true when Azure supports the model only via the Response API.
func azureRequiresResponseAPI(modelName string) bool {
	normalized := normalizedModelName(modelName)
	return strings.HasPrefix(normalized, "gpt-5")
}

// AzureRequiresResponseAPI reports whether an Azure deployment must be called via the Response API surface.
// Exposed so controllers can keep conversion heuristics in sync with the adaptor's routing logic.
func AzureRequiresResponseAPI(modelName string) bool {
	return azureRequiresResponseAPI(modelName)
}

// shouldForceResponseAPI reports whether the upstream request must use the Response API surface.
func shouldForceResponseAPI(metaInfo *meta.Meta) bool {
	if metaInfo == nil {
		return false
	}
	switch metaInfo.ChannelType {
	case channeltype.OpenAI:
		if metaInfo.ResponseAPIFallback {
			return false
		}
		return !IsModelsOnlySupportedByChatCompletionAPI(metaInfo.ActualModelName)
	case channeltype.Azure:
		return azureRequiresResponseAPI(metaInfo.ActualModelName)
	case channeltype.OpenAICompatible:
		if metaInfo.ResponseAPIFallback {
			return false
		}
		if openai_compatible.IsGitHubModelsBaseURL(metaInfo.BaseURL) {
			return false
		}
		return channeltype.UseOpenAICompatibleResponseAPI(metaInfo.Config.APIFormat)
	default:
		return false
	}
}

func normalizedModelName(modelName string) string {
	return strings.ToLower(strings.TrimSpace(modelName))
}

func (a *Adaptor) Init(meta *meta.Meta) {
	a.ChannelType = meta.ChannelType
}

func (a *Adaptor) GetRequestURL(meta *meta.Meta) (string, error) {
	switch meta.ChannelType {
	case channeltype.Azure:
		if strings.TrimSpace(meta.ActualModelName) == "" {
			return "", errors.Errorf("azure request url build failed: empty ActualModelName for path %q", meta.RequestURLPath)
		}

		defaultVersion := meta.Config.APIVersion
		if strings.HasPrefix(meta.ActualModelName, "o1") ||
			strings.HasPrefix(meta.ActualModelName, "o4") ||
			strings.HasPrefix(meta.ActualModelName, "o3") {
			defaultVersion = "2025-04-01-preview"
		} else if azureRequiresResponseAPI(meta.ActualModelName) {
			defaultVersion = "v1"
		}

		if meta.Mode == relaymode.ImagesGenerations {
			fullRequestURL := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s", meta.BaseURL, meta.ActualModelName, defaultVersion)
			return fullRequestURL, nil
		}

		useResponseAPI := meta.Mode == relaymode.ResponseAPI || azureRequiresResponseAPI(meta.ActualModelName)
		requestPath := meta.RequestURLPath
		if idx := strings.Index(requestPath, "?"); idx >= 0 {
			requestPath = requestPath[:idx]
		}
		if strings.TrimSpace(requestPath) == "" {
			requestPath = "/v1/chat/completions"
		}
		if useResponseAPI {
			requestURL := "/openai/v1/responses"
			if strings.TrimSpace(defaultVersion) != "" {
				requestURL = fmt.Sprintf("%s?api-version=%s", requestURL, defaultVersion)
			}
			return GetFullRequestURL(meta.BaseURL, requestURL, meta.ChannelType), nil
		}
		task := strings.TrimPrefix(requestPath, "/")
		task = strings.TrimPrefix(task, "v1/")
		model_ := meta.ActualModelName
		requestURL := fmt.Sprintf("/openai/deployments/%s/%s?api-version=%s", model_, task, defaultVersion)
		return GetFullRequestURL(meta.BaseURL, requestURL, meta.ChannelType), nil
	case channeltype.OpenAICompatible:
		requestPath := strings.TrimSpace(meta.RequestURLPath)
		query := ""
		if idx := strings.Index(requestPath, "?"); idx >= 0 {
			query = requestPath[idx:]
			requestPath = requestPath[:idx]
		}

		format := channeltype.NormalizeOpenAICompatibleAPIFormat(meta.Config.APIFormat)
		isGitHub := openai_compatible.IsGitHubModelsBaseURL(meta.BaseURL)

		if meta.Mode == relaymode.ChatCompletions || meta.Mode == relaymode.ClaudeMessages || meta.Mode == relaymode.ResponseAPI {
			if format == channeltype.OpenAICompatibleAPIFormatResponse && !isGitHub {
				requestPath = "/v1/responses"
			} else {
				if requestPath == "" || requestPath == "/" || requestPath == "/v1/responses" || requestPath == "/v1/messages" {
					requestPath = "/v1/chat/completions"
				}
			}
		}

		if isGitHub {
			requestPath = openai_compatible.NormalizeGitHubRequestPath(requestPath, meta.Mode)
		} else if requestPath == "" {
			requestPath = "/v1/chat/completions"
		}

		return GetFullRequestURL(meta.BaseURL, requestPath+query, meta.ChannelType), nil
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
		requestPath := meta.RequestURLPath
		if idx := strings.Index(requestPath, "?"); idx >= 0 {
			requestPath = requestPath[:idx]
		}
		if requestPath == "/v1/messages" {
			if meta.ChannelType == channeltype.OpenAI &&
				!IsModelsOnlySupportedByChatCompletionAPI(meta.ActualModelName) &&
				!meta.ResponseAPIFallback {
				responseAPIPath := "/v1/responses"
				return GetFullRequestURL(meta.BaseURL, responseAPIPath, meta.ChannelType), nil
			}
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

	metaInfo := meta.GetByContext(c)
	lg := gmw.GetLogger(c)
	if shouldNormalizeToolMessageContentForDeepSeek(metaInfo, request) {
		normalizeClaudeThinkingForDeepSeek(lg, request)
		normalizeDeepSeekToolMessageContent(lg, request)
	}

	if config.DebugEnabled {
	}

	if relayMode == relaymode.ResponseAPI {
		if err := a.applyRequestTransformations(metaInfo, request); err != nil {
			return nil, errors.Wrap(err, "apply request transformations for Response API")
		}
		logConvertedRequest(c, metaInfo, relayMode, request)
		return request, nil
	}

	if err := a.applyRequestTransformations(metaInfo, request); err != nil {
		return nil, errors.Wrap(err, "apply request transformations")
	}

	if (relayMode == relaymode.ChatCompletions || relayMode == relaymode.ClaudeMessages) &&
		shouldForceResponseAPI(metaInfo) {
		if unsupportedFieldCount := countResponseAPIUnsupportedContentFields(request.Messages); unsupportedFieldCount > 0 && lg != nil {
			modelName := ""
			channelID := 0
			if metaInfo != nil {
				modelName = metaInfo.ActualModelName
				channelID = metaInfo.ChannelId
			}
			lg.Debug("dropping unsupported content fields before response api conversion",
				zap.Int("unsupported_field_count", unsupportedFieldCount),
				zap.String("model", modelName),
				zap.Int("channel_id", channelID),
			)
		}
		responseAPIRequest := ConvertChatCompletionToResponseAPI(request)
		c.Set(ctxkey.ConvertedRequest, responseAPIRequest)
		metaInfo.RequestURLPath = "/v1/responses"
		meta.Set2Context(c, metaInfo)
		logConvertedRequest(c, metaInfo, relayMode, responseAPIRequest)
		return responseAPIRequest, nil
	}

	logConvertedRequest(c, metaInfo, relayMode, request)
	return request, nil
}

// applyRequestTransformations applies the existing request transformations.
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
		if request.StreamOptions == nil {
			request.StreamOptions = &model.StreamOptions{}
		}
		request.StreamOptions.IncludeUsage = true
	}

	if request.MaxTokens != 0 {
		tmpMaxTokens := request.MaxTokens
		request.MaxCompletionTokens = &tmpMaxTokens
		request.MaxTokens = 0
	}

	if request.MaxCompletionTokens == nil || *request.MaxCompletionTokens <= 0 {
		defaultMaxCompletionTokens := config.DefaultMaxToken
		request.MaxCompletionTokens = &defaultMaxCompletionTokens
	}

	actualModel := meta.ActualModelName
	if strings.TrimSpace(actualModel) == "" {
		actualModel = request.Model
	}
	channelType := 0
	if meta != nil {
		channelType = meta.ChannelType
	}
	supportsReasoning := isModelSupportedReasoning(actualModel)

	if supportsReasoning {
		targetsResponseAPI := meta.Mode == relaymode.ResponseAPI ||
			(meta.ChannelType == channeltype.OpenAI && !IsModelsOnlySupportedByChatCompletionAPI(actualModel))

		if targetsResponseAPI {
			request.Temperature = nil
		} else {
			temperature := float64(1)
			request.Temperature = &temperature
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
	} else {
		request.ReasoningEffort = nil
	}

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

	if request.ToolChoice != nil {
		normalized, _ := NormalizeToolChoice(request.ToolChoice)
		if meta != nil && (meta.ChannelType == channeltype.OpenAI || meta.ChannelType == channeltype.Azure) {
			flattened, _ := NormalizeToolChoiceForResponse(normalized)
			normalized = flattened
		}
		request.ToolChoice = normalized
	}

	if request.ResponseFormat != nil && request.ResponseFormat.JsonSchema != nil && request.ResponseFormat.JsonSchema.Schema != nil {
		if normalized, changed := NormalizeStructuredJSONSchema(request.ResponseFormat.JsonSchema.Schema, channelType); changed {
			request.ResponseFormat.JsonSchema.Schema = normalized
		}
	}

	if len(request.Functions) > 0 {
		for idx := range request.Functions {
			if params, ok := request.Functions[idx].Parameters.(map[string]any); ok && params != nil {
				if normalized, changed := NormalizeStructuredJSONSchema(params, channelType); changed {
					request.Functions[idx].Parameters = normalized
				}
			}
		}
	}

	if len(request.Tools) > 0 {
		for idx := range request.Tools {
			if fn := request.Tools[idx].Function; fn != nil {
				if paramMap, ok := fn.Parameters.(map[string]any); ok && paramMap != nil {
					if normalized, changed := NormalizeStructuredJSONSchema(paramMap, channelType); changed {
						fn.Parameters = normalized
					}
				}
			}
		}
	}

	if request.Stream && !config.EnforceIncludeUsage && strings.HasSuffix(request.Model, "-audio") {
		return errors.New("set ENFORCE_INCLUDE_USAGE=true to enable stream mode for gpt-4o-audio")
	}

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
				if parts[pi].ImageURL.Url != "" && strings.HasPrefix(parts[pi].ImageURL.Url, "data:image/") {
					if err := image.ValidateDataURLImage(parts[pi].ImageURL.Url); err != nil {
						return errors.Wrap(err, "validate inline image data")
					}
				}
			}
		}
		if changed {
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

	converted, err := openai_compatible.ConvertClaudeRequest(c, request)
	if err != nil {
		return nil, err
	}

	openaiRequest, ok := converted.(*model.GeneralOpenAIRequest)
	if !ok {
		return converted, nil
	}

	openaiRequest.Thinking = request.Thinking

	metaInfo := meta.GetByContext(c)
	if shouldNormalizeToolMessageContentForDeepSeek(metaInfo, openaiRequest) {
		normalizeClaudeThinkingForDeepSeek(gmw.GetLogger(c), openaiRequest)
	}
	if shouldNormalizeToolMessageContentForDeepSeek(metaInfo, openaiRequest) {
		normalizeDeepSeekToolMessageContent(gmw.GetLogger(c), openaiRequest)
	}
	if shouldForceResponseAPI(metaInfo) {
		if err := a.applyRequestTransformations(metaInfo, openaiRequest); err != nil {
			return nil, errors.Wrap(err, "apply request transformations for Claude conversion")
		}

		responseAPIRequest := ConvertChatCompletionToResponseAPI(openaiRequest)
		c.Set(ctxkey.ConvertedRequest, responseAPIRequest)
		return responseAPIRequest, nil
	}

	return openaiRequest, nil
}
