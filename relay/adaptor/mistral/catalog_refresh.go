package mistral

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// init installs verified public chat models and cached-input rates. It takes no
// parameters and returns no values. Legacy IDs and ordinary token prices remain
// unchanged. Sources audited 2026-09-24:
//   - https://docs.mistral.ai/inference/pricing
//   - https://docs.mistral.ai/models/zai-glm-5-2
//   - https://docs.mistral.ai/models/zai-glm-5-3
//   - https://docs.mistral.ai/models/leanstral-1-5
func init() {
	for name, cachedUSD := range map[string]float64{
		"mistral-medium-latest": 0.15,
		"mistral-medium-2604":   0.15,
		"mistral-medium-3-5":    0.15,
		"mistral-large-latest":  0.05,
		"mistral-large-2512":    0.05,
		"mistral-small-latest":  0.015,
		"mistral-small-2603":    0.015,
		"ministral-14b-2512":    0.02,
		"ministral-8b-latest":   0.015,
		"ministral-8b-2512":     0.015,
		"ministral-3b-latest":   0.01,
		"ministral-3b-2512":     0.01,
		"codestral-latest":      0.03,
		"codestral-2508":        0.03,
		"codestral-embed-2505":  0.015,
	} {
		cfg, ok := ModelRatios[name]
		if !ok {
			panic("mistral: missing cached-price base " + name)
		}
		cfg = cfg.Clone()
		cfg.CachedInputRatio = cachedUSD * ratio.MilliTokensUsd
		ModelRatios[name] = cfg
	}

	for _, name := range []string{"zai-glm-5-2", "zai-glm-5-3"} {
		if _, exists := ModelRatios[name]; exists {
			panic("mistral: duplicate hosted model " + name)
		}
		ModelRatios[name] = adaptor.ModelConfig{
			Ratio:             1.4 * ratio.MilliTokensUsd,
			CachedInputRatio:  0.14 * ratio.MilliTokensUsd,
			CompletionRatio:   4.4 / 1.4,
			ContextLength:     1_000_000,
			MaxOutputTokens:   128_000,
			InputModalities:   []string{"text"},
			OutputModalities:  []string{"text"},
			SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
			SupportedSamplingParameters: []string{
				"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice",
			},
			Description: "Public-preview GLM model hosted by Mistral with 1M context and 128K maximum output. Uses Mistral's model ID and USD tariff: $1.40 input, $0.14 cached input, and $4.40 output per million tokens; not Z.AI's cached-input price. No provider-specific reasoning-effort vocabulary is inferred.",
		}
	}
	ModelRatios["labs-leanstral-1-5"] = adaptor.ModelConfig{
		Ratio:             0,
		CompletionRatio:   1,
		ContextLength:     262_144,
		MaxOutputTokens:   128_000,
		InputModalities:   []string{"text"},
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: []string{
			"temperature", "top_p", "max_tokens", "stop", "response_format", "tools", "tool_choice",
		},
		Description: "Leanstral 1.5 public-preview code agent for Lean 4 formal proof engineering. Mistral lists both input and output as free; 256K context and 128K maximum output. Preview availability remains controlled by the upstream account.",
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
