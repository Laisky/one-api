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

// TestClaudePreparedQuoteOwnership verifies provider-visible reasoning, tool arguments, and schemas are quoted without changing the outgoing object.
func TestClaudePreparedQuoteOwnership(t *testing.T) {
	text := strings.Repeat("synthetic reasoning argument schema ", 1000)
	request := &ClaudeMessagesRequest{Model: "claude-3-5-haiku-20241022"}
	outgoing := &relaymodel.GeneralOpenAIRequest{Model: request.Model,
		Messages: []relaymodel.Message{{Role: "assistant", Content: "short", Thinking: &text, ToolCalls: []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: "lookup", Arguments: text}}}}},
		Tools:    []relaymodel.Tool{{Type: "function", Function: &relaymodel.Function{Name: "lookup", Description: text}}},
	}
	before, err := json.Marshal(outgoing)
	require.NoError(t, err)
	quoted, err := preparedClaudePromptTokens(context.Background(), request, outgoing)
	require.NoError(t, err)
	oracle := openai.CountTokenText(text, request.Model)
	require.GreaterOrEqual(t, quoted, 3*oracle)
	after, err := json.Marshal(outgoing)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

// TestClaudeNativeQuotePreservesThinkingPolicy verifies opaque native replay signatures do not become ordinary text charges.
func TestClaudeNativeQuotePreservesThinkingPolicy(t *testing.T) {
	request := &ClaudeMessagesRequest{Model: "claude-3-5-haiku-20241022", Messages: []relaymodel.ClaudeMessage{{Role: "assistant", Content: []any{
		map[string]any{"type": "text", "text": "short reply"},
		map[string]any{"type": "thinking", "thinking": "small thought", "signature": strings.Repeat("synthetic-opaque-signature", 5000)},
		map[string]any{"type": "redacted_thinking", "data": strings.Repeat("synthetic-opaque-data", 5000)},
	}}}}
	oracle := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "assistant", Content: "short reply"}}, request.Model)
	quoted, err := preparedClaudePromptTokens(context.Background(), request, request)
	require.NoError(t, err)
	require.Equal(t, oracle, quoted)
}

// TestClaudeNativeDocumentMetadataQuote verifies all forwarded plain-document title, context, and source text remain in the private quote.
func TestClaudeNativeDocumentMetadataQuote(t *testing.T) {
	text := strings.Repeat("synthetic document guidance ", 1000)
	for _, field := range []string{"title", "context", "source"} {
		t.Run(field, func(t *testing.T) {
			document := map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": "short document"}}
			if field == "source" {
				document["source"].(map[string]any)["data"] = text
			} else {
				document[field] = text
			}
			request := &ClaudeMessagesRequest{Model: "claude-3-5-haiku-20241022", Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: []any{document}}}}
			oracle := openai.CountTokenMessages(context.Background(), []relaymodel.Message{{Role: "user", Content: text}}, request.Model)
			require.GreaterOrEqual(t, requireClaudePromptTokens(t, context.Background(), request), oracle)
		})
	}
}
