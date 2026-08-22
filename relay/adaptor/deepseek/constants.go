package deepseek

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	// deepseekTextInputs lists the input modalities supported by text-only DeepSeek V4 models.
	deepseekTextInputs = []string{"text"}
	// deepseekVisionInputs lists the input modalities supported by DeepSeek V4 Vision models.
	deepseekVisionInputs = []string{"text", "image"}
	// deepseekTextOutputs lists the output modalities supported by DeepSeek V4 models.
	deepseekTextOutputs = []string{"text"}

	// deepseekFlashFeatures advertises DeepSeek V4 Flash capabilities across its
	// Chat Completions and native Responses API endpoints.
	deepseekFlashFeatures = []string{"tools", "json_mode", "logprobs", "reasoning", "web_search"}
	// deepseekProFeatures advertises DeepSeek V4 Pro capabilities across its
	// Chat Completions and native Responses API endpoints.
	deepseekProFeatures = []string{"tools", "json_mode", "logprobs", "reasoning", "web_search"}
	// deepseekVisionFeatures advertises the capabilities documented for the
	// experimental V4 Flash Vision model. It intentionally excludes web_search
	// until DeepSeek documents that combination for the vision model.
	deepseekVisionFeatures = []string{"tools", "json_mode", "logprobs", "reasoning"}

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

// DeepSeek V4 regular per-token ratios. The base values are the prices in effect
// before the scheduled peak/off-peak pricing change on 2026-08-16 16:00 UTC.
const (
	// deepseekV4FlashInputRatio is the V4 Flash cache-miss input ratio.
	deepseekV4FlashInputRatio = 0.14 * ratio.MilliTokensUsd
	// deepseekV4FlashCachedInputRatio is the V4 Flash cache-hit input ratio.
	deepseekV4FlashCachedInputRatio = 0.0028 * ratio.MilliTokensUsd
	// deepseekV4ProInputRatio is the V4 Pro cache-miss input ratio.
	deepseekV4ProInputRatio = 0.435 * ratio.MilliTokensUsd
	// deepseekV4ProCachedInputRatio is the V4 Pro cache-hit input ratio.
	deepseekV4ProCachedInputRatio = 0.003625 * ratio.MilliTokensUsd

	// deepseekV4PricingEffectiveDate is the first local date covered by the new
	// pricing schedule. The documented activation instant, 2026-08-16 16:00 UTC,
	// is 2026-08-17 00:00 in the schedule's Asia/Shanghai timezone.
	deepseekV4PricingEffectiveDate = "2026-08-17"
)

// deepseekV4PricingWindows returns the official post-activation off-peak and
// peak pricing windows for one model.
// Parameters: offPeakInput, offPeakCachedInput, and offPeakOutput are the
// off-peak prices per million tokens; peakInput, peakCachedInput, and
// peakOutput are the corresponding peak prices per million tokens.
// Returns: two Asia/Shanghai time windows beginning on the effective date.
func deepseekV4PricingWindows(
	offPeakInput float64,
	offPeakCachedInput float64,
	offPeakOutput float64,
	peakInput float64,
	peakCachedInput float64,
	peakOutput float64,
) []adaptor.TimeWindow {
	return []adaptor.TimeWindow{
		{
			Name:     "deepseek-offpeak",
			TimeZone: "Asia/Shanghai",
			DateFrom: deepseekV4PricingEffectiveDate,
			Ranges: []adaptor.ClockRange{
				{Start: "18:00", End: "09:00"},
				{Start: "12:00", End: "14:00"},
			},
			Overlay: adaptor.ModelConfig{
				Ratio:            offPeakInput * ratio.MilliTokensUsd,
				CachedInputRatio: offPeakCachedInput * ratio.MilliTokensUsd,
				CompletionRatio:  offPeakOutput / offPeakInput,
			},
		},
		{
			Name:     "deepseek-peak",
			TimeZone: "Asia/Shanghai",
			DateFrom: deepseekV4PricingEffectiveDate,
			Ranges: []adaptor.ClockRange{
				{Start: "09:00", End: "12:00"},
				{Start: "14:00", End: "18:00"},
			},
			Overlay: adaptor.ModelConfig{
				Ratio:            peakInput * ratio.MilliTokensUsd,
				CachedInputRatio: peakCachedInput * ratio.MilliTokensUsd,
				CompletionRatio:  peakOutput / peakInput,
			},
		},
	}
}

// ModelRatios contains the currently available DeepSeek API models and their
// pricing and capability metadata. Model IDs, capabilities, and prices were
// verified on 2026-08-21 against the official DeepSeek documentation:
//   - https://api-docs.deepseek.com/quick_start/pricing/
//   - https://api-docs.deepseek.com/guides/vision/
//   - https://api-docs.deepseek.com/api/list-models/
//   - https://api-docs.deepseek.com/api/create-chat-completion/
//   - https://api-docs.deepseek.com/guides/responses_api/
//
// Images sent to deepseek-v4-flash-vision-exp are converted to prompt tokens by
// DeepSeek and therefore use the normal cache-hit/cache-miss input price. They
// must not be charged with ImagePricing, which is reserved for generated images.
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
		Description:   "DeepSeek-V4-Flash-0731, a 284B/13B-active MoE model with thinking and non-thinking modes, 1M context, and native Responses and Anthropic API support.",
		// The published schedule changes all token prices after 2026-08-16 16:00 UTC.
		TimeWindows: deepseekV4PricingWindows(0.22, 0.007, 0.66, 0.44, 0.014, 1.32),
	},
	// deepseek-v4-flash-vision-exp is the experimental multimodal V4 Flash model.
	// DeepSeek documents the same token prices and limits as deepseek-v4-flash.
	// Each image is converted into prompt tokens (at most 384 tokens per image),
	// and the API response's usage object is authoritative for final billing.
	"deepseek-v4-flash-vision-exp": {
		Ratio:                       deepseekV4FlashInputRatio,
		CachedInputRatio:            deepseekV4FlashCachedInputRatio,
		CompletionRatio:             0.28 / 0.14,
		ContextLength:               1048576,
		MaxOutputTokens:             393216,
		InputModalities:             deepseekVisionInputs,
		OutputModalities:            deepseekTextOutputs,
		SupportedFeatures:           deepseekVisionFeatures,
		SupportedSamplingParameters: deepseekSamplingParams,
		SupportedReasoningEfforts:   deepseekFlashReasoningEfforts,
		DefaultReasoningEffort:      "high",
		Quantization:                "fp4",
		Description:                 "DeepSeek-V4-Flash-Vision-Exp, an experimental multimodal V4 Flash model with text and image input, thinking and non-thinking modes, 1M context, and Chat Completions, Responses, and Anthropic API support.",
		TimeWindows:                 deepseekV4PricingWindows(0.22, 0.007, 0.66, 0.44, 0.014, 1.32),
	},
	// deepseek-v4-pro costs $0.435/1M cache-miss input, $0.003625/1M cache-hit
	// input, and $0.87/1M output before 2026-08-16 16:00 UTC. Its scheduled
	// post-activation prices are $0.66/$0.022/$1.98 off-peak and
	// $1.32/$0.044/$3.96 at peak for cache-miss input/cache-hit input/output.
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
		Description:   "DeepSeek-V4-Pro-0813, a 1.6T/49B-active MoE model with thinking and non-thinking modes, 1M context, and native Responses and Anthropic API support.",
		// The published schedule changes all token prices after 2026-08-16 16:00 UTC.
		TimeWindows: deepseekV4PricingWindows(0.66, 0.022, 1.98, 1.32, 0.044, 3.96),
	},
}

// DeepseekToolingDefaults documents that DeepSeek does not publish separate
// built-in tool prices; server-side web search incurs normal model token usage.
var DeepseekToolingDefaults = adaptor.ChannelToolConfig{}
