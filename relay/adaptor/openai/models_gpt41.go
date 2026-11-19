package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// gpt41ModelRatios captures pricing and metadata for the GPT-4.1 family.
// All variants advertise a ~1M token context window with image input support.
// Source: https://platform.openai.com/docs/models/gpt-4.1
const gpt41ContextLength = int32(1_047_576)

var gpt41ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-4.1": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.5 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1: 1M-context multimodal flagship for advanced coding and instruction following.",
	},
	"gpt-4.1-2025-04-14": {
		Ratio:                       2.0 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.5 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1 snapshot from 2025-04-14.",
	},
	"gpt-4.1-mini": {
		Ratio:                       0.4 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.1 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1 mini: 1M-context cost-efficient multimodal model.",
	},
	"gpt-4.1-mini-2025-04-14": {
		Ratio:                       0.4 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.1 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1 mini snapshot from 2025-04-14.",
	},
	"gpt-4.1-nano": {
		Ratio:                       0.1 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.025 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1 nano: ultra-low-cost multimodal model with 1M-token context.",
	},
	"gpt-4.1-nano-2025-04-14": {
		Ratio:                       0.1 * ratio.MilliTokensUsd,
		CompletionRatio:             4.0,
		CachedInputRatio:            0.025 * ratio.MilliTokensUsd,
		ContextLength:               gpt41ContextLength,
		MaxOutputTokens:             32768,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           []string{"tools", "json_mode", "structured_outputs"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4.1 nano snapshot from 2025-04-14.",
	},
}
