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
// and SupportedSamplingParameters is restricted accordingly.
//
// Sources verified 2026-05-01:
//   - https://huggingface.co/deepseek-ai
//   - https://help.aliyun.com/zh/model-studio/getting-started/models
var deepseekModelRatios = map[string]adaptor.ModelConfig{
	"deepseek-r1": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             8,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1",
		Description:                 "DeepSeek-R1: open-weight reasoning flagship hosted on Alibaba Model Studio.",
	},
	"deepseek-v3": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               65536,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: qwenStandardSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-V3",
		Description:                 "DeepSeek-V3: open-weight chat flagship hosted on Alibaba Model Studio.",
	},
	"deepseek-r1-distill-qwen-1.5b": {
		Ratio:                       0.07 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-1.5B",
		Description:                 "DeepSeek-R1 distilled into Qwen 1.5B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-7b": {
		Ratio:                       0.14 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-7B",
		Description:                 "DeepSeek-R1 distilled into Qwen 7B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-14b": {
		Ratio:                       0.28 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-14B",
		Description:                 "DeepSeek-R1 distilled into Qwen 14B (reasoning-focused).",
	},
	"deepseek-r1-distill-qwen-32b": {
		Ratio:                       0.42 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Qwen-32B",
		Description:                 "DeepSeek-R1 distilled into Qwen 32B (reasoning-focused).",
	},
	"deepseek-r1-distill-llama-8b": {
		Ratio:                       0.14 * ratio.MilliTokensRmb,
		CompletionRatio:             0.28,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Llama-8B",
		Description:                 "DeepSeek-R1 distilled into Llama 8B (reasoning-focused).",
	},
	"deepseek-r1-distill-llama-70b": {
		Ratio:                       1 * ratio.MilliTokensRmb,
		CompletionRatio:             2,
		ContextLength:               32768,
		MaxOutputTokens:             8192,
		InputModalities:             []string{"text"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"reasoning"},
		SupportedSamplingParameters: qwenReasoningSamplingParameters(),
		Quantization:                "bf16",
		HuggingFaceID:               "deepseek-ai/DeepSeek-R1-Distill-Llama-70B",
		Description:                 "DeepSeek-R1 distilled into Llama 70B (reasoning-focused).",
	},
}
