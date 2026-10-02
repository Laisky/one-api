package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init adds source-verified September releases without rewriting legacy IDs. It
// takes no arguments and returns nothing. No unpublished output limit, effort
// default, or precision is inferred. Sources checked on 2026-09-24:
//   - https://fireworks.ai/models?modelTypes=Serverless
//   - https://fireworks.ai/models/fireworks/ember-1
//   - https://fireworks.ai/models/deepseek-ai/deepseek-v4p1-flash
func init() {
	for id, cfg := range map[string]adaptor.ModelConfig{
		"accounts/fireworks/models/ember-1": {
			Ratio: 3 * ratio.MilliTokensUsd, CachedInputRatio: 0.3 * ratio.MilliTokensUsd, CompletionRatio: 15 / 3,
			ContextLength:   1048576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
			Description:                 "Fireworks Ember-1, a Kimi K3-based vision and reasoning model released 2026-09-22. Uses Fireworks serverless rates, not dedicated GPU pricing.",
			HuggingFaceID:               "",
		},
		"accounts/fireworks/models/deepseek-v4p1-flash": {
			Ratio: 0.22 * ratio.MilliTokensUsd, CachedInputRatio: 0.007 * ratio.MilliTokensUsd, CompletionRatio: 0.66 / 0.22,
			ContextLength:   1048576,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
			Description:                 "DeepSeek V4.1 Flash hosted by Fireworks, with native text and image input. Uses Fireworks rates, not the first-party daypart tariff.",
			HuggingFaceID:               "deepseek-ai/DeepSeek-V4.1-Flash",
		},
	} {
		if _, exists := ModelRatios[id]; exists {
			panic("fireworks: duplicate September model " + id)
		}
		ModelRatios[id] = cfg
	}
}
