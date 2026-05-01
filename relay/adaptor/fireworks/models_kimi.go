package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// kimiModels contains Moonshot Kimi family models served by Fireworks
// (Kimi K2.5, K2.6). Sources:
//   - https://fireworks.ai/models/fireworks/kimi-k2p5
//   - https://fireworks.ai/models/fireworks/kimi-k2p6
var kimiModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/kimi-k2p5": {
		Ratio:                       0.60 * ratio.MilliTokensUsd,
		CompletionRatio:             3.00 / 0.60,
		CachedInputRatio:            0.10 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "moonshotai/Kimi-K2.5",
		Description:                 "Moonshot AI Kimi K2.5 (1T MoE) multimodal agentic model unifying vision/text with switchable thinking and multi-agent execution.",
	},
	"accounts/fireworks/models/kimi-k2p6": {
		Ratio:                       0.95 * ratio.MilliTokensUsd,
		CompletionRatio:             4.00 / 0.95,
		CachedInputRatio:            0.16 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		MaxOutputTokens:             262144,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "moonshotai/Kimi-K2.6",
		Description:                 "Moonshot AI Kimi K2.6 (1T MoE) native multimodal agentic model for long-horizon coding, autonomous execution, and swarm orchestration.",
	},
}
