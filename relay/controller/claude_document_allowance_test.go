package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/config"
)

// TestClaudeNativeDocumentAllowancePolicy proves only operator configuration
// adjusts opaque document credit and metadata projection preserves all callers.
func TestClaudeNativeDocumentAllowancePolicy(t *testing.T) {
	original := config.ClaudeNativeDocumentTokenAllowance
	t.Cleanup(func() { config.ClaudeNativeDocumentTokenAllowance = original })
	request := documentQuoteRequest(map[string]any{"type": "file", "file_id": "file_synthetic", "page_count": 0, "tokens": 0}, true)
	before, err := json.Marshal(request)
	require.NoError(t, err)
	config.ClaudeNativeDocumentTokenAllowance = 4096
	first := requireClaudePromptTokens(t, context.Background(), request)
	config.ClaudeNativeDocumentTokenAllowance = 8192
	second := requireClaudePromptTokens(t, context.Background(), request)
	require.Equal(t, 4096, second-first)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after)
	block := map[string]any{"type": "document", "title": "title", "context": "context", "citations": map[string]any{"enabled": true}, "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": strings.Repeat("private fixture data", 10)}}
	copy := claudeDocumentTextMetadata(block)
	require.Equal(t, "title", copy["title"])
	require.Equal(t, "context", copy["context"])
	require.Equal(t, block["citations"], copy["citations"])
	require.NotContains(t, copy["source"], "data")
	require.Contains(t, block["source"], "data")
}
