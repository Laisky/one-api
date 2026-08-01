package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	// deepseekTextInputs lists the input modalities supported by DeepSeek V4 models.
	deepseekTextInputs = []string{"text"}
	// deepseekTextOutputs lists the output modalities supported by DeepSeek V4 models.
	deepseekTextOutputs = []string{"text"}

	// deepseekFlashFeatures advertises DeepSeek V4 Flash capabilities across its
	// Chat Completions and native Responses API endpoints.
	deepseekFlashFeatures = []string{"tools", "json_mode", "logprobs", "reasoning", "web_search"}
	// deepseekProFeatures advertises DeepSeek V4 Pro Chat Completions capabilities.
	// Native Responses API support has not yet been enabled for this model.
	deepseekProFeatures = []string{"tools", "json_mode", "logprobs", "reasoning"}

	// deepseekSamplingParams lists the OpenAI-compatible sampling parameters
	// accepted by DeepSeek Chat Completions. Temperature and top_p have no effect
	// while thinking is enabled.
	deepseekSamplingParams = []string{"temperature", "top_p", "stop", "max_tokens"}

	// deepseekFlashReasoningEfforts lists the distinct reasoning levels supported
	// by DeepSeek V4 Flash. The API defaults to high.
	deepseekFlashReasoningEfforts = []string{"low", "high", "max"}
	// deepseekProReasoningEfforts lists the effective reasoning levels supported
	// by DeepSeek V4 Pro. The API accepts low but currently treats it as high.
	deepseekProReasoningEfforts = []string{"high", "max"}
)

// DeepSeek V4 regular per-token ratios. DeepSeek has announced that a future
// peak-hours policy will double all billing items between 09:00-12:00 and
// 14:00-18:00 Beijing time, but it has not published an effective date. Keep
// these flat prices until the official activation announcement.
const (
	// deepseekV4FlashInputRatio is the V4 Flash cache-miss input ratio.
	deepseekV4FlashInputRatio = 0.14 * ratio.MilliTokensUsd
	// deepseekV4FlashCachedInputRatio is the V4 Flash cache-hit input ratio.
	deepseekV4FlashCachedInputRatio = 0.0028 * ratio.MilliTokensUsd
	// deepseekV4ProInputRatio is the V4 Pro cache-miss input ratio.
	deepseekV4ProInputRatio = 0.435 * ratio.MilliTokensUsd
	// deepseekV4ProCachedInputRatio is the V4 Pro cache-hit input ratio.
	deepseekV4ProCachedInputRatio = 0.003625 * ratio.MilliTokensUsd
)

// ModelRatios contains the currently available DeepSeek API models and their
// pricing and capability metadata. Model IDs and prices were verified on
// 2026-08-01 against the official DeepSeek documentation:
//   - https://api-docs.deepseek.com/quick_start/pricing/
//   - https://api-docs.deepseek.com/api/list-models/
//   - https://api-docs.deepseek.com/api/create-chat-completion/
//   - https://api-docs.deepseek.com/guides/responses_api/
//   - https://api-docs.deepseek.com/updates/
//
// The retired deepseek-chat and deepseek-reasoner aliases are intentionally
// omitted; DeepSeek made them inaccessible after 2026-07-24 15:59 UTC.
var ModelRatios = map[string]adaptor.ModelConfig{
	// deepseek-v4-flash uses the DeepSeek-V4-Flash-0731 API version. Its regular
	// price is $0.14/1M cache-miss input, $0.0028/1M cache-hit input, and
	// $0.28/1M output.
	"deepseek-v4-flash": {
		Ratio:                       deepseekV4FlashInputRatio,
		CachedInputRatio:            deepseekV4FlashCachedInputRatio,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1048576,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekFlashFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		SupportedReasoningEfforts:   deepseekFlashReasoningEfforts,
		DefaultReasoningEffort:      "high",
		// Official instruct weights use FP4 MoE experts and FP8 for most other parameters.
		Quantization:  "fp4",
		HuggingFaceID: "deepseek-ai/DeepSeek-V4-Flash",
		Description:   "DeepSeek-V4-Flash-0731, a 284B/13B-active MoE model with thinking and non-thinking modes, 1M context, and native Responses API support.",
	},
	// deepseek-v4-pro costs $0.435/1M cache-miss input, $0.003625/1M cache-hit
	// input, and $0.87/1M output. Native Responses API support remains pending.
	"deepseek-v4-pro": {
		Ratio:                       deepseekV4ProInputRatio,
		CachedInputRatio:            deepseekV4ProCachedInputRatio,
		CompletionRatio:             0.87 / 0.435,
		ContextLength:               1048576,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekTextInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekProFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		SupportedReasoningEfforts:   deepseekProReasoningEfforts,
		DefaultReasoningEffort:      "high",
		// Official instruct weights use FP4 MoE experts and FP8 for most other parameters.
		Quantization:  "fp4",
		HuggingFaceID: "deepseek-ai/DeepSeek-V4-Pro",
		Description:   "DeepSeek-V4-Pro, a 1.6T/49B-active MoE model with thinking and non-thinking modes and a 1M context window; native Responses API support is pending.",
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish separate
// built-in tool prices; server-side web search incurs normal model token usage.
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
