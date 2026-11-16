package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/billing/ratio"
)

// november2026ModelRatios records the exact newly published native OpenAI IDs.
// Verified 2026-11-16 against the model cards and Standard pricing table:
// https://developers.openai.com/api/docs/models/gpt-6-cyber
// https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst
// https://developers.openai.com/api/docs/models/gpt-image-2.6-flare
// https://developers.openai.com/api/docs/pricing
// Access approval remains an upstream responsibility. No dated snapshots or
// undocumented reasoning-effort ladder are inferred from the release date.
var november2026ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-6-cyber": {
		Ratio:             20 * ratio.MilliTokensUsd,
		CompletionRatio:   100.0 / 20.0,
		CachedInputRatio:  2 * ratio.MilliTokensUsd,
		CacheWrite5mRatio: 25 * ratio.MilliTokensUsd, // Legacy field name; OpenAI uses 30m.
		Tiers: []adaptor.ModelRatioTier{{
			Ratio:               40 * ratio.MilliTokensUsd,
			CompletionRatio:     150.0 / 40.0,
			CachedInputRatio:    4 * ratio.MilliTokensUsd,
			InputTokenThreshold: 272_001,
			// Long-context cache-write rates are not published. Quota marks
			// such receipts unpriced rather than settling the inherited rate.
		}},
		ContextLength:    1_050_000,
		MaxOutputTokens:  128_000,
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		SupportedFeatures: []string{
			"reasoning", "streaming", "tools", "structured_outputs", "web_search", "prompt_caching",
		},
		Description: "GPT-6 Cyber: reasoning for authorized security research; requires upstream Daybreak Red approval. Standard pricing; inputs over 272K use the long-context tier. Long-context cache writes and 12h cache retention require separately qualified billing.",
	},
	"gpt-image-2.6-sunburst": newGPTImage25Config(
		"GPT Image 2.6 Sunburst: image generation and editing for detailed visual work; billed from text/image input and image output tokens."),
	"gpt-image-2.6-flare": newGPTImage25Config(
		"GPT Image 2.6 Flare: image generation and editing for fast iteration; billed from text/image input and image output tokens."),
}
