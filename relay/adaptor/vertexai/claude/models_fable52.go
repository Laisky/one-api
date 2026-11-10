package vertexai

import (
	"github.com/Laisky/one-api/relay/adaptor"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
)

// init publishes the Google-managed Vertex Claude 5.2 IDs using Anthropic's
// model limits. Numeric values are first-party reference metadata only; the
// public Vertex adaptor excludes them from defaults and requires channel tariffs;
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
			cfg.Description = "Claude Mythos 5.2 on Vertex AI for verified organizations only. Requires explicit operator-verified Google endpoint token and cache tariffs; reference metadata is not a verified Google price. Entitlement is separate."
		} else {
			cfg.Description = "Claude Fable 5.2 on Vertex AI with 1M context, 128K output and always-on adaptive thinking. Requires explicit operator-verified Google endpoint token and cache tariffs; reference metadata is not a verified Google price."
		}
		ModelRatios[model] = cfg
	}
	ModelList = adaptor.GetModelListFromPricing(ModelRatios)
}
