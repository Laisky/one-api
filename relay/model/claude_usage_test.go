package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// claudeUsageSum returns the prompt size a Claude client derives from usage.
func claudeUsageSum(usage ClaudeUsage) int {
	return usage.InputTokens + usage.CacheReadInputTokens + usage.CacheCreationInputTokens
}

// TestClaudeUsageFromUsageOpenAISemantics verifies that an OpenAI-semantics
// receipt (prompt includes cache reads and writes) is presented with
// input_tokens excluding both buckets, so the Claude prompt sum equals the
// upstream prompt and cache reads are not counted twice.
func TestClaudeUsageFromUsageOpenAISemantics(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                           string
		usage                          Usage
		input, read, w5m, w1h, prompt  int
		wantCreationObject, wantOutput bool
	}{
		{name: "uncached", usage: Usage{PromptTokens: 1000, CompletionTokens: 40}, input: 1000, prompt: 1000, wantOutput: true},
		{name: "cache_read", usage: Usage{PromptTokens: 1000, CompletionTokens: 40, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 800}}, input: 200, read: 800, prompt: 1000, wantOutput: true},
		{name: "fully_cached", usage: Usage{PromptTokens: 800, CompletionTokens: 40, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 800}}, input: 0, read: 800, prompt: 800, wantOutput: true},
		{name: "read_and_writes", usage: Usage{PromptTokens: 1000, CompletionTokens: 40, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 600}, CacheWrite5mTokens: 150, CacheWrite1hTokens: 50}, input: 200, read: 600, w5m: 150, w1h: 50, prompt: 1000, wantCreationObject: true, wantOutput: true},
		// Writes beyond the uncached remainder are trimmed 1h first, exactly as
		// quota.Compute trims them, so the buckets still sum to the prompt.
		{name: "writes_exceed_remainder", usage: Usage{PromptTokens: 100, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 60}, CacheWrite5mTokens: 30, CacheWrite1hTokens: 30}, input: 0, read: 60, w5m: 30, w1h: 10, prompt: 100, wantCreationObject: true},
		// 12h writes have no Anthropic bucket and remain ordinary input.
		{name: "write_12h_stays_input", usage: Usage{PromptTokens: 1000, CompletionTokens: 1, CacheWrite12hTokens: 300}, input: 1000, prompt: 1000, wantOutput: true},
		{name: "negative_counters_ignored", usage: Usage{PromptTokens: 10, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: -4}, CacheWrite5mTokens: -2}, input: 10, prompt: 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.True(t, tc.usage.PromptIncludesCacheBuckets("gpt-4o"))
			got := ClaudeUsageFromUsage(&tc.usage, "gpt-4o")
			require.Equal(t, tc.input, got.InputTokens)
			require.Equal(t, tc.read, got.CacheReadInputTokens)
			require.Equal(t, tc.w5m+tc.w1h, got.CacheCreationInputTokens)
			require.Equal(t, tc.prompt, claudeUsageSum(got), "input + cache buckets must equal the upstream prompt")
			if tc.wantOutput {
				require.Equal(t, tc.usage.CompletionTokens, got.OutputTokens)
			}
			if !tc.wantCreationObject {
				require.Nil(t, got.CacheCreation)
				return
			}
			require.NotNil(t, got.CacheCreation)
			require.Equal(t, tc.w5m, got.CacheCreation.Ephemeral5mInputTokens)
			require.Equal(t, tc.w1h, got.CacheCreation.Ephemeral1hInputTokens)
		})
	}
}

// TestClaudeUsageFromUsageAnthropicSemantics verifies that usage already in
// Anthropic semantics keeps PromptTokens as input_tokens: the receipt of a
// Claude model (as one-api's own Claude usage convention reports it) and a
// receipt whose cache buckets exceed the prompt, which cannot include them.
// Both are the cases quota.Compute bills as prompt-excludes-cache.
func TestClaudeUsageFromUsageAnthropicSemantics(t *testing.T) {
	t.Parallel()
	claudeReceipt := Usage{PromptTokens: 2000, CompletionTokens: 5, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 500}, CacheWrite1hTokens: 100}
	for _, modelName := range []string{"claude-sonnet-4-5", "anthropic/Claude-Opus-4.1", "us.anthropic.CLAUDE-haiku"} {
		require.False(t, claudeReceipt.PromptIncludesCacheBuckets(modelName), modelName)
		got := ClaudeUsageFromUsage(&claudeReceipt, modelName)
		require.Equal(t, 2000, got.InputTokens, modelName)
		require.Equal(t, 500, got.CacheReadInputTokens, modelName)
		require.Equal(t, 100, got.CacheCreationInputTokens, modelName)
		require.Equal(t, 100, got.CacheCreation.Ephemeral1hInputTokens, modelName)
	}

	bucketsExceed := Usage{PromptTokens: 5, CompletionTokens: 7, PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 3}, CacheWrite5mTokens: 11, CacheWrite1hTokens: 13}
	require.False(t, bucketsExceed.PromptIncludesCacheBuckets("gpt-4o"))
	got := ClaudeUsageFromUsage(&bucketsExceed, "gpt-4o")
	require.Equal(t, ClaudeUsage{InputTokens: 5, OutputTokens: 7, CacheReadInputTokens: 3, CacheCreationInputTokens: 24,
		CacheCreation: &ClaudeCacheCreation{Ephemeral5mInputTokens: 11, Ephemeral1hInputTokens: 13}}, got)
}

// TestClaudeUsageFromUsageNilAndWire verifies the nil receipt yields zero usage
// and that zero cache buckets are omitted from the wire object.
func TestClaudeUsageFromUsageNilAndWire(t *testing.T) {
	t.Parallel()
	require.Equal(t, ClaudeUsage{}, ClaudeUsageFromUsage(nil, "gpt-4o"))
	require.True(t, (*Usage)(nil).PromptIncludesCacheBuckets("gpt-4o"))

	encoded, err := json.Marshal(ClaudeUsageFromUsage(&Usage{PromptTokens: 9, CompletionTokens: 2}, "gpt-4o"))
	require.NoError(t, err)
	require.JSONEq(t, `{"input_tokens":9,"output_tokens":2}`, string(encoded))
}

// TestConvertedClaudeBillingUsage verifies the settlement receipt keeps exactly
// the counters the controller historically re-parsed from converted bodies and
// is nil when the upstream reported no positive counter.
func TestConvertedClaudeBillingUsage(t *testing.T) {
	t.Parallel()
	require.Nil(t, ConvertedClaudeBillingUsage(nil))
	require.Nil(t, ConvertedClaudeBillingUsage(&Usage{PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 3}}))
	require.Nil(t, ConvertedClaudeBillingUsage(&Usage{PromptTokens: -1}))

	source := &Usage{
		PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1234, ServiceTier: "flex", CacheWrite5mTokens: 150,
		CacheWrite1hTokens: 50, CacheWrite12hTokens: 9, CachedTokens: 7,
		PromptTokensDetails:     &UsagePromptTokensDetails{CachedTokens: 600, AudioTokens: 20, TextTokens: 980},
		CompletionTokensDetails: &UsageCompletionTokensDetails{ReasoningTokens: 10},
	}
	receipt := ConvertedClaudeBillingUsage(source)
	require.Equal(t, &Usage{
		PromptTokens: 1000, CompletionTokens: 40, TotalTokens: 1040, CacheWrite5mTokens: 150, CacheWrite1hTokens: 50,
		PromptTokensDetails: &UsagePromptTokensDetails{CachedTokens: 600},
	}, receipt)
	require.NotSame(t, source.PromptTokensDetails, receipt.PromptTokensDetails, "the receipt must not alias the upstream details")

	outputOnly := ConvertedClaudeBillingUsage(&Usage{CompletionTokens: 3, CacheWrite5mTokens: -4})
	require.Equal(t, &Usage{CompletionTokens: 3, TotalTokens: 3}, outputOnly)
}
