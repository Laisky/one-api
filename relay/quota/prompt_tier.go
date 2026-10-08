package quota

import (
	"math"

	relaymodel "github.com/Laisky/one-api/relay/model"
)

// promptTokensForTier returns the full input length used to select a tariff.
// Claude usage excludes cache reads and writes from PromptTokens, while ordinary
// OpenAI-style usage already includes them. Saturation prevents integer overflow
// from selecting a cheaper tier. This does not change the token buckets charged.
func promptTokensForTier(modelName string, usage *relaymodel.Usage) int {
	if usage == nil {
		return 0
	}
	total := max(usage.PromptTokens, 0)
	if !isClaudeModelName(modelName) {
		return total
	}
	cached := 0
	if usage.PromptTokensDetails != nil {
		cached = usage.PromptTokensDetails.CachedTokens
	}
	for _, count := range []int{cached, usage.CacheWrite5mTokens, usage.CacheWrite1hTokens} {
		count = max(count, 0)
		if count > math.MaxInt-total {
			return math.MaxInt
		}
		total += count
	}
	return total
}
