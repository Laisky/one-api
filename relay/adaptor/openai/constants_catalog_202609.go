package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var gpt56CanonicalEfforts = []string{"none", "low", "medium", "high", "xhigh", "max"}

var gpt56SolCurrentLongContextTier = adaptor.ModelRatioTier{
	Ratio:               8.0 * ratio.MilliTokensUsd,
	CompletionRatio:     30.0 / 8.0,
	CachedInputRatio:    0.8 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   10.0 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

var gpt56CyberLongContextTier = adaptor.ModelRatioTier{
	Ratio:               25.0 * ratio.MilliTokensUsd,
	CompletionRatio:     112.5 / 25.0,
	CachedInputRatio:    2.5 * ratio.MilliTokensUsd,
	CacheWrite5mRatio:   31.25 * ratio.MilliTokensUsd,
	InputTokenThreshold: 272_001,
}

// applyOpenAIModelCatalog202609 applies catalog entries and pricing verified on
// 2026-09-01. It mutates and returns models so ModelList is derived from the same
// final map during package initialization.
func applyOpenAIModelCatalog202609(models map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	for _, modelName := range []string{"gpt-5.6", "gpt-5.6-sol"} {
		cfg := models[modelName].Clone()
		cfg.Ratio = 4.0 * ratio.MilliTokensUsd
		cfg.CompletionRatio = 20.0 / 4.0
		cfg.CachedInputRatio = 0.4 * ratio.MilliTokensUsd
		cfg.CacheWrite5mRatio = 5.0 * ratio.MilliTokensUsd
		cfg.Tiers = []adaptor.ModelRatioTier{gpt56SolCurrentLongContextTier}
		cfg.SupportedReasoningEfforts = append([]string(nil), gpt56CanonicalEfforts...)
		models[modelName] = cfg
	}

	for _, modelName := range []string{"gpt-5.6-terra", "gpt-5.6-luna"} {
		cfg := models[modelName].Clone()
		cfg.SupportedReasoningEfforts = append([]string(nil), gpt56CanonicalEfforts...)
		models[modelName] = cfg
	}

	cyber := adaptor.ModelConfig{
		Ratio:                       12.5 * ratio.MilliTokensUsd,
		CompletionRatio:             75.0 / 12.5,
		CachedInputRatio:            1.25 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           15.625 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt56CyberLongContextTier},
		ContextLength:               400_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   append([]string(nil), gpt56CanonicalEfforts...),
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-5.6 Cyber: advanced cybersecurity reasoning model for approved, authorized defensive research.",
	}
	models["gpt-5.6-cyber"] = cyber

	daybreakRed := cyber.Clone()
	daybreakRed.Description = "Daybreak Red: approved cybersecurity alias that currently maps to gpt-5.6-cyber."
	models["gpt-daybreak-red-latest"] = daybreakRed

	daybreakBlue := models["gpt-5.6-sol"].Clone()
	daybreakBlue.Description = "Daybreak Blue: approved defensive-cybersecurity alias that currently maps to gpt-5.6-sol."
	models["gpt-daybreak-blue-latest"] = daybreakBlue

	models["gpt-transcribe"] = adaptor.ModelConfig{
		Ratio:           7.5 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptTokensPerSecond: 10,
			UsdPerSecond:          0.0045 / 60.0,
		},
		InputModalities:  []string{"audio", "text"},
		OutputModalities: []string{"text"},
		Description:      "GPT Transcribe: high-accuracy speech-to-text for files, streamed files, and committed Realtime turns.",
	}

	models["gpt-live-transcribe"] = adaptor.ModelConfig{
		Audio: &adaptor.AudioPricingConfig{
			UsdPerSecond: 0.017 / 60.0,
		},
		InputModalities:  []string{"audio", "text"},
		OutputModalities: []string{"text"},
		Description:      "GPT Live Transcribe: low-latency streaming speech-to-text with tunable latency and context hints.",
	}

	return models
}
