package controller

import (
	"strings"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	openaipayload "github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/apitype"
	"github.com/Laisky/one-api/relay/channeltype"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

type thinkingQueryState int

const (
	thinkingQueryUnspecified thinkingQueryState = iota
	thinkingQueryDisabled
	thinkingQueryEnabled
)

// applyThinkingQueryToChatRequest inspects the thinking query parameter and applies
// the corresponding reasoning defaults to a chat completion request when the
// downstream provider supports extended reasoning.
func applyThinkingQueryToChatRequest(c *gin.Context, request *relaymodel.GeneralOpenAIRequest, meta *metalib.Meta) {
	state := parseThinkingQueryState(c)
	if request == nil || state == thinkingQueryUnspecified {
		return
	}

	modelName := resolveModelName(meta, request.Model)
	if state == thinkingQueryEnabled && supportsThinkingInjection(meta, modelName) {
		ensureReasoningEffort(c, request, modelName)
		ensureIncludeReasoning(meta, request)
	}

	ensureVLLMThinkingOverride(c, meta, request, modelName, state)
}

// applyThinkingQueryToResponseRequest applies reasoning defaults to Response API
// requests when thinking is enabled via query parameters.
func applyThinkingQueryToResponseRequest(c *gin.Context, request *openaipayload.ResponseAPIRequest, meta *metalib.Meta) {
	state := parseThinkingQueryState(c)
	if request == nil || state == thinkingQueryUnspecified {
		return
	}

	modelName := resolveModelName(meta, request.Model)
	if state == thinkingQueryEnabled && supportsThinkingInjection(meta, modelName) {
		ensureResponseReasoning(c, request, modelName)
	}

	ensureResponseVLLMThinkingOverride(c, meta, request, modelName, state)
}

// isThinkingQueryTruthy reports whether the thinking query parameter requests
// auto-enabling reasoning features for the current request context.
func isThinkingQueryTruthy(c *gin.Context) bool {
	return parseThinkingQueryState(c) == thinkingQueryEnabled
}

// parseThinkingQueryState parses the thinking query parameter as a tri-state toggle.
func parseThinkingQueryState(c *gin.Context) thinkingQueryState {
	if c == nil {
		return thinkingQueryUnspecified
	}

	value, ok := c.GetQuery("thinking")
	if !ok {
		return thinkingQueryUnspecified
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return thinkingQueryEnabled
	case "0", "false", "no", "off":
		return thinkingQueryDisabled
	default:
		return thinkingQueryUnspecified
	}
}

// isTruthy normalizes a string and returns true when it matches a known truthy token.
func isTruthy(val string) bool {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ensureVLLMThinkingOverride injects Qwen/vLLM thinking overrides through extra_body.
func ensureVLLMThinkingOverride(c *gin.Context, meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest, modelName string, state thinkingQueryState) {
	if request == nil || !shouldApplyVLLMThinkingOverride(c, meta, modelName, state) {
		return
	}

	if !ensureExtraBodyChatTemplateThinking(request.ExtraBody) {
		return
	}
	if request.ExtraBody == nil {
		request.ExtraBody = map[string]any{}
	}
	applyEnableThinking(request.ExtraBody, state == thinkingQueryEnabled)
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("applied vllm thinking override",
			zap.String("model", modelName),
			zap.Bool("enable_thinking", state == thinkingQueryEnabled),
		)
	}
}

// ensureResponseVLLMThinkingOverride injects Qwen/vLLM thinking overrides into Response API extra_body.
func ensureResponseVLLMThinkingOverride(c *gin.Context, meta *metalib.Meta, request *openaipayload.ResponseAPIRequest, modelName string, state thinkingQueryState) {
	if request == nil || !shouldApplyVLLMThinkingOverride(c, meta, modelName, state) {
		return
	}

	if !ensureExtraBodyChatTemplateThinking(request.ExtraBody) {
		return
	}
	if request.ExtraBody == nil {
		request.ExtraBody = map[string]any{}
	}
	applyEnableThinking(request.ExtraBody, state == thinkingQueryEnabled)
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("applied response vllm thinking override",
			zap.String("model", modelName),
			zap.Bool("enable_thinking", state == thinkingQueryEnabled),
		)
	}
}

// shouldApplyVLLMThinkingOverride reports whether the current request should receive
// the vLLM/Qwen chat_template_kwargs thinking toggle and emits debug diagnostics
// when a matching model is skipped because the provider does not look compatible.
func shouldApplyVLLMThinkingOverride(c *gin.Context, meta *metalib.Meta, modelName string, state thinkingQueryState) bool {
	if state == thinkingQueryUnspecified || !isVLLMThinkingModel(modelName) {
		return false
	}
	if supportsVLLMThinkingOverride(meta, modelName) {
		return true
	}
	if lg := gmw.GetLogger(c); lg != nil {
		channelType := 0
		if meta != nil {
			channelType = meta.ChannelType
		}
		lg.Debug("skipped vllm thinking override",
			zap.String("model", modelName),
			zap.Int("channel_type", channelType),
			zap.Bool("base_url_matches_vllm", isLikelyVLLMBaseURL(meta)),
		)
	}
	return false
}

// supportsVLLMThinkingOverride reports whether a model/provider pair follows the
// vLLM/Qwen chat_template_kwargs toggle.
func supportsVLLMThinkingOverride(meta *metalib.Meta, modelName string) bool {
	if !isVLLMThinkingModel(modelName) {
		return false
	}
	if meta == nil {
		return false
	}

	return meta.ChannelType == channeltype.OpenAICompatible || isLikelyVLLMBaseURL(meta)
}

// isVLLMThinkingModel reports whether a model name matches the known Qwen/vLLM
// thinking toggle convention.
func isVLLMThinkingModel(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}

	return strings.Contains(name, "qwen3")
}

// isLikelyVLLMBaseURL reports whether the configured upstream base URL clearly
// identifies a vLLM deployment.
func isLikelyVLLMBaseURL(meta *metalib.Meta) bool {
	if meta == nil {
		return false
	}
	baseURL := strings.ToLower(strings.TrimSpace(meta.BaseURL))
	if baseURL == "" {
		return false
	}

	return strings.Contains(baseURL, "vllm")
}

// ensureExtraBodyChatTemplateThinking reports whether extra_body may receive a default enable_thinking value.
func ensureExtraBodyChatTemplateThinking(extraBody map[string]any) bool {
	if extraBody == nil {
		return true
	}
	chatTemplateKwargs, ok := extraBody["chat_template_kwargs"].(map[string]any)
	if !ok || chatTemplateKwargs == nil {
		return true
	}
	_, exists := chatTemplateKwargs["enable_thinking"]
	return !exists
}

// applyEnableThinking writes enable_thinking into extra_body.chat_template_kwargs.
func applyEnableThinking(extraBody map[string]any, enabled bool) {
	chatTemplateKwargs, _ := extraBody["chat_template_kwargs"].(map[string]any)
	if chatTemplateKwargs == nil {
		chatTemplateKwargs = map[string]any{}
	}
	chatTemplateKwargs["enable_thinking"] = enabled
	extraBody["chat_template_kwargs"] = chatTemplateKwargs
}

// resolveModelName determines the effective model name, preferring the mapped
// model stored on meta over the local fallback when available.
func resolveModelName(meta *metalib.Meta, fallback string) string {
	if meta != nil && strings.TrimSpace(meta.ActualModelName) != "" {
		return meta.ActualModelName
	}
	return fallback
}

// supportsThinkingInjection returns true when the channel and model support
// automatic reasoning parameter injection.
func supportsThinkingInjection(meta *metalib.Meta, modelName string) bool {
	if strings.TrimSpace(modelName) == "" {
		return false
	}

	if meta != nil {
		switch meta.APIType {
		case apitype.Anthropic, apitype.AwsClaude:
			return false
		}
	}

	return isReasoningCapableModel(modelName)
}

// ensureReasoningEffort populates reasoning_effort on the chat request when it
// has not been provided by the caller.
func ensureReasoningEffort(c *gin.Context, request *relaymodel.GeneralOpenAIRequest, modelName string) {
	if request.ReasoningEffort != nil && strings.TrimSpace(*request.ReasoningEffort) != "" {
		return
	}

	requested := strings.TrimSpace(c.Query("reasoning_effort"))
	desired := normalizeReasoningEffort(modelName, requested)
	if desired == "" {
		desired = defaultReasoningEffort(modelName)
	}
	if desired == "" {
		return
	}

	request.ReasoningEffort = stringPtr(desired)
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("reasoning effort applied",
			zap.String("model", modelName),
			zap.String("reasoning_effort", desired),
			zap.Bool("from_query", requested != ""),
			zap.String("requested_effort", requested),
		)
	}
}

// ensureIncludeReasoning guarantees OpenRouter requests opt into reasoning payloads.
func ensureIncludeReasoning(meta *metalib.Meta, request *relaymodel.GeneralOpenAIRequest) {
	if meta == nil || meta.ChannelType != channeltype.OpenRouter {
		return
	}
	if request.IncludeReasoning != nil {
		return
	}
	include := true
	request.IncludeReasoning = &include
}

// ensureResponseReasoning ensures Response API requests include a reasoning effort configuration.
func ensureResponseReasoning(c *gin.Context, request *openaipayload.ResponseAPIRequest, modelName string) {
	var existing string
	if request.Reasoning != nil && request.Reasoning.Effort != nil {
		existing = strings.TrimSpace(*request.Reasoning.Effort)
	}
	if existing != "" {
		return
	}

	requested := strings.TrimSpace(c.Query("reasoning_effort"))
	desired := normalizeReasoningEffort(modelName, requested)
	if desired == "" {
		desired = defaultReasoningEffort(modelName)
	}
	if desired == "" {
		return
	}

	if request.Reasoning == nil {
		request.Reasoning = &relaymodel.OpenAIResponseReasoning{}
	}
	request.Reasoning.Effort = stringPtr(desired)
	if lg := gmw.GetLogger(c); lg != nil {
		lg.Debug("response reasoning effort applied",
			zap.String("model", modelName),
			zap.String("reasoning_effort", desired),
			zap.Bool("from_query", requested != ""),
			zap.String("requested_effort", requested),
		)
	}
}

func isMediumOnlyOpenAIReasoningModel(name string) bool {
	if name == "" {
		return false
	}

	if strings.Contains(name, "gpt-5.1-chat") {
		return true
	}

	if name[0] == 'o' {
		if len(name) == 1 {
			return true
		}
		switch name[1] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', '-':
			return true
		}
	}

	return false
}

// isReasoningCapableModel identifies models that accept reasoning configuration payloads.
func isReasoningCapableModel(modelName string) bool {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return false
	}

	if isMediumOnlyOpenAIReasoningModel(name) {
		return true
	}

	switch {
	case strings.HasPrefix(name, "gpt-5") && !strings.HasPrefix(name, "gpt-5-chat"):
		return true
	case strings.Contains(name, "deep-research"):
		return true
	case strings.HasPrefix(name, "grok"):
		return true
	case strings.Contains(name, "deepseek-r1"):
		return true
	case strings.Contains(name, "reasoner"):
		return true
	default:
		return false
	}
}

// defaultReasoningEffort returns the preferred reasoning effort for a model when none is specified.
func defaultReasoningEffort(modelName string) string {
	name := strings.ToLower(strings.TrimSpace(modelName))
	if name == "" {
		return ""
	}
	if strings.Contains(name, "deep-research") || isMediumOnlyOpenAIReasoningModel(name) {
		return "medium"
	}
	return "high"
}

// normalizeReasoningEffort sanitizes a requested reasoning effort value for a model.
func normalizeReasoningEffort(modelName, effort string) string {
	normalized := strings.ToLower(strings.TrimSpace(effort))
	if normalized == "" {
		return ""
	}
	if !isReasoningEffortAllowed(modelName, normalized) {
		return ""
	}
	return normalized
}

// isReasoningEffortAllowed reports whether the supplied effort is permitted for the model.
func isReasoningEffortAllowed(modelName, effort string) bool {
	if effort == "" {
		return false
	}
	switch effort {
	case "low", "medium", "high":
	default:
		return false
	}

	name := strings.ToLower(strings.TrimSpace(modelName))
	if strings.Contains(name, "deep-research") || isMediumOnlyOpenAIReasoningModel(name) {
		return effort == "medium"
	}
	return true
}

// stringPtr returns a pointer to a copy of the provided string value.
func stringPtr(v string) *string {
	value := v
	return &value
}
