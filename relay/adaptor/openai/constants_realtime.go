package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// realtimeModelRatios captures pricing and metadata for OpenAI Realtime models.
// Realtime endpoints stream text and audio chunks bidirectionally; modalities
// reflect this duality while audio-specific pricing is encoded via the Audio
// sub-config.
// Sources verified 2026-05-18:
//   - https://developers.openai.com/api/docs/pricing (Realtime and audio table)
var realtimeModelRatios = map[string]adaptor.ModelConfig{
	// gpt-realtime-2: text $4/$24, audio $32/$64, cached text $0.40, image $5/$0.50
	// Source: https://developers.openai.com/api/docs/pricing#multimodal-models
	"gpt-realtime-2": {
		Ratio:            4.0 * ratio.MilliTokensUsd,
		CompletionRatio:  24.0 / 4.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           8, // $32/$4 = 8x
			CompletionRatio:       2, // $64/$32 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio", "image"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Realtime 2: next-gen bidirectional audio + image input with tool calls.",
	},
	// gpt-realtime-translate: usd-per-minute audio translation endpoint
	// Source: https://developers.openai.com/api/docs/pricing#multimodal-models
	"gpt-realtime-translate": {
		Audio: &adaptor.AudioPricingConfig{
			UsdPerSecond: 0.034 / 60.0, // $0.034 per minute
		},
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"audio"},
		Description:      "GPT Realtime translate: usd-per-minute speech translation endpoint.",
	},
	// gpt-realtime-1.5: text $4/$16, audio $32/$64, cached text $0.40
	"gpt-realtime-1.5": {
		Ratio:            4.0 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           8, // $32/$4 = 8x
			CompletionRatio:       2, // $64/$32 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Realtime 1.5: bidirectional audio streaming with tool calls.",
	},
	// gpt-realtime-mini: text $0.60/$2.40, audio $10/$20, cached text $0.06, image $0.80/$0.08
	"gpt-realtime-mini": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.06 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16.67, // $10/$0.6 ≈ 16.67x
			CompletionRatio:       2,     // $20/$10 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio", "image"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Realtime mini: cost-optimized realtime audio streaming with image input.",
	},
	// gpt-realtime: pinned to gpt-realtime-1.5 pricing ($4 in / $16 out) so existing channels
	// preserve their billing contract; explicit upgrades should request gpt-realtime-2 by name.
	"gpt-realtime": {
		Ratio:            4.0 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.4 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           8,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT Realtime: alias pinned to gpt-realtime-1.5 pricing. Call gpt-realtime-2 explicitly for the latest model.",
	},
	// gpt-4o-realtime-preview: text $5/$20, audio $40/$80, cached text $2.50
	"gpt-4o-realtime-preview": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 2.5 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           8, // $40/$5 = 8x
			CompletionRatio:       2, // $80/$40 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o Realtime preview: streaming audio with tool calling.",
	},
	"gpt-4o-realtime-preview-2025-06-03": {
		Ratio:            5.0 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 2.5 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           8,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o Realtime preview snapshot from 2025-06-03.",
	},
	// gpt-4o-mini-realtime-preview: text $0.60/$2.40, audio $10/$20, cached text $0.30
	"gpt-4o-mini-realtime-preview": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16.67, // $10/$0.6 ≈ 16.67x
			CompletionRatio:       2,     // $20/$10 = 2x
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o mini Realtime preview: low-latency speech for cost-sensitive workloads.",
	},
	"gpt-4o-mini-realtime-preview-2024-12-17": {
		Ratio:            0.6 * ratio.MilliTokensUsd,
		CompletionRatio:  4.0,
		CachedInputRatio: 0.3 * ratio.MilliTokensUsd,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16.67,
			CompletionRatio:       2,
			PromptTokensPerSecond: 10,
		},
		ContextLength:               128000,
		MaxOutputTokens:             4096,
		InputModalities:             []string{"text", "audio"},
		OutputModalities:            []string{"text", "audio"},
		SupportedFeatures:           []string{"tools"},
		SupportedSamplingParameters: standardSamplingParameters(),
		Description:                 "GPT-4o mini Realtime preview snapshot from 2024-12-17.",
	},
}
