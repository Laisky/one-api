package openai_compatible

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// claudeToolResultLoweringRequest wraps one tool_result content value in a minimal continuation.
func claudeToolResultLoweringRequest(content any) *model.ClaudeRequest {
	return &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 16, Messages: []model.ClaudeMessage{
		{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "toolu_1", "name": "shot", "input": map[string]any{}}}},
		{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "content": content}}},
	}}
}

// lowerClaudeToolResult runs the actual Claude lowering and returns the produced tool message.
func lowerClaudeToolResult(t *testing.T, content any, native bool) (model.Message, error) {
	t.Helper()
	c, _ := newGinTestContext()
	convert := ConvertClaudeRequest
	if native {
		convert = ConvertClaudeRequestWithNativeMedia
	}
	converted, err := convert(c, claudeToolResultLoweringRequest(content))
	if err != nil {
		return model.Message{}, err
	}
	for _, message := range converted.(*model.GeneralOpenAIRequest).Messages {
		if message.Role == "tool" {
			return message, nil
		}
	}
	t.Fatal("no tool message was produced")
	return model.Message{}, nil
}

// TestClaudeToolResultMediaLowering keeps media types for native routes and the established text elsewhere.
func TestClaudeToolResultMediaLowering(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "captured"},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1n"}},
		map[string]any{"type": "document", "source": map[string]any{"type": "url", "url": "https://example.com/a.pdf"}},
	}
	native, err := lowerClaudeToolResult(t, content, true)
	require.NoError(t, err)
	legacy, err := lowerClaudeToolResult(t, content, false)
	require.NoError(t, err)
	require.Equal(t, legacy.Content, native.Content, "Content keeps the established flattened text on every route")
	require.Empty(t, legacy.ToolResultParts, "non-native routes keep their established contract")
	require.Len(t, native.ToolResultParts, 3)
	require.Equal(t, model.ContentTypeText, native.ToolResultParts[0].Type)
	require.Equal(t, "captured", *native.ToolResultParts[0].Text)
	require.Equal(t, model.ContentTypeImageURL, native.ToolResultParts[1].Type)
	require.Equal(t, "data:image/png;base64,aW1n", native.ToolResultParts[1].ImageURL.Url)
	require.Equal(t, model.ContentTypeFile, native.ToolResultParts[2].Type)
	require.Equal(t, "https://example.com/a.pdf", native.ToolResultParts[2].FileURL)

	for name, textOnly := range map[string]any{
		"string":           "done",
		"text_blocks":      []any{map[string]any{"type": "text", "text": "done"}},
		"unknown":          []any{map[string]any{"type": "search_result", "title": "t"}},
		"text_document":    []any{map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": "notes"}}},
		"content_document": []any{map[string]any{"type": "document", "source": map[string]any{"type": "content", "content": []any{map[string]any{"type": "text", "text": "notes"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			message, err := lowerClaudeToolResult(t, textOnly, true)
			require.NoError(t, err)
			require.Nil(t, message.ToolResultParts, "text-only results keep the single-string contract")
		})
	}
}

// TestClaudeToolResultMalformedMediaNative rejects undecodable media before native lowering only.
func TestClaudeToolResultMalformedMediaNative(t *testing.T) {
	for name, block := range map[string]map[string]any{
		"image_without_source": {"type": "image"},
		"image_unknown_source": {"type": "image", "source": map[string]any{"type": "s3", "uri": "x"}},
		"document_bad_base64":  {"type": "document", "source": map[string]any{"type": "base64", "data": "JVBE"}},
		"audio_wrong_mime":     {"type": "audio", "source": map[string]any{"type": "base64", "media_type": "video/mp4", "data": "AAAA"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := lowerClaudeToolResult(t, []any{block}, true)
			require.ErrorContains(t, err, "validation failed")
			legacy, err := lowerClaudeToolResult(t, []any{block}, false)
			require.NoError(t, err, "non-native routes keep flattening as before")
			require.NotEmpty(t, legacy.Content)
		})
	}
}
