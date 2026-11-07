package quota

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	modelcfg "github.com/Laisky/one-api/model"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaudeUsagePresentationMatchesBilling pins relaymodel.ClaudeUsageFromUsage
// to the cache-bucket interpretation Compute settles with: pricing the presented
// Anthropic buckets (fresh input, cache read, 5m and 1h writes) at their own
// prices must reproduce the billed quota, for OpenAI-semantics receipts, Claude
// model receipts, and receipts whose buckets exceed the prompt. A drift between
// the two interpretations would show Claude clients a prompt size that differs
// from what was billed.
func TestClaudeUsagePresentationMatchesBilling(t *testing.T) {
	t.Parallel()
	const (
		inputPrice, readPrice, write5mPrice, write1hPrice, outputPrice = 1.0, 0.25, 1.25, 2.0, 1.0
	)
	pricingFor := func(modelName string) map[string]modelcfg.ModelConfigLocal {
		return map[string]modelcfg.ModelConfigLocal{modelName: {
			Ratio: inputPrice, CompletionRatio: outputPrice / inputPrice, CachedInputRatio: readPrice,
			CacheWrite5mRatio: write5mPrice, CacheWrite1hRatio: write1hPrice,
		}}
	}
	details := func(cached int) *relaymodel.UsagePromptTokensDetails {
		return &relaymodel.UsagePromptTokensDetails{CachedTokens: cached}
	}
	for _, tc := range []struct {
		name, modelName string
		usage           relaymodel.Usage
		includes        bool
	}{
		{name: "openai_uncached", modelName: "gpt-4o", usage: relaymodel.Usage{PromptTokens: 1000, CompletionTokens: 40}, includes: true},
		{name: "openai_cached", modelName: "gpt-4o", usage: relaymodel.Usage{PromptTokens: 1000, CompletionTokens: 40, PromptTokensDetails: details(800)}, includes: true},
		{name: "openai_cached_writes", modelName: "gemini-2.5-flash", usage: relaymodel.Usage{PromptTokens: 1000, CompletionTokens: 40, PromptTokensDetails: details(600), CacheWrite5mTokens: 120, CacheWrite1hTokens: 80}, includes: true},
		{name: "openai_writes_trimmed", modelName: "gpt-4o", usage: relaymodel.Usage{PromptTokens: 100, CompletionTokens: 8, PromptTokensDetails: details(60), CacheWrite5mTokens: 32, CacheWrite1hTokens: 24}, includes: true},
		{name: "claude_model", modelName: "claude-sonnet-4-5", usage: relaymodel.Usage{PromptTokens: 1000, CompletionTokens: 40, PromptTokensDetails: details(800), CacheWrite5mTokens: 120, CacheWrite1hTokens: 80}},
		{name: "buckets_exceed_prompt", modelName: "gpt-4o", usage: relaymodel.Usage{PromptTokens: 8, CompletionTokens: 4, PromptTokensDetails: details(12), CacheWrite5mTokens: 4}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			usage := tc.usage
			require.Equal(t, tc.includes, usage.PromptIncludesCacheBuckets(tc.modelName))
			require.Equal(t, !tc.includes, isClaudeModelName(tc.modelName) ||
				usage.PromptTokensDetails != nil && usage.PromptTokensDetails.CachedTokens > usage.PromptTokens ||
				usage.CacheWrite5mTokens+usage.CacheWrite1hTokens > usage.PromptTokens,
				"PromptIncludesCacheBuckets must mirror Compute's promptExcludesCacheBuckets")

			presented := relaymodel.ClaudeUsageFromUsage(&usage, tc.modelName)
			write5m, write1h := 0, 0
			if presented.CacheCreation != nil {
				write5m, write1h = presented.CacheCreation.Ephemeral5mInputTokens, presented.CacheCreation.Ephemeral1hInputTokens
			}
			require.Equal(t, presented.CacheCreationInputTokens, write5m+write1h)
			want := int64(math.Ceil(float64(presented.InputTokens)*inputPrice + float64(presented.CacheReadInputTokens)*readPrice +
				float64(write5m)*write5mPrice + float64(write1h)*write1hPrice + float64(presented.OutputTokens)*outputPrice))

			billed := Compute(ComputeInput{
				Usage: &usage, ModelName: tc.modelName, ModelRatio: inputPrice, GroupRatio: 1,
				ChannelModelConfigs: pricingFor(tc.modelName), RequestTime: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC),
			})
			require.Equal(t, want, billed.TotalQuota, "presented buckets priced individually must equal the billed quota")
		})
	}
}
