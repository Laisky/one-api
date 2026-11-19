package fireworks

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// minimaxModels contains MiniMax family models served by Fireworks (M2.5, M2.7, M3).
// Sources:
//   - https://fireworks.ai/models/fireworks/minimax-m2p5
//   - https://fireworks.ai/models/fireworks/minimax-m2p7
//   - https://fireworks.ai/models/fireworks/minimax-m3
//
// Additional cards checked 2026-09-24 through 2026-11-16 against
// https://docs.fireworks.ai/serverless/pricing
// https://fireworks.ai/models/fireworks/minimax-m3p1-flash
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
		Description:                 "MiniMax-M2.5 (228.7B MoE) RL-trained for SOTA coding, agentic tool use, and multi-step office workflows. Retired from Fireworks serverless 2026-06-17; on-demand/dedicated only; migrate to MiniMax M2.7.",
	},
	"accounts/fireworks/models/minimax-m2p7": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		CachedInputRatio:            0.059 * ratio.MilliTokensUsd,
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
	"accounts/fireworks/models/minimax-m3": {
		Ratio:                       0.30 * ratio.MilliTokensUsd,
		CompletionRatio:             1.20 / 0.30,
		CachedInputRatio:            0.06 * ratio.MilliTokensUsd,
		ContextLength:               512000,
		MaxOutputTokens:             512000,
		InputModalities:             fwTextOnlyModalities,
		OutputModalities:            fwTextOnlyModalities,
		SupportedFeatures:           fwReasoningFeatures,
		SupportedSamplingParameters: fwChatSamplingParams,
		HuggingFaceID:               "MiniMaxAI/MiniMax-M3",
		Description:                 "MiniMax M3 (428B MoE, about 23B active) with sparse attention and 512K Fireworks context. The Fireworks serverless card currently exposes text-only input despite the base model's native multimodal architecture.",
	},
	"accounts/fireworks/models/minimax-m3p1-flash": {
		Ratio:                       .15 * ratio.MilliTokensUsd,
		CachedInputRatio:            .015 * ratio.MilliTokensUsd,
		CompletionRatio:             fireworksOutputRatio(.60, .15),
		ContextLength:               1_048_576,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedSamplingParameters: []string{"temperature", "top_p", "max_tokens", "max_completion_tokens", "stop", "tools", "tool_choice"},
		Description:                 "Fireworks-hosted MiniMax M3.1 Flash with native text/image input and 1M context. The card describes always-on reasoning with adjustable effort, but does not enumerate the hosted effort vocabulary or default." + " Maximum output and hosted precision are unspecified. Priority, Fast, Ultra-fast and deployment-specific charges are separate operator configuration.",
		SupportedFeatures:           []string{"tools", "reasoning"},
	},
}
