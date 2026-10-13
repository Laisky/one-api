package minimax

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// minimaxLegacyModels returns independently constructed legacy catalogue records.
// It takes no parameters. Historical prices are not claims of current availability.
func minimaxLegacyModels() map[string]adaptor.ModelConfig {
	return map[string]adaptor.ModelConfig{
		"M2-her": {
			Ratio: 2.1 * ratio.MilliTokensRmb, CompletionRatio: 8.4 / 2.1,
			ContextLength: 65536, MaxOutputTokens: 2048,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax M2-her dialogue / role-play specialized variant.",
		},
		"abab6.5-chat": {
			Ratio: 30 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 245760, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5 long-context closed-weight chat model (legacy).",
		},
		"abab6.5s-chat": {
			Ratio: 10 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 245760, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5s cost-optimized variant of abab6.5 (legacy).",
		},
		"abab6.5t-chat": {
			Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 8192, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6.5t tiny cost-optimized chat model (legacy).",
		},
		"abab6-chat": {
			Ratio: 100 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 32768, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab6 closed-weight chat model with 32k context (legacy).",
		},
		"abab5.5-chat": {
			Ratio: 15 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 16384, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab5.5 closed-weight chat model (legacy).",
		},
		"abab5.5s-chat": {
			Ratio: 5 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 8192, MaxOutputTokens: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax abab5.5s cost-optimized chat model (legacy).",
		},
		"MiniMax-VL-01": {
			Ratio: 20 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 1000000, MaxOutputTokens: 8192,
			InputModalities: minimaxVisionInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Description: "MiniMax-VL-01 multimodal chat model accepting text and image inputs (legacy estimated pricing).",
		},
		"MiniMax-Text-01": {
			Ratio: 15 * ratio.MilliTokensRmb, CompletionRatio: 1,
			ContextLength: 1000000, MaxOutputTokens: 8192,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxChatFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Quantization: "bf16", HuggingFaceID: "MiniMaxAI/MiniMax-Text-01",
			Description: "MiniMax-Text-01 long-context chat model with open weights on HuggingFace (legacy estimated pricing).",
		},
		"MiniMax-M1": {
			Ratio: 2.8 * ratio.MilliTokensRmb, CompletionRatio: 16.8 / 2.8,
			ContextLength: 1024000, MaxOutputTokens: 40960,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			SupportedFeatures: minimaxReasoningFeatures, SupportedSamplingParameters: minimaxSamplingParams,
			Quantization: "bf16", HuggingFaceID: "MiniMaxAI/MiniMax-M1-80k",
			Description: "MiniMax-M1 reasoning MoE model with 1M context (open weights, legacy estimated pricing).",
		},
		"embo-01": {
			Ratio: 0.5 * ratio.MilliTokensRmb, CompletionRatio: 1, ContextLength: 4096,
			InputModalities: minimaxTextInputs, OutputModalities: minimaxTextOutputs,
			Description: "MiniMax embo-01 text embedding model (legacy estimated pricing).",
		},
	}
}
