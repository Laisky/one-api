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

// documentQuoteRequest creates a native document or a nested tool-result view.
func documentQuoteRequest(source map[string]any, nested bool) *ClaudeMessagesRequest {
	block := map[string]any{"type": "document", "title": "Synthetic fixture", "context": "Read the fixture.", "source": source}
	if nested {
		block = map[string]any{"type": "tool_result", "tool_use_id": "synthetic-call", "content": []any{block}}
	}
	return &ClaudeMessagesRequest{Model: "claude-3-5-haiku-20241022", Messages: []relaymodel.ClaudeMessage{{Role: "user", Content: []any{block}}}}
}

// TestClaudeNativeDocumentEncodingQuote compares two valid encodings of the
// same rendered PDF page: the page-based estimate must not depend on how the
// page image is compressed.
func TestClaudeNativeDocumentEncodingQuote(t *testing.T) {
	compact, raw := claudeDocumentQuotePDF(t, true), claudeDocumentQuotePDF(t, false)
	require.Greater(t, len(raw), 100*len(compact))
	for _, nested := range []bool{false, true} {
		small := documentQuoteRequest(map[string]any{"type": "base64", "media_type": "application/pdf", "data": compact}, nested)
		large := documentQuoteRequest(map[string]any{"type": "base64", "media_type": "application/pdf", "data": raw}, nested)
		before, err := json.Marshal(large)
		require.NoError(t, err)
		qSmall := requireClaudePromptTokens(t, context.Background(), small)
		qLarge := requireClaudePromptTokens(t, context.Background(), large)
		t.Logf("PDF_QUOTE_ENCODING nested=%v compressed_bytes=%d raw_bytes=%d compressed_quote=%d raw_quote=%d", nested, len(compact), len(raw), qSmall, qLarge)
		require.Equal(t, qSmall, qLarge, "equal rendered pages receive equal page-based estimates regardless of compression")
		require.Greater(t, qSmall, 0, "opaque source still needs an explicit allowance")
		after, err := json.Marshal(large)
		require.NoError(t, err)
		require.Equal(t, before, after, "counting must preserve the provider payload")
	}
}

// TestClaudeNativeDocumentSourceControls retains metadata/text charges and an
// additive source allowance without fetching remote URLs or file handles.
func TestClaudeNativeDocumentSourceControls(t *testing.T) {
	metadata := strings.Repeat("synthetic title context citation ", 2000)
	for _, source := range []map[string]any{
		{"type": "base64", "media_type": "application/pdf", "data": claudeDocumentQuotePDF(t, true)},
		{"type": "file", "file_id": "file_synthetic_document"},
		{"type": "url", "url": "https://example.invalid/synthetic.pdf"},
	} {
		request := documentQuoteRequest(source, false)
		one := requireClaudePromptTokens(t, context.Background(), request)
		require.Positive(t, one, "document bytes and metadata both contribute to the estimate")
		block := request.Messages[0].Content.([]any)[0].(map[string]any)
		block["context"] = metadata
		withContext := requireClaudePromptTokens(t, context.Background(), request)
		require.GreaterOrEqual(t, withContext-one, openai.CountTokenText(metadata, request.Model)-16)
		request.Messages[0].Content = []any{block, block}
		two := requireClaudePromptTokens(t, context.Background(), request)
		require.Greater(t, two, withContext+1000, "a second native document consumes a second allowance")
	}
	text := strings.Repeat("plain document text ", 3000)
	request := documentQuoteRequest(map[string]any{"type": "text", "media_type": "text/plain", "data": text}, false)
	require.GreaterOrEqual(t, requireClaudePromptTokens(t, context.Background(), request), openai.CountTokenText(text, request.Model))
}
