package azure

import (
	"slices"

	"github.com/Laisky/one-api/relay/adaptor/openai"
)

// init refreshes Foundry discovery from Microsoft's September 2026 Claude list.
// It takes no arguments and returns nothing. Existing native Messages routing
// and per-family pricing remain unchanged; deployment-specific prices still
// require operator overrides. Access eligibility is not a catalog filter.
// Source: https://learn.microsoft.com/azure/foundry/foundry-models/concepts/claude-models
func init() {
	for _, id := range []string{
		"claude-opus-5-5", "claude-opus-5", "claude-fable-5-1", "claude-mythos-5-1",
	} {
		if !slices.Contains(FoundryClaudeModels, id) {
			FoundryClaudeModels = append(FoundryClaudeModels, id)
		}
	}
	ModelList = append(slices.Clone(openai.ModelList), FoundryClaudeModels...)
}
