package geminiOpenaiCompatible

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// Catalog sources (existing verified contracts):
// - https://ai.google.dev/gemini-api/docs/pricing
// - https://cloud.google.com/gemini-enterprise-agent-platform/generative-ai/pricing
// - https://ai.google.dev/gemini-api/docs/models
// - https://ai.google.dev/gemini-api/docs/changelog
// - https://ai.google.dev/gemini-api/docs/models/gemini-3-pro-image
// - https://ai.google.dev/gemma/docs/
// - https://huggingface.co/google
// - https://ai.google.dev/gemini-api/docs/live
// - https://ai.google.dev/gemini-api/docs/openai
// - https://ai.google.dev/gemini-api/docs/thinking
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-image
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.1-flash-lite-image
// - https://deepmind.google/models/gemini/flash/
// - https://ai.google.dev/gemini-api/docs/latest-model
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-live-extended-thinking
// - https://ai.google.dev/gemini-api/docs/deprecations
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-tts
// - https://ai.google.dev/gemini-api/docs/models/gemini-3.8-flash-lite-tts
// - https://ai.google.dev/gemini-api/docs/models/gemini-4-argon
// - https://ai.google.dev/gemini-api/docs/generate-content/latest-model
// - https://ai.google.dev/gemini-api/docs/models/gemini-nano-banana-2.1
// - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/nano-banana-2-1
// - https://aistudio.google.com/docs/models/gemini-4-carbon
// - https://aistudio.google.com/docs/pricing#gemini-4-carbon
// - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-carbon
// - https://aistudio.google.com/docs/latest-model
// - https://aistudio.google.com/docs/pricing#gemini-4-hydrogen
// - https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/gemini/4-hydrogen
// - https://aistudio.google.com/docs/changelog

// ModelRatios assembles complete families before any importing adaptor reads
// pricing or discovery. Lifecycle descriptions and metadata are stored with
// their model definitions, so initialization no longer depends on filenames.
var ModelRatios = adaptor.JoinModelCatalogs(
	gemini2ModelRatios, gemini3ModelRatios, gemini4ModelRatios,
	audioModelRatios, imageModelRatios, embeddingModelRatios,
	gemmaModelRatios, roboticsModelRatios, videoModelRatios,
)

// ModelList derives discovery from the complete catalog without init patches.
var ModelList = adaptor.GetModelListFromPricing(ModelRatios)

// geminiStandardPriceWindow builds the full-day UTC transition to published 2027 rates.
// Parameters: inputUsd, outputUsd, and cachedInputUsd are USD per million tokens.
// Returns: an independently allocated pricing window that preserves other metadata.
func geminiStandardPriceWindow(inputUsd, outputUsd, cachedInputUsd float64) adaptor.TimeWindow {
	return adaptor.TimeWindow{
		Name:     "standard-pricing-from-2027",
		TimeZone: "UTC",
		DateFrom: "2027-01-01",
		Ranges:   []adaptor.ClockRange{{Start: "00:00", End: "00:00"}},
		Overlay: adaptor.ModelConfig{
			Ratio:            inputUsd * ratio.MilliTokensUsd,
			CompletionRatio:  outputUsd / inputUsd,
			CachedInputRatio: cachedInputUsd * ratio.MilliTokensUsd,
		},
	}
}
