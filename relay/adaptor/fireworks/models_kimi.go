package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// kimiModels contains Moonshot Kimi family models served by Fireworks.
// Sources:
//   - https://app.fireworks.ai/models/fireworks/kimi-k3
//   - https://app.fireworks.ai/models/fireworks/kimi-k2p6
//   - https://app.fireworks.ai/models/fireworks/kimi-k2p7-code
var kimiModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/kimi-k3": {
		Ratio:                       3.00 * ratio.MilliTokensUsd,
		CompletionRatio:             15.00 / 3.00,
		CachedInputRatio:            0.30 * ratio.MilliTokensUsd,
		ContextLength:               1048576,
		MaxOutputTokens:             131072,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "moonshotai/Kimi-K3",
		Description:                 "Moonshot Kimi K3 (2.78T MoE) flagship with Kimi Delta Attention, native visual understanding, 1M-token context, function calling, and long-horizon coding/reasoning.",
	},
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
		Description:                 "Moonshot AI Kimi K2.5 (1T MoE) multimodal agentic model unifying vision/text with switchable thinking and multi-agent execution. Retired from Fireworks serverless 2026-06-26; on-demand/dedicated only; migrate to Kimi K2.6.",
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

	// Kimi K2.7 Code — $0.95 in / $4.00 out.
	"accounts/fireworks/models/kimi-k2p7-code": {
		Ratio:                       0.95 * ratio.MilliTokensUsd,
		CompletionRatio:             4.00 / 0.95,
		CachedInputRatio:            0.19 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextImageInModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "moonshotai/Kimi-K2.7-Code",
		Description:                 "Moonshot Kimi K2.7 Code on Fireworks, multimodal coding model, 262K context, $0.95/$4.00 per 1M.",
	},
}
