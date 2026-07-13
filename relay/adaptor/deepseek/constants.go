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
	// frequency_penalty and presence_penalty were deprecated in the DeepSeek V4
	// chat-completions API (no-ops that never take effect), so they are no longer
	// advertised. Source: https://api-docs.deepseek.com/api/create-chat-completion
	deepseekSamplingParams = []string{"temperature", "top_p", "stop", "max_tokens"}
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

// DeepSeek V4 base (list = off-peak / 平时·低谷) per-token ratios. The published
// list price on api-docs.deepseek.com/quick_start/pricing IS the off-peak price:
// a request bills at list unless it falls inside the peak window below, which
// doubles every line item. V4-Flash and V4-Pro share their cache-miss input and
// cache-hit input ratios with the legacy deepseek-chat / deepseek-reasoner
// aliases, so they are factored out here to keep the base and the peak overlay
// in lock-step (a single source of truth for each price).
const (
	// deepseekV4FlashInputRatio is the V4-Flash cache-miss input ratio at list (off-peak).
	deepseekV4FlashInputRatio = 0.14 * ratio.MilliTokensUsd
	// deepseekV4FlashCachedInputRatio is the V4-Flash cache-hit input ratio at list (off-peak).
	deepseekV4FlashCachedInputRatio = 0.0028 * ratio.MilliTokensUsd
	// deepseekV4ProInputRatio is the V4-Pro cache-miss input ratio at list (off-peak).
	deepseekV4ProInputRatio = 0.435 * ratio.MilliTokensUsd
	// deepseekV4ProCachedInputRatio is the V4-Pro cache-hit input ratio at list (off-peak).
	deepseekV4ProCachedInputRatio = 0.003625 * ratio.MilliTokensUsd

	// deepseekPeakSurcharge is the multiple of the list price charged during the
	// peak (高峰) window. DeepSeek's 2026-06-29 peak-valley (峰谷) notice doubles
	// every line item (cache-hit input, cache-miss input, output) during peak
	// hours and keeps the published list price for all other hours, so a single
	// uniform multiplier reproduces the schedule for both models.
	deepseekPeakSurcharge = 2.0

	// deepseekPeakEffectiveFrom gates the peak surcharge to the V4 official
	// release ("mid-July 2026" per the 2026-06-29 notice; DeepSeek emails users
	// 24h before the switch). Before this local date the API bills flat list at
	// all hours. Adjust once DeepSeek announces the exact activation date.
	deepseekPeakEffectiveFrom = "2026-07-15"
)

// deepseekPeakWindow builds the peak (高峰) pricing overlay for a DeepSeek V4
// model whose ModelConfig carries the published list (= off-peak) price as its
// base ratios.
//
// DeepSeek's 2026-06-29 峰谷定价 notice defines the daily PEAK hours as
// 09:00–12:00 and 14:00–18:00 Beijing time (Asia/Shanghai) and bills them at 2x
// the list price; all other hours keep the list price unchanged (维持当前定价不变).
// There is never a discount below list.
//
// Only Ratio and CachedInputRatio are overlaid (each doubled). Output is
// surcharged automatically because output price = Ratio * CompletionRatio and
// the overlay inherits CompletionRatio, so doubling Ratio doubles output too.
// CompletionRatio is therefore intentionally omitted from the overlay (0 == inherit).
//
// Sources: DeepSeek 2026-06-29 user-notification email as reported by IT之家
// (ithome.com/0/970/123.htm) and Sina Finance; api-docs.deepseek.com pricing page
// (still flat list as of 2026-07-09, pending the mid-July V4 official release).
func deepseekPeakWindow(listInputRatio, listCachedInputRatio float64) []adaptor.TimeWindow {
	return []adaptor.TimeWindow{
		{
			Name:     "deepseek-peak",
			TimeZone: "Asia/Shanghai",
			DateFrom: deepseekPeakEffectiveFrom,
			Ranges: []adaptor.ClockRange{
				{Start: "09:00", End: "12:00"},
				{Start: "14:00", End: "18:00"},
			},
			Overlay: adaptor.ModelConfig{
				Ratio:            listInputRatio * deepseekPeakSurcharge,
				CachedInputRatio: listCachedInputRatio * deepseekPeakSurcharge,
			},
		},
	}
}

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
		Ratio:                       deepseekV4FlashInputRatio,
		CachedInputRatio:            deepseekV4FlashCachedInputRatio,
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
		// Base ratios are the published list (= off-peak) price; peak (高峰) hours bill at 2x — see deepseekPeakWindow.
		TimeWindows: deepseekPeakWindow(deepseekV4FlashInputRatio, deepseekV4FlashCachedInputRatio),
	},
	// deepseek-reasoner = V4-Flash thinking mode (always-on thinking).
	"deepseek-reasoner": {
		Ratio:                       deepseekV4FlashInputRatio,
		CachedInputRatio:            deepseekV4FlashCachedInputRatio,
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
		// Base ratios are the published list (= off-peak) price; peak (高峰) hours bill at 2x — see deepseekPeakWindow.
		TimeWindows: deepseekPeakWindow(deepseekV4FlashInputRatio, deepseekV4FlashCachedInputRatio),
	},
	// deepseek-v4-flash list price: $0.14/1M cache-miss input, $0.0028/1M cache-hit input,
	// $0.28/1M output, 1M context, 384K max output (= 384*1024 = 393216 tokens).
	"deepseek-v4-flash": {
		Ratio:                       deepseekV4FlashInputRatio,
		CachedInputRatio:            deepseekV4FlashCachedInputRatio,
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
		// Base ratios are the published list (= off-peak) price; peak (高峰) hours bill at 2x — see deepseekPeakWindow.
		TimeWindows: deepseekPeakWindow(deepseekV4FlashInputRatio, deepseekV4FlashCachedInputRatio),
	},
	// deepseek-v4-pro list price: $0.435/1M cache-miss input, $0.003625/1M cache-hit input,
	// $0.87/1M output, 1M context, 384K max output. DeepSeek announced that after the
	// 75% promotional discount ends on 2026-05-31 15:59 UTC the API price officially
	// adjusts to 1/4 of the previous list ($1.74 / $0.0145 / $3.48 → $0.435 / $0.003625 / $0.87),
	// so the promo rate becomes the permanent list rate.
	// Source: https://api-docs.deepseek.com/quick_start/pricing (footnote 3).
	"deepseek-v4-pro": {
		Ratio:                       deepseekV4ProInputRatio,
		CachedInputRatio:            deepseekV4ProCachedInputRatio,
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
		// Base ratios are the published list (= off-peak) price; peak (高峰) hours bill at 2x — see deepseekPeakWindow.
		TimeWindows: deepseekPeakWindow(deepseekV4ProInputRatio, deepseekV4ProCachedInputRatio),
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish built-in tool pricing (retrieved 2026-05-18).
// Source: https://api-docs.deepseek.com/quick_start/pricing
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
