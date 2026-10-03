package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
)

// init registers the documented, undated Vertex AI Opus 5.5 ID and rebuilds
// ModelList so the parent channel registry can discover it. It takes no
// parameters and returns no values. Prices are global list prices; regional
// premiums and account-specific rates remain channel configuration concerns.
//
// Sources (verified 2026-09-22):
//   - https://platform.claude.com/docs/en/models/opus-5-5/overview
//   - https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
//   - https://platform.claude.com/docs/en/about-claude/pricing
func init() {
	// Anthropic's package initialization completes before this package starts.
	// Reuse the canonical limits and rates without duplicating pricing values.
	config := anthropic.ModelRatios["claude-opus-5-5"]
	// Do not advertise first-party server tools merely because the base model
	// supports them. This profile lists the common client-tool/reasoning core.
	config.SupportedFeatures = []string{"tools", "reasoning"}
	config.Description = "Claude Opus 5.5 on Vertex AI with 1M-token context, 128K output, and always-on adaptive thinking (default effort: medium). Global list input/output pricing is $4/$20 per million tokens; cache reads are $0.20 per million tokens. Regional premiums are not included."
	ModelRatios["claude-opus-5-5"] = config
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
