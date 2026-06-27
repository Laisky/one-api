package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// qwenModels contains Alibaba Qwen family chat/VL models served by Fireworks.
// Reranker and embedding members of the Qwen3 family live in models_rerank.go
// and models_embedding.go respectively.
var qwenModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/qwen3-vl-30b-a3b-thinking": {
		Ratio:                       0.15 * ratio.MilliTokensUsd,
		CompletionRatio:             0.60 / 0.15,
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		// Qwen3 thinking models expose a budget-style control; Fireworks' reasoning
		// guide accepts Anthropic-style `thinking.budget_tokens` mapped here. The
		// Qwen3 technical report caps thinking at 38,912 tokens per problem.
		// Sources: https://docs.fireworks.ai/guides/reasoning, Qwen3 technical report (max 38,912 tokens).
		SupportedReasoningEfforts: []string{"low", "medium", "high"},
		DefaultReasoningEffort:    "medium",
		MaxReasoningTokens:        38912,
		Quantization:              "fp16",
		HuggingFaceID:             "Qwen/Qwen3-VL-30B-A3B-Thinking",
		Description:               "Alibaba Qwen3-VL-30B-A3B Thinking (31.1B MoE) multimodal reasoning model with 256K context and visual perception/agent skills.",
	},
	"accounts/fireworks/models/qwen2p5-72b-instruct": {
		Ratio:                       0.90 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen2.5-72B-Instruct",
		Description:                 "Alibaba Qwen2.5 72B Instruct dense model with 128K context, strong multilingual coverage, coding, and math.",
	},
	"accounts/fireworks/models/qwen3-8b": {
		Ratio:                       0.20 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               131072,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen3-8B",
		Description:                 "Alibaba Qwen3 8B dense model with switchable thinking/non-thinking modes and 128K context.",
	},
	"accounts/fireworks/models/qwen3-0p6b": {
		Ratio:                       0.10 * ratio.MilliTokensUsd,
		CompletionRatio:             1.0,
		ContextLength:               32768,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "Qwen/Qwen3-0.6B",
		Description:                 "Alibaba Qwen3 0.6B ultra-compact dense model for embedded chat workloads with 32K context.",
	},
	"accounts/fireworks/models/qwen3p7-plus": {
		Ratio:                       0.40 * ratio.MilliTokensUsd,
		CompletionRatio:             1.60 / 0.40,
		CachedInputRatio:            0.08 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Description:                 "Alibaba Qwen3.7 Plus (MoE) flagship closed multimodal model, available exclusively through Fireworks AI, with 256K context and function calling.",
	},
}
