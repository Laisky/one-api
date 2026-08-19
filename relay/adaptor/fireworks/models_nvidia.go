package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// nvidiaModels contains NVIDIA models served by Fireworks (Nemotron family).
// Sources:
//   - https://app.fireworks.ai/models/fireworks/nemotron-lightning-3p5-30b-a3b
//   - https://app.fireworks.ai/models/fireworks/nemotron-3-ultra-nvfp4
var nvidiaModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/nemotron-lightning-3p5-30b-a3b": {
		Ratio:                       0.05 * ratio.MilliTokensUsd,
		CompletionRatio:             0.20 / 0.05,
		CachedInputRatio:            0.01 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Description:                 "NVIDIA Nemotron Lightning 3.5 30B A3B serverless text model with 262K context and function calling.",
	},

	// Nemotron-3 Ultra 550B NVFP4 — $0.60 in / $2.40 out.
	"accounts/fireworks/models/nemotron-3-ultra-nvfp4": {
		Ratio:                       0.60 * ratio.MilliTokensUsd,
		CompletionRatio:             2.40 / 0.60,
		CachedInputRatio:            0.119 * ratio.MilliTokensUsd,
		ContextLength:               262144,
		MaxOutputTokens:             32768,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwReasoningSamplingParams,
		Quantization:                "nvfp4",
		HuggingFaceID:               "nvidia/NVIDIA-Nemotron-3-Ultra-550B-A55B-NVFP4",
		Description:                 "NVIDIA Nemotron-3 Ultra 550B NVFP4 reasoning MoE on Fireworks, 262K context, $0.60/$2.40 per 1M.",
	},
}
