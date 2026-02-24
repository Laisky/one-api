package openai

import (
	"testing"

	"github.com/stretchr/testify/require"

	rmodel "github.com/songquanpeng/one-api/relay/model"
)

// TestExtractResponseAPIUsage verifies usage extraction from one websocket response event.
func TestExtractResponseAPIUsage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":10,"output_tokens":20,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":5}}}}`)

	responseID, usage, ok := extractResponseAPIUsage(payload)
	require.True(t, ok)
	require.Equal(t, "resp_123", responseID)
	require.NotNil(t, usage)
	require.Equal(t, 10, usage.PromptTokens)
	require.Equal(t, 20, usage.CompletionTokens)
	require.Equal(t, 30, usage.TotalTokens)
	require.NotNil(t, usage.PromptTokensDetails)
	require.Equal(t, 3, usage.PromptTokensDetails.CachedTokens)
	require.NotNil(t, usage.CompletionTokensDetails)
	require.Equal(t, 5, usage.CompletionTokensDetails.ReasoningTokens)
}

// TestExtractResponseAPIUsageMissingUsage verifies non-usage events are ignored.
func TestExtractResponseAPIUsageMissingUsage(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.created","response":{"id":"resp_123"}}`)

	responseID, usage, ok := extractResponseAPIUsage(payload)
	require.False(t, ok)
	require.Equal(t, "", responseID)
	require.Nil(t, usage)
}

// TestAccumulateResponseAPIUsageDeduplicate verifies repeated snapshots with the same
// response ID are counted only once.
func TestAccumulateResponseAPIUsageDeduplicate(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"type":"response.completed","response":{"id":"resp_123","usage":{"input_tokens":7,"output_tokens":9,"total_tokens":16}}}`)
	usage := &rmodel.Usage{}
	counted := map[string]struct{}{}

	accumulateResponseAPIUsage(payload, usage, counted)
	accumulateResponseAPIUsage(payload, usage, counted)

	require.Equal(t, 7, usage.PromptTokens)
	require.Equal(t, 9, usage.CompletionTokens)
	require.Equal(t, 16, usage.TotalTokens)
	require.Len(t, counted, 1)
}
