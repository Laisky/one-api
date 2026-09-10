package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Prices are USD per million tokens. The base configuration uses the current
// off-peak rate; time windows override all three prices together.
const (
	deepseekFlashInputPrice       = 0.15
	deepseekFlashCachedInputPrice = 0.003
	deepseekFlashOutputPrice      = 0.60
	deepseekProInputPrice         = 0.66
	deepseekProCachedInputPrice   = 0.022
	deepseekProOutputPrice        = 1.98
)

// deepseekModelConfig constructs independent capability slices for an API name.
// Parameters: description identifies the current version; vision enables image
// inputs, including uploaded image files (not arbitrary document ingestion).
// Returns: metadata shared by the current Flash and Pro API models.
func deepseekModelConfig(description string, vision bool) adaptor.ModelConfig {
	inputs := []string{"text"}
	if vision {
		inputs = append(inputs, "image", "file")
	}
	return adaptor.ModelConfig{
		ContextLength:     1048576,
		MaxOutputTokens:   393216,
		InputModalities:   inputs,
		OutputModalities:  []string{"text"},
		SupportedFeatures: []string{"tools", "json_mode", "logprobs", "reasoning"},
		// Temperature has no effect in thinking mode. top_p is effective there
		// with a 0.95 floor; in non-thinking mode top_p is fixed at 1.0.
		SupportedSamplingParameters: []string{"temperature", "top_p", "stop", "max_tokens"},
		SupportedReasoningEfforts:   []string{"low", "high", "max"},
		DefaultReasoningEffort:      "high",
		Description:                description,
	}
}

// deepseekFlashModelConfig returns current V4.1 Flash metadata and pricing.
// Parameters: description distinguishes the recommended name from aliases.
// Returns: a fresh configuration so aliases cannot share mutable pricing slices.
func deepseekFlashModelConfig(description string) adaptor.ModelConfig {
	cfg := deepseekModelConfig(description, true)
	cfg.Ratio = deepseekFlashInputPrice * ratio.MilliTokensUsd
	cfg.CachedInputRatio = deepseekFlashCachedInputPrice * ratio.MilliTokensUsd
	cfg.CompletionRatio = deepseekFlashOutputPrice / deepseekFlashInputPrice
	cfg.TimeWindows = deepseekPricingWindows("", "", deepseekFlashInputPrice, deepseekFlashCachedInputPrice, deepseekFlashOutputPrice)
	return cfg
}

// deepseekProModelConfig returns the current text-only Pro metadata, preserving
// Pro prices until its announced upstream redirection on 2026-09-14 04:00 UTC.
// Parameters: none. Returns: current metadata with the scheduled pricing switch.
func deepseekProModelConfig() adaptor.ModelConfig {
	cfg := deepseekModelConfig("DeepSeek-V4-Pro-0813 with thinking and non-thinking modes, 1M context, and native Responses and Anthropic API support. From 2026-09-14 04:00 UTC, this API name is served by V4.1 Flash at Flash prices until V4.1 Pro is released.", false)
	cfg.Ratio = deepseekProInputPrice * ratio.MilliTokensUsd
	cfg.CachedInputRatio = deepseekProCachedInputPrice * ratio.MilliTokensUsd
	cfg.CompletionRatio = deepseekProOutputPrice / deepseekProInputPrice
	cfg.TimeWindows = deepseekProPricingWindows()
	return cfg
}

// ModelRatios contains the current DeepSeek API names, including documented
// compatibility aliases. Verified on 2026-09-10 against:
//   - https://api-docs.deepseek.com/quick_start/pricing/
//   - https://api-docs.deepseek.com/guides/thinking_mode/
//   - https://api-docs.deepseek.com/guides/vision/
//   - https://api-docs.deepseek.com/guides/responses_api/
//
// These are current defaults, not a historical price archive. Image inputs use
// upstream prompt-token usage, never generated-image pricing. Retired V3 names
// deepseek-chat and deepseek-reasoner remain excluded. Old V4 weight/quantization
// metadata is not attached to aliases now served by the newer V4.1 model.
var ModelRatios = map[string]adaptor.ModelConfig{
	"deepseek-flash":               deepseekFlashModelConfig("DeepSeek-V4.1-Flash, the recommended multimodal Flash API name, with thinking and non-thinking modes, 1M context, and native Responses and Anthropic API support."),
	"deepseek-v4-flash":            deepseekFlashModelConfig("Compatibility alias for DeepSeek-V4.1-Flash; the original V4 Flash model is retired. Supports text and image input and uses current Flash prices."),
	"deepseek-v4-flash-vision-exp": deepseekFlashModelConfig("Compatibility alias for DeepSeek-V4.1-Flash; the experimental V4 vision model is retired. Supports text and image input and uses current Flash prices."),
	"deepseek-v4-pro":              deepseekProModelConfig(),
}

// DeepseekToolingDefaults does not advertise a separate built-in tool policy.
// The current Responses API ignores web_search and other built-in tool types;
// function tools remain supported and their token usage is billed normally.
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{
	Pricing: map[string]adaptor.ToolPricingConfig{},
}
