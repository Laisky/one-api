package baiduv2

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Shared metadata helpers for the Qianfan v2 (OpenAI-compatible) ERNIE chat models
// and DeepSeek hosted variants. Reused across ModelRatios entries so the table
// stays compact and consistent.
var (
	// ernieV2TextInputs lists the input modalities for text-only ERNIE v2 chat models.
	ernieV2TextInputs = []string{"text"}
	// ernieV2TextOutputs lists the output modalities for ERNIE v2 chat completions.
	ernieV2TextOutputs = []string{"text"}

	// ernieV2ChatFeatures lists the capability set for ERNIE 4.0/3.5 chat tiers
	// that support tool-calling and JSON mode on the Qianfan v2 API.
	ernieV2ChatFeatures = []string{"tools", "json_mode"}
	// ernieV2BasicFeatures lists the reduced capability set for Speed/Lite/Tiny/
	// Character/Novel tiers — tool-calling per Qianfan docs but no advertised JSON mode.
	ernieV2BasicFeatures = []string{"tools"}

	// deepseekV2ChatFeatures advertises the capability set for non-thinking DeepSeek
	// models hosted on Baidu Qianfan v2.
	deepseekV2ChatFeatures = []string{"tools", "json_mode", "structured_outputs"}
	// deepseekV2ReasoningFeatures advertises the capability set for thinking-mode
	// DeepSeek and DeepSeek-R1-distilled models hosted on Baidu Qianfan v2.
	deepseekV2ReasoningFeatures = []string{"tools", "json_mode", "structured_outputs", "reasoning"}

	// qianfanV2SamplingParameters lists OpenAI-compatible sampling parameters Qianfan v2
	// accepts. Baidu also exposes top_k and a repetition penalty alongside the
	// standard OpenAI knobs for Chinese-cloud chat APIs.
	qianfanV2SamplingParameters = []string{
		"temperature",
		"top_p",
		"top_k",
		"frequency_penalty",
		"presence_penalty",
		"repetition_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
	// deepseekV2SamplingParameters lists the standard OpenAI-compatible sampling
	// parameters supported by hosted DeepSeek chat models on Qianfan v2.
	deepseekV2SamplingParameters = []string{
		"temperature",
		"top_p",
		"frequency_penalty",
		"presence_penalty",
		"stop",
		"seed",
		"max_tokens",
	}
)

// ModelRatios contains all supported models and their pricing/configuration metadata.
// Model list is derived from the keys of this map, eliminating redundancy.
//
// Pricing source: https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9do2 (verified 2026-05-01).
//
// Capability metadata sources:
//   - https://cloud.baidu.com/doc/qianfan-api/s/ (Qianfan v2 API reference)
//   - https://ai.baidu.com/ai-doc/AISTUDIO/Mmhslv9lf (per-model context/output limits)
//   - https://huggingface.co/baidu (open-weight ERNIE 4.5 family)
//   - https://huggingface.co/deepseek-ai (hosted DeepSeek chat models)
var ModelRatios = map[string]adaptor.ModelConfig{
	// ERNIE 4.0 Models
	"ernie-4.0-8k-latest": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K (latest alias): closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k-preview": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K (preview): closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-8k": {
		Ratio:                       0.12 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 8K: closed-weight flagship chat model on Qianfan v2.",
	}, // CNY 0.12 / 1k tokens
	"ernie-4.0-turbo-8k-latest": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K (latest alias): closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k-preview": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K (preview): closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-8k": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 8K: closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens
	"ernie-4.0-turbo-128k": {
		Ratio:                       0.02 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 4.0 Turbo 128K: long-context closed-weight balanced-cost chat model.",
	}, // CNY 0.02 / 1k tokens

	// ERNIE 3.5 Models
	"ernie-3.5-8k-preview": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K (preview): closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens
	"ernie-3.5-8k": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 8K: closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens
	"ernie-3.5-128k": {
		Ratio:                       0.012 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2ChatFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE 3.5 128K: long-context closed-weight general-purpose chat model.",
	}, // CNY 0.012 / 1k tokens

	// ERNIE Speed Models
	"ernie-speed-8k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed 8K: throughput-optimized closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.004 / 1k tokens
	"ernie-speed-128k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed 128K: long-context throughput-optimized closed-weight chat tier.",
	}, // CNY 0.004 / 1k tokens
	"ernie-speed-pro-128k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Speed Pro 128K: enhanced throughput-optimized closed-weight chat tier.",
	}, // CNY 0.004 / 1k tokens

	// ERNIE Lite Models
	"ernie-lite-8k": {
		Ratio:                       0.008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Lite 8K: cost-efficient closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.008 / 1k tokens
	"ernie-lite-pro-128k": {
		Ratio:                       0.008 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               131072,
		MaxOutputTokens:             4096,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Lite Pro 128K: long-context cost-efficient closed-weight chat tier.",
	}, // CNY 0.008 / 1k tokens

	// ERNIE Tiny Models
	"ernie-tiny-8k": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Tiny 8K: ultra low-cost closed-weight chat tier on Qianfan v2.",
	}, // CNY 0.004 / 1k tokens

	// ERNIE Character Models
	"ernie-char-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Character 8K: closed-weight role-play / persona chat model.",
	}, // CNY 0.04 / 1k tokens
	"ernie-char-fiction-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Character Fiction 8K: closed-weight model tuned for fictional persona dialogue.",
	}, // CNY 0.04 / 1k tokens
	"ernie-novel-8k": {
		Ratio:                       0.04 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               8192,
		MaxOutputTokens:             2048,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           ernieV2BasicFeatures,
		SupportedSamplingParameters: qianfanV2SamplingParameters,
		Description:                 "Baidu ERNIE Novel 8K: closed-weight model specialized for long-form fiction continuation.",
	}, // CNY 0.04 / 1k tokens

	// DeepSeek Models (hosted on Baidu)
	"deepseek-v3": {
		Ratio:                       0.01 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ChatFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Description:                 "DeepSeek V3 hosted on Baidu Qianfan v2: open-weight MoE chat model (non-thinking mode).",
	}, // CNY 0.01 / 1k tokens
	"deepseek-r1": {
		Ratio:                       0.01 * ratio.MilliTokensRmb,
		CompletionRatio:             8,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Description:                 "DeepSeek R1 hosted on Baidu Qianfan v2: open-weight reasoning chat model (thinking mode).",
	}, // CNY 0.01 / 1k tokens
	"deepseek-r1-distill-qwen-32b": {
		Ratio:                       0.004 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:                 "DeepSeek R1 distilled into Qwen-32B, hosted on Baidu Qianfan v2.",
	}, // CNY 0.004 / 1k tokens
	"deepseek-r1-distill-qwen-14b": {
		Ratio:                       0.003 * ratio.MilliTokensRmb,
		CompletionRatio:             1,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             ernieV2TextInputs,
		OutputModalities:            ernieV2TextOutputs,
		SupportedFeatures:           deepseekV2ReasoningFeatures,
		SupportedSamplingParameters: deepseekV2SamplingParameters,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-14B",
		Description:                 "DeepSeek R1 distilled into Qwen-14B, hosted on Baidu Qianfan v2.",
	}, // CNY 0.003 / 1k tokens
}

// ModelList derived from ModelRatios for backward compatibility
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// BaiduV2ToolingDefaults records that the updated Wenxin billing docs require authentication for tool pricing (retrieved 2025-11-12).
// Source: https://r.jina.ai/https://cloud.baidu.com/doc/WENXINWORKSHOP/s/Blfmc9do2 (restricted access)
var BaiduV2ToolingDefaults = adaptor.ChannelToolConfig{}
