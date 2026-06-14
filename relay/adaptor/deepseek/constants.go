package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for DeepSeek chat models. Reused across ModelRatios
// entries to keep the table compact and consistent.
var (
	// deepseekTextInputs lists the input modalities supported by all DeepSeek chat models.
	deepseekTextInputs = []string{"text"}
	// deepseekTextOutputs lists the output modalities for DeepSeek chat completion.
	deepseekTextOutputs = []string{"text"}

	// deepseekChatFeatures advertises the capability set for non-thinking DeepSeek chat models.
	// DeepSeek chat completions support tools, JSON mode and structured outputs per the official docs.
	deepseekChatFeatures = []string{"tools", "json_mode", "structured_outputs", "logprobs"}
	// deepseekReasoningFeatures advertises the capability set for thinking-mode DeepSeek models.
	deepseekReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "logprobs", "reasoning"}

	// deepseekSamplingParams lists the OpenAI-compatible sampling parameters DeepSeek chat accepts.
	deepseekSamplingParams = []string{"temperature", "top_p", "frequency_penalty", "presence_penalty", "stop", "max_tokens"}
	// deepseekReasonerSamplingParams lists the restricted sampling set for the legacy reasoner model.
	// DeepSeek's reasoner endpoint historically ignored temperature/top_p; only the listed knobs apply.
	deepseekReasonerSamplingParams = []string{"max_tokens", "stop"}

	// deepseekReasoningEfforts lists the reasoning_effort levels accepted by the
	// V4 chat models when thinking is enabled. DeepSeek currently publishes
	// "high" and "max" only; "max" is auto-selected for agentic Claude Code /
	// OpenCode style flows.
	// Source: https://api-docs.deepseek.com/api/create-chat-completion
	deepseekReasoningEfforts = []string{"high", "max"}
)

// ModelRatios contains all supported models and their pricing ratios
// Model list is derived from the keys of this map, eliminating redundancy
// Based on official DeepSeek pricing: https://api-docs.deepseek.com/quick_start/pricing
// Capability metadata sources (retrieved 2026-05-18):
//   - https://api-docs.deepseek.com/quick_start/pricing
//   - https://api-docs.deepseek.com/api/create-chat-completion
//   - https://huggingface.co/deepseek-ai/DeepSeek-V4-Flash
//   - https://huggingface.co/deepseek-ai/DeepSeek-V4-Pro
//
// Per the official docs, the public chat-completions API currently exposes
// only deepseek-v4-flash and deepseek-v4-pro. The legacy aliases
// deepseek-chat / deepseek-reasoner remain available until 2026-07-24 and
// route to deepseek-v4-flash non-thinking / thinking mode respectively.
var ModelRatios = map[string]adaptor.ModelConfig{
	// Legacy aliases (deprecation date 2026-07-24) — both pin to DeepSeek V4-Flash.
	// deepseek-chat = V4-Flash non-thinking mode.
	"deepseek-chat": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekChatFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Flash",
		Description:                 "Legacy alias of DeepSeek V4-Flash non-thinking mode; scheduled for deprecation 2026-07-24.",
	},
	// deepseek-reasoner = V4-Flash thinking mode (always-on thinking).
	"deepseek-reasoner": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekReasonerSamplingParams,
		// Legacy reasoner endpoint forces thinking on and does not accept reasoning_effort.
		Quantization:  "fp8",
		HuggingFaceID: "deepseek-ai/DeepSeek-V4-Flash",
		Description:   "Legacy alias of DeepSeek V4-Flash thinking mode; scheduled for deprecation 2026-07-24.",
	},
	// deepseek-v4-flash list price: $0.14/1M cache-miss input, $0.0028/1M cache-hit input,
	// $0.28/1M output, 1M context, 384K max output (= 384*1024 = 393216 tokens).
	"deepseek-v4-flash": {
		Ratio:                       0.14 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.0028 * ratio.MilliTokensUsd,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		// thinking.reasoning_effort: "high" (default) | "max" — applies only when thinking.type=enabled.
		// "low"/"medium" are silently mapped to "high"; "xhigh" maps to "max".
		SupportedReasoningEfforts: deepseekReasoningEfforts,
		DefaultReasoningEffort:    "high",
		Quantization:              "fp8",
		HuggingFaceID:             "deepseek-ai/DeepSeek-V4-Flash",
		Description:               "DeepSeek V4 Flash MoE chat model with thinking and non-thinking modes; 1M context.",
	},
	// deepseek-v4-pro list price: $0.435/1M cache-miss input, $0.003625/1M cache-hit input,
	// $0.87/1M output, 1M context, 384K max output. DeepSeek announced that after the
	// 75% promotional discount ends on 2026-05-31 15:59 UTC the API price officially
	// adjusts to 1/4 of the previous list ($1.74 / $0.0145 / $3.48 → $0.435 / $0.003625 / $0.87),
	// so the promo rate becomes the permanent list rate.
	// Source: https://api-docs.deepseek.com/quick_start/pricing (footnote 3).
	"deepseek-v4-pro": {
		Ratio:                       0.435 * ratio.MilliTokensUsd,
		CachedInputRatio:            0.003625 * ratio.MilliTokensUsd,
		CompletionRatio:             0.87 / 0.435,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekReasoningFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		// thinking.reasoning_effort: "high" (default) | "max" — applies only when thinking.type=enabled.
		SupportedReasoningEfforts: deepseekReasoningEfforts,
		DefaultReasoningEffort:    "high",
		Quantization:              "fp8",
		HuggingFaceID:             "deepseek-ai/DeepSeek-V4-Pro",
		Description:               "DeepSeek V4 Pro MoE chat model with thinking and non-thinking modes; 1M context.",
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish built-in tool pricing (retrieved 2026-05-18).
// Source: https://api-docs.deepseek.com/quick_start/pricing
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
