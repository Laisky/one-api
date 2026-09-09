package openai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	billingratio "github.com/Laisky/one-api/relay/billing/ratio"
)

// image25ModelRatios describes the GPT Image 2.5 aliases and official snapshots.
// Sources verified 2026-09-09:
//   - https://developers.openai.com/api/docs/models/gpt-image-2.5-sunburst
//   - https://developers.openai.com/api/docs/models/gpt-image-2.5-flare
//   - https://developers.openai.com/api/docs/guides/image-generation
//   - https://developers.openai.com/api/reference/resources/images/methods/generate
//
// Both models charge USD 5/1.25 per million uncached/cached text input tokens,
// USD 8/2 per million uncached/cached image input tokens, and USD 30 per million
// image output tokens. The image controller maintains these five billing buckets.
// Equal token rates do not imply equal costs per image. Do not copy GPT Image 2
// render estimates or add a fixed render fee on top of actual output-token usage.
var image25ModelRatios = map[string]adaptor.ModelConfig{
	"gpt-image-2.5-sunburst": newGPTImage25Config(
		"GPT Image 2.5 Sunburst: high-quality image generation and precision editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-sunburst-2026-09-08": newGPTImage25Config(
		"GPT Image 2.5 Sunburst snapshot (2026-09-08): high-quality image generation and precision editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-flare": newGPTImage25Config(
		"GPT Image 2.5 Flare: fast, high-quality everyday image generation and editing; supports low, medium, high, xhigh, max, and auto quality."),
	"gpt-image-2.5-flare-2026-09-08": newGPTImage25Config(
		"GPT Image 2.5 Flare snapshot (2026-09-08): fast, high-quality everyday image generation and editing; supports low, medium, high, xhigh, max, and auto quality."),
}

// newGPTImage25Config creates independent metadata for one GPT Image 2.5 model.
// Parameters: description identifies the alias or snapshot. Returns: token-only
// pricing and Image API defaults without undocumented context or output limits.
func newGPTImage25Config(description string) adaptor.ModelConfig {
	return adaptor.ModelConfig{
		Ratio:            5 * billingratio.MilliTokensUsd,
		CachedInputRatio: 1.25 * billingratio.MilliTokensUsd,
		CompletionRatio:  30.0 / 5.0, // Image output, not text output.
		Image: &adaptor.ImagePricingConfig{
			// Leave PricePerImageUsd and tier multipliers unset: the controller
			// reconciles actual image-token usage instead of a fixed render fee.
			PromptRatio:      8.0 / 5.0,
			DefaultSize:      "auto",
			DefaultQuality:   "auto",
			PromptTokenLimit: 32000, // Image API prompt-length limit; not context tokens.
			MinImages:        1,
			MaxImages:        10,
		},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"image"},
		Description:      description,
	}
}
