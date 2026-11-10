package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
)

// init publishes the Google-managed Vertex Claude 5.2 IDs using Anthropic's
// standard global rates and model limits. Regional premiums remain overrides;
// hosted Anthropic-only tool features are not inherited.
// Sources:
//   - https://platform.claude.com/docs/en/models/fable-5-2/overview
//   - https://platform.claude.com/docs/en/models/mythos-5-2/overview
//   - https://platform.claude.com/docs/en/build-with-claude/claude-on-vertex-ai
func init() {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		cfg := anthropic.ModelRatios[model]
		cfg.SupportedFeatures = []string{"tools", "reasoning"}
		if model == "claude-mythos-5-2" {
			cfg.Description = "Claude Mythos 5.2 on Vertex AI for verified organizations only. Global standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens; regional premiums and entitlements are not assumed."
		} else {
			cfg.Description = "Claude Fable 5.2 on Vertex AI with 1M context, 128K output and always-on adaptive thinking. Global standard $8/$40 input/output, $0.20 cache reads and $10/$16 cache writes per million tokens; region-specific prices require overrides."
		}
		ModelRatios[model] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
