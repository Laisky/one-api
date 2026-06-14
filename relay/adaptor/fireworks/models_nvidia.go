package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// nvidiaModels contains NVIDIA models served by Fireworks (Nemotron family).
// Sources:
//   - https://fireworks.ai/models/fireworks/nemotron-3-ultra-nvfp4
var nvidiaModels = map[string]adaptor.ModelConfig{
	// Nemotron-3 Ultra 550B NVFP4 — $0.60 in / $2.40 out.
	"accounts/fireworks/models/nemotron-3-ultra-nvfp4": {
		Ratio:                       0.60 * ratio.MilliTokensUsd,
		CompletionRatio:             2.40 / 0.60,
		CachedInputRatio:            0.12 * ratio.MilliTokensUsd,
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
