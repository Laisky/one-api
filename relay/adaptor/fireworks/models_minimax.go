package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// minimaxModels contains MiniMax family models served by Fireworks (M2.5, M2.7).
// Sources:
//   - https://fireworks.ai/models/fireworks/minimax-m2p5
//   - https://fireworks.ai/models/fireworks/minimax-m2p7
var minimaxModels = map[string]adaptor.ModelConfig{
	"accounts/fireworks/models/minimax-m2p5": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		CachedInputRatio:            0.03 * ratio.MilliTokensUsd,
		ContextLength:               196608,
		MaxOutputTokens:             196608,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp8",
		HuggingFaceID:               "MiniMaxAI/MiniMax-M2.5",
		Description:                 "MiniMax-M2.5 (228.7B MoE) RL-trained for SOTA coding, agentic tool use, and multi-step office workflows.",
	},
	"accounts/fireworks/models/minimax-m2p7": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		CachedInputRatio:            0.06 * ratio.MilliTokensUsd,
		ContextLength:               196608,
		MaxOutputTokens:             196608,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwChatFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		Quantization:                "fp16",
		HuggingFaceID:               "MiniMaxAI/MiniMax-M2.7",
		Description:                 "MiniMax M2.7 (228.7B MoE) agentic model for complex agent harnesses, dynamic tool search, and elaborate productivity tasks.",
	},
}
