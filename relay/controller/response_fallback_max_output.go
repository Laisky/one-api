package controller

import (
	"time"

	gmw "github.com/Laisky/gin-middlewares/v7"
	"github.com/Laisky/zap"
	"github.com/gin-gonic/gin"

	"github.com/Laisky/one-api/model"
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	metalib "github.com/Laisky/one-api/relay/meta"
	relaymodel "github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/pricing"
)

// applyResponseFallbackClaudeMaxOutput gives a Responses request that the
// fallback lowers to Claude Messages the model's documented maximum output
// when the client sent no max_output_tokens. The Responses API reads an
// omitted limit as "up to the model's maximum", while Claude requires
// max_tokens and the converter would otherwise apply DEFAULT_MAX_TOKEN, which
// truncates long answers and patches (Codex never sends a limit). It runs
// after quota reservation, so the hold is unchanged and only the upstream
// request differs. Parameters: c is the request context, meta the resolved
// upstream, request the Responses request, chatRequest the lowered request
// (mutated), configs the channel model configs, provider the pricing adaptor
// and promptTokens the estimated prompt size.
func applyResponseFallbackClaudeMaxOutput(c *gin.Context, meta *metalib.Meta, request *openai.ResponseAPIRequest, chatRequest *relaymodel.GeneralOpenAIRequest, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, promptTokens int) {
	limit := responseFallbackClaudeMaxOutput(meta, request, chatRequest, configs, provider, promptTokens)
	if limit <= 0 {
		return
	}
	chatRequest.MaxTokens = limit
	lg := gmw.GetLogger(c)
	lg.Debug("response fallback uses the model maximum output for Claude",
		zap.String("model", meta.ActualModelName),
		zap.Int("max_tokens", limit),
		zap.Int("prompt_tokens", promptTokens))
}

// responseFallbackClaudeMaxOutput returns the max_tokens a Claude-lowered
// fallback request should send when the client set no output limit, or 0 to
// keep the converter default. The limit comes from the channel's model config
// (max_output_tokens, then max_tokens), the provider's published config, and
// for Claude models the Anthropic catalog, in that order. It is capped at the
// context window left after the prompt, which pre-4.5 Claude models require.
// Parameters mirror applyResponseFallbackClaudeMaxOutput.
func responseFallbackClaudeMaxOutput(meta *metalib.Meta, request *openai.ResponseAPIRequest, chatRequest *relaymodel.GeneralOpenAIRequest, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, promptTokens int) int {
	if chatRequest == nil || request == nil || !responseFallbackLowersToClaude(meta) {
		return 0
	}
	if request.MaxOutputTokens != nil || chatRequest.MaxTokens > 0 ||
		(chatRequest.MaxCompletionTokens != nil && *chatRequest.MaxCompletionTokens > 0) {
		return 0
	}
	output, contextLength := claudeModelOutputLimits(meta.ActualModelName, configs, provider, meta.StartTime)
	if output <= 0 {
		return 0
	}
	if contextLength > 0 && promptTokens > 0 {
		room := contextLength - promptTokens
		if room <= 0 {
			return 0
		}
		output = min(output, room)
	}
	return output
}

// claudeModelOutputLimits resolves a model's maximum output tokens and context
// window from the channel config, the provider's published config, and the
// Anthropic catalog. Parameters: modelName is the upstream model, configs the
// channel model configs, provider the pricing adaptor and at the request time.
// Returns the limits, 0 when unknown.
func claudeModelOutputLimits(modelName string, configs map[string]model.ModelConfigLocal, provider adaptor.Adaptor, at time.Time) (output, contextLength int) {
	candidates := make([]adaptor.ModelConfig, 0, 3)
	if cfg, ok := pricing.ResolveModelConfig(modelName, configs, provider, at); ok {
		candidates = append(candidates, cfg)
	}
	if provider != nil {
		if cfg, ok := pricing.ResolveModelConfig(modelName, nil, provider, at); ok {
			candidates = append(candidates, cfg)
		}
	}
	if cfg, ok := anthropic.ModelRatios[modelName]; ok {
		candidates = append(candidates, cfg)
	}
	for _, cfg := range candidates {
		if output <= 0 {
			output = int(cfg.MaxOutputTokens)
			if output <= 0 {
				output = int(cfg.MaxTokens)
			}
		}
		if contextLength <= 0 {
			contextLength = int(cfg.ContextLength)
		}
	}
	return output, contextLength
}
