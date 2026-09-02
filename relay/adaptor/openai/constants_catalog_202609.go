package openai

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

var (
	gpt56CanonicalReasoningEfforts = []string{"none", "low", "medium", "high", "xhigh", "max"}

	gpt56SolLongContextTier = adaptor.ModelRatioTier{
		Ratio:               8.0 * ratio.MilliTokensUsd,
		CompletionRatio:     30.0 / 8.0,
		CachedInputRatio:    0.8 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:   10.0 * ratio.MilliTokensUsd,
		InputTokenThreshold: 272_001,
	}

	gpt56CyberLongContextTier = adaptor.ModelRatioTier{
		Ratio:               25.0 * ratio.MilliTokensUsd,
		CompletionRatio:     112.5 / 25.0,
		CachedInputRatio:    2.5 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:   31.25 * ratio.MilliTokensUsd,
		InputTokenThreshold: 272_001,
	}
)

func cloneModelConfig(config adaptor.ModelConfig) adaptor.ModelConfig {
	cloned := config
	cloned.Tiers = slices.Clone(config.Tiers)
	cloned.InputModalities = slices.Clone(config.InputModalities)
	cloned.OutputModalities = slices.Clone(config.OutputModalities)
	cloned.SupportedFeatures = slices.Clone(config.SupportedFeatures)
	cloned.SupportedSamplingParameters = slices.Clone(config.SupportedSamplingParameters)
	cloned.SupportedReasoningEfforts = slices.Clone(config.SupportedReasoningEfforts)

	if config.Audio != nil {
		audio := *config.Audio
		cloned.Audio = &audio
	}
	if config.Image != nil {
		image := *config.Image
		cloned.Image = &image
	}

	return cloned
}

func applyOpenAIModelCatalog202609(modelRatios map[string]adaptor.ModelConfig) map[string]adaptor.ModelConfig {
	sol := adaptor.ModelConfig{
		Ratio:                       4.0 * ratio.MilliTokensUsd,
		CompletionRatio:             20.0 / 4.0,
		CachedInputRatio:            0.4 * ratio.MilliTokensUsd,
		CacheWrite5mRatio:           5.0 * ratio.MilliTokensUsd,
		Tiers:                       []adaptor.ModelRatioTier{gpt56SolLongContextTier},
		ContextLength:               1_050_000,
		MaxOutputTokens:             128_000,
		InputModalities:             []string{"text", "image"},
		OutputModalities:            []string{"text"},
		SupportedFeatures:           append([]string{"web_search"}, gpt5ReasoningFeatures...),
		SupportedSamplingParameters: reasoningSamplingParameters(),
		SupportedReasoningEfforts:   gpt56CanonicalReasoningEfforts,
		DefaultReasoningEffort:      "medium",
		Description:                 "GPT-5.6 Sol: frontier reasoning model with 1.05M context and promotional API pricing.",
	}

	for _, modelName := range []string{"gpt-5.6", "gpt-5.6-sol"} {
		modelRatios[modelName] = cloneModelConfig(sol)
	}

	cyber := cloneModelConfig(sol)
	cyber.Ratio = 12.5 * ratio.MilliTokensUsd
	cyber.CompletionRatio = 75.0 / 12.5
	cyber.CachedInputRatio = 1.25 * ratio.MilliTokensUsd
	cyber.CacheWrite5mRatio = 15.625 * ratio.MilliTokensUsd
	cyber.Tiers = []adaptor.ModelRatioTier{gpt56CyberLongContextTier}
	cyber.ContextLength = 400_000
	cyber.Description = "GPT-5.6 Cyber: specialized cybersecurity reasoning model with separately provisioned access."
	modelRatios["gpt-5.6-cyber"] = cyber

	red := cloneModelConfig(cyber)
	red.Description = "GPT Daybreak Red: alias for gpt-5.6-cyber; separate approval and provisioning required."
	modelRatios["gpt-daybreak-red-latest"] = red

	blue := cloneModelConfig(sol)
	blue.Description = "GPT Daybreak Blue: alias for gpt-5.6-sol; separate approval and provisioning required."
	modelRatios["gpt-daybreak-blue-latest"] = blue

	modelRatios["gpt-transcribe"] = adaptor.ModelConfig{
		Ratio:           7.5 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       0,
			PromptTokensPerSecond: 10,
			UsdPerSecond:          0.0045 / 60,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      "GPT Transcribe: speech-to-text model for file and Realtime transcription.",
	}

	modelRatios["gpt-live-transcribe"] = adaptor.ModelConfig{
		// RelayAudioHelper bills input duration as 10 synthetic audio tokens per second.
		// This ratio therefore resolves to the documented $0.017 per audio minute.
		Ratio:           28.333333333333333 * ratio.MilliTokensUsd,
		CompletionRatio: 1.0,
		Audio: &adaptor.AudioPricingConfig{
			PromptRatio:           16,
			CompletionRatio:       0,
			PromptTokensPerSecond: 10,
			UsdPerSecond:          0.017 / 60,
		},
		InputModalities:  []string{"audio"},
		OutputModalities: []string{"text"},
		Description:      "GPT Live Transcribe: low-latency Realtime transcription model.",
	}

	return modelRatios
}
