package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// large4ModelConfig returns an independently owned Large 4 chat configuration.
// It takes no parameters. Sources verified on 2026-10-12:
// https://docs.mistral.ai/models/mistral-large-4-0
// https://docs.mistral.ai/inference/pricing
// https://docs.mistral.ai/resources/changelogs
// https://docs.mistral.ai/studio/conversations/reasoning
//
// The October 6 announcement grants a two-week launch discount but publishes
// no exact cutoff time or timezone. The bounded overlay uses the repository's
// explicit UTC calendar-date convention, not a claimed provider cutoff instant.
// Standard list prices are the fallback, so the discount cannot become permanent.
func large4ModelConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                     1.36 * ratio.MilliTokensUsd,
		CachedInputRatio:           0.14 * ratio.MilliTokensUsd,
		CompletionRatio:           4.18 / 1.36,
		ContextLength:             1_000_000,
		InputModalities:           []string{"text", "image"},
		OutputModalities:          []string{"text"},
		SupportedFeatures:         []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedReasoningEfforts: []string{"none", "high"},
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "stop", "seed", "frequency_penalty", "presence_penalty", "reasoning_effort",
		},
		TimeWindows: []adaptor.TimeWindow{{
			Name: "large4-launch-utc-convention", TimeZone: "UTC",
			DateFrom: "2026-10-06", DateTo: "2026-10-20",
			Ranges: []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
			Overlay: adaptor.ModelConfig{
				Ratio:            0.68 * ratio.MilliTokensUsd,
				CachedInputRatio: 0.07 * ratio.MilliTokensUsd,
				CompletionRatio:  2.09 / 0.68,
			},
		}},
		Description: "Mistral Large 4 public-preview multimodal chat model with a published 1M context and none/high reasoning. The output ceiling and hosted quantization are unspecified. Standard USD/M input/cache/output: 1.36/0.14/4.18. Two-week launch prices: 0.68/0.07/2.09; the gateway uses October 6 through October 19 UTC because the provider does not publish an exact cutoff timezone. Operators must reconcile this date-boundary convention with their upstream invoice. Open weights are announced but no released weight ID is assumed.",
	}
}
