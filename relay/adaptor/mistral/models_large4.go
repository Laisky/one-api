package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// large4ModelConfig returns an independently owned Large 4 chat configuration.
// It takes no parameters. Sources verified on 2026-11-16:
// https://docs.mistral.ai/models/mistral-large-4-0
// https://docs.mistral.ai/inference/pricing
// https://docs.mistral.ai/resources/changelogs
// https://docs.mistral.ai/inference/conversations/reasoning
//
// The October 6 announcement grants a two-week launch discount but publishes
// no exact cutoff time or timezone. The bounded overlay uses the repository's
// explicit UTC calendar-date convention, not a claimed provider cutoff instant.
// Standard list prices are the fallback, so the discount cannot become permanent.
func large4ModelConfig() adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:                     0.70 * ratio.MilliTokensUsd,
		CachedInputRatio:          0.07 * ratio.MilliTokensUsd,
		CompletionRatio:           2.10 / 0.70,
		ContextLength:             1_000_000,
		InputModalities:           []string{"text", "image"},
		OutputModalities:          []string{"text"},
		SupportedFeatures:         []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "high",
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
		Description: "Mistral Large 4, generally available since 2026-11-05, with a published 1M context and low/medium/high reasoning (default high). Since October 29, none is no longer supported. The output ceiling and hosted quantization are unspecified. Current USD/M input/cache/output: 0.70/0.07/2.10. Historical two-week launch prices remain 0.68/0.07/2.09; the gateway uses October 6 through October 19 UTC because the provider does not publish an exact cutoff timezone. Current defaults do not reconstruct historical standard-price changes. Operators must reconcile date-boundary conventions with their upstream invoice.",
	}
}

// large4LatestModelConfig returns the documented mutable Large 4 alias without
// retroactively attaching the October launch discount to the former Large 3 alias.
// It takes no parameters and returns an independently owned configuration.
func large4LatestModelConfig() adaptor.ModelConfig {
	cfg := large4ModelConfig()
	cfg.TimeWindows = nil
	cfg.Description = "Mutable mistral-large-latest alias, documented as Mistral Large 4 on 2026-11-16. Current USD/M input/cache/output: 0.70/0.07/2.10; 1M context and low/medium/high reasoning, default high. The former Large 3 alias does not inherit Large 4's expired launch promotion. Pinned mistral-large-2512 remains independently configured."
	return cfg
}
