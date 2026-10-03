package aws

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildClaudeUsageMapsCacheBuckets verifies all five token categories are
// distinct in the receipt consumed by the Invoke handlers.
func TestBuildClaudeUsageMapsCacheBuckets(t *testing.T) {
	t.Parallel()
	var receipt invokeUsageReceipt
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":21,"output_tokens":50,"cache_read_input_tokens":180000,"cache_creation_input_tokens":8000,"cache_creation":{"ephemeral_5m_input_tokens":8000}}`)))
	usage := receipt.snapshot()
	require.NotNil(t, usage)
	require.Equal(t, 21, usage.PromptTokens)
	require.Equal(t, 50, usage.CompletionTokens)
	require.Equal(t, 71, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 180000, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 8000, usage.CacheWrite5mTokens)
	require.Zero(t, usage.CacheWrite1hTokens)
}

// TestBuildClaudeUsageLegacyCacheCreationFallback preserves legacy receipts
// that report only a cache-write total instead of the TTL split.
func TestBuildClaudeUsageLegacyCacheCreationFallback(t *testing.T) {
	t.Parallel()
	var receipt invokeUsageReceipt
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":1234,"cache_creation_input_tokens":4321}`)))
	usage := receipt.snapshot()
	require.Equal(t, 1234, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 4321, usage.CacheWrite5mTokens)
	require.Zero(t, usage.CacheWrite1hTokens)
}

// TestAccumulateClaudeStreamUsageMapsCacheBuckets uses actual wire receipts,
// rather than lossy typed event metadata, to verify omitted fields persist and
// cumulative output updates replace rather than add to the initial count.
func TestAccumulateClaudeStreamUsageMapsCacheBuckets(t *testing.T) {
	t.Parallel()
	var receipt invokeUsageReceipt
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":21,"output_tokens":1,"cache_read_input_tokens":180000,"cache_creation":{"ephemeral_5m_input_tokens":8000}}`)))
	for _, raw := range []string{`{"output_tokens":3}`, `{"output_tokens":50}`, `{"output_tokens":50}`} {
		require.NoError(t, receipt.apply(json.RawMessage(raw)))
	}
	usage := receipt.snapshot()
	require.Equal(t, 21, usage.PromptTokens)
	require.Equal(t, 50, usage.CompletionTokens)
	require.Equal(t, 180000, usage.PromptTokensDetails.CachedTokens)
	require.Equal(t, 8000, usage.CacheWrite5mTokens)
	require.Zero(t, usage.CacheWrite1hTokens)
}

// TestInvokeReceiptMissingZeroAndInvalid validates presence semantics and
// verifies that invalid counter updates cannot corrupt billable prior usage.
func TestInvokeReceiptMissingZeroAndInvalid(t *testing.T) {
	t.Parallel()
	var receipt invokeUsageReceipt
	require.Nil(t, receipt.snapshot())
	require.NoError(t, receipt.apply(nil))
	require.NoError(t, receipt.apply(json.RawMessage(`{}`)))
	require.Nil(t, receipt.snapshot())
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":0,"output_tokens":0}`)))
	require.NotNil(t, receipt.snapshot())
	require.Zero(t, receipt.snapshot().TotalTokens)
	require.NoError(t, receipt.apply(json.RawMessage(`{"input_tokens":7,"output_tokens":4}`)))
	before := receipt.snapshot()
	for _, raw := range []string{`{"output_tokens":-1}`, `{"output_tokens":1.5}`, `{"output_tokens":"3"}`, `{"output_tokens":9223372036854775807}`, `[]`} {
		require.Error(t, receipt.apply(json.RawMessage(raw)), raw)
		require.Equal(t, before, receipt.snapshot())
	}
	require.NoError(t, receipt.apply(json.RawMessage(`{"output_tokens":0}`)))
	require.Equal(t, 7, receipt.snapshot().PromptTokens)
	require.Zero(t, receipt.snapshot().CompletionTokens)
}
