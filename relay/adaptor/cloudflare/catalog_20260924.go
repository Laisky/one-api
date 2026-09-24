package cloudflare

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init adds source-verified September releases without rewriting legacy IDs. It
// takes no arguments and returns nothing. No unpublished output limit, effort
// default, or precision is inferred. Sources checked on 2026-09-24:
//   - https://developers.cloudflare.com/workers-ai/models/glm-5.3/
//   - https://developers.cloudflare.com/workers-ai/models/glm-5.3-flash/
func init() {
	for id, cfg := range map[string]adaptor.ModelConfig{
		"@cf/zai-org/glm-5.3": {
			Ratio: 1.4 * ratio.MilliTokensUsd, CachedInputRatio: 0.26 * ratio.MilliTokensUsd, CompletionRatio: 4.4 / 1.4,
			ContextLength:   1310720,
			InputModalities: []string{"text"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
			Description:                 "GLM 5.3 on Cloudflare Workers AI. Context follows the model card's exact 1,310,720-token field rather than its rounded 1M prose. Paid-plan eligibility is configured upstream.",
			HuggingFaceID:               "",
		},
		"@cf/zai-org/glm-5.3-flash": {
			Ratio: 0.15 * ratio.MilliTokensUsd, CachedInputRatio: 0.03 * ratio.MilliTokensUsd, CompletionRatio: 0.5 / 0.15,
			ContextLength:   1310720,
			InputModalities: []string{"text", "image"}, OutputModalities: []string{"text"},
			SupportedFeatures:           []string{"tools", "reasoning"},
			SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice", "response_format"},
			Description:                 "GLM 5.3 Flash on Cloudflare Workers AI with native vision. Context follows the published 1,310,720-token model-info field. No upstream account eligibility check is added.",
			HuggingFaceID:               "",
		},
	} {
		if _, exists := ModelRatios[id]; exists {
			panic("cloudflare: duplicate September model " + id)
		}
		ModelRatios[id] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
