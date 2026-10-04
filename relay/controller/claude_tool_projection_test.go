package controller

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/openai"
	relaymodel "github.com/Laisky/one-api/relay/model"
)

// TestClaudeToolBlockPromptTokens uses a plain-text message oracle to ensure accepted tool content is counted.
func TestClaudeToolBlockPromptTokens(t *testing.T) {
	text := strings.Repeat("synthetic argument or result ", 1000)
	for _, tc := range []struct {
		name  string
		block map[string]any
	}{
		{"tool_input", map[string]any{"type": "tool_use", "id": "synthetic", "name": "lookup", "input": map[string]any{"query": text}}},
		{"string_result", map[string]any{"type": "tool_result", "tool_use_id": "synthetic", "content": text}},
		{"structured_result", map[string]any{"type": "tool_result", "tool_use_id": "synthetic", "content": []any{map[string]any{"type": "text", "text": text}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &ClaudeMessagesRequest{Model: "claude-3-5-haiku-20241022", Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: []any{tc.block}}}}
			oracle := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "user", Content: text}}, request.Model)
			require.GreaterOrEqual(t, getClaudeMessagesPromptTokens(context.Background(), request), oracle)
		})
	}
}

// TestClaudeToolResultProjection preserves nested images and the request while producing token-counting parts.
func TestClaudeToolResultProjection(t *testing.T) {
	request := &ClaudeMessagesRequest{Model: "claude-sonnet-4", Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: []any{
		map[string]any{"type": "tool_result", "tool_use_id": "synthetic", "content": []any{
			map[string]any{"type": "text", "text": "synthetic text"},
			map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.invalid/synthetic.png"}},
			map[string]any{"type": "image", "source": map[string]any{"type": "file", "file_id": "synthetic-file"}},
		}},
	}}}}
	before, err := json.Marshal(request)
	require.NoError(t, err)
	parts := convertClaudeToOpenAIForTokenCounting(request).Messages[0].ParseContent()
	require.Len(t, parts, 2)
	require.Equal(t, relaymodel.ContentTypeText, parts[0].Type)
	require.Equal(t, "synthetic text", *parts[0].Text)
	require.Equal(t, relaymodel.ContentTypeImageURL, parts[1].Type)
	require.Equal(t, "https://example.invalid/synthetic.png", parts[1].ImageURL.Url)
	require.Equal(t, claudeFileImageFallbackTokens, countClaudeFileImageTokens(request))
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.Equal(t, before, after)
}
