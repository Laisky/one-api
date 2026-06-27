package ali

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// deepseekModelRatios captures pricing and metadata for DeepSeek models hosted by
// Alibaba Model Studio. The base DeepSeek-R1 / DeepSeek-V3 weights and the Qwen /
// Llama distills are open-weight; the canonical Hugging Face slugs are recorded
// here. Quantization "bf16" reflects DeepSeek's published served precision.
//
// DeepSeek-R1 and the distill series are reasoning models (chain-of-thought is
// surfaced in their output channels), so SupportedFeatures advertises "reasoning"
// and SupportedSamplingParameters is restricted accordingly. DeepSeek-V3.1 and
// V3.2 are hybrid-thinking models on Bailian; their toggle mirrors the upstream
// `thinking.type` enum and reasoning is therefore advertised.
//
// Sources verified 2026-05-18:
//   - https://huggingface.co/deepseek-ai
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
//   - https://help.aliyun.com/zh/model-studio/siliconflow-deepseek-api
//   - https://www.alibabacloud.com/help/en/model-studio/deepseek-api
var deepseekModelRatios = map[string]adaptor.ModelConfig{
	"deepseek-r1": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             8,
		ContextLength:               16384,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Description:                 "DeepSeek-R1: open-weight reasoning flagship hosted on Alibaba Model Studio.",
	},
	"deepseek-r1-0528": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             8,
		ContextLength:               16384,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-0528",
		Description:                 "DeepSeek-R1-0528 refreshed reasoning checkpoint hosted on Alibaba Model Studio.",
	},
	"deepseek-v3": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               131072,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Description:                 "DeepSeek-V3: open-weight chat flagship hosted on Alibaba Model Studio.",
	},
	"deepseek-v3.1": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		MaxReasoningTokens:          65536,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.1",
		Description:                 "DeepSeek-V3.1 hybrid-thinking chat model hosted on Alibaba Model Studio.",
	},
	"deepseek-v3.2": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		MaxReasoningTokens:          65536,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.2",
		Description:                 "DeepSeek-V3.2 hybrid-thinking chat model hosted on Alibaba Model Studio (recommended default).",
	},
	"deepseek-v3.2-exp": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             4,
		ContextLength:               131072,
		MaxOutputTokens:             65536,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		MaxReasoningTokens:          65536,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3.2-Exp",
		Description:                 "DeepSeek-V3.2 experimental snapshot hosted on Alibaba Model Studio.",
	},
	"deepseek-r1-distill-qwen-1.5b": {
		Ratio:                       0.07 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B",
		Description:                 "DeepSeek-R1 distilled into Qwen 1.5B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-7b": {
		Ratio:                       0.14 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-7B",
		Description:                 "DeepSeek-R1 distilled into Qwen 7B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-14b": {
		Ratio:                       0.28 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-14B",
		Description:                 "DeepSeek-R1 distilled into Qwen 14B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-32b": {
		Ratio:                       0.42 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:                 "DeepSeek-R1 distilled into Qwen 32B (reasoning-focused).",
	},
	"deepseek-r1-distill-llama-8b": {
		Ratio:                       0.14 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Llama-8B",
		Description:                 "DeepSeek-R1 distilled into Llama 8B (reasoning-focused).",
	},
	// deepseek-v4-flash: Alibaba Model Studio China-mainland CNY pricing
	// (verified 2026-06-27 against help.aliyun.com/zh/model-studio/model-pricing):
	// input ¥1 / output ¥2 per 1M. Alibaba bills context-cache hits at 20% of the
	// standard input price (¥1 * 0.20 = ¥0.2), per help.aliyun.com/zh/model-studio/context-cache.
	"deepseek-v4-flash": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CachedInputRatio:            0.2 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               1000000,
		MaxOutputTokens:             393216,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs", "reasoning"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		MaxReasoningTokens:          393216,
		Quantization:                "fp8",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V4-Flash",
		Description:                 "DeepSeek-V4-Flash MoE chat model (hybrid thinking, 1M context) hosted on Alibaba Model Studio.",
	},
	"deepseek-r1-distill-llama-70b": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               32768,
		MaxOutputTokens:             16384,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		MaxReasoningTokens:          16384,
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Llama-70B",
		Description:                 "DeepSeek-R1 distilled into Llama 70B (reasoning-focused).",
	},
}
