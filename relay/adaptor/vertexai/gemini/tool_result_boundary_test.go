package vertexai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// claudeToolResultRequest builds a Claude continuation whose single tool_result carries content.
func claudeToolResultRequest(content any) *model.ClaudeRequest {
	return &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 32, Messages: []model.ClaudeMessage{
		{Role: "user", Content: "describe the screenshot"},
		{Role: "assistant", Content: []any{map[string]any{"type": "tool_use", "id": "toolu_1", "name": "screenshot", "input": map[string]any{}}}},
		{Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": "toolu_1", "content": content}}},
	}}
}

// claudeToolResultVertexWire lowers a Claude request through the actual native Vertex request boundary.
func claudeToolResultVertexWire(t *testing.T, request *model.ClaudeRequest) ([]byte, error) {
	t.Helper()
	chat, err := gemini.ConvertClaudeToNativeChat(claudeSignatureContext(), request)
	require.NoError(t, err)
	native, err := (&Adaptor{}).ConvertRequest(claudeSignatureContext(), relaymode.ChatCompletions, chat)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(native)
	require.NoError(t, err)
	return raw, nil
}

// TestVertexClaudeMediaToolResultFailsClosed proves image, document, audio and video tool_result blocks are
// rejected before dispatch instead of being flattened into a functionResponse JSON object.
func TestVertexClaudeMediaToolResultFailsClosed(t *testing.T) {
	image := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "aW1hZ2U="}}
	for name, content := range map[string]any{
		"single_image":       []any{image},
		"text_then_image":    []any{map[string]any{"type": "text", "text": "captured"}, image},
		"image_url_source":   []any{map[string]any{"type": "image", "source": map[string]any{"type": "url", "url": "https://example.com/a.png"}}},
		"pdf_document":       []any{map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "JVBERi0="}}},
		"audio_clip":         []any{map[string]any{"type": "audio", "source": map[string]any{"type": "base64", "media_type": "audio/wav", "data": "UklGRg=="}}},
		"single_object_body": image,
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := claudeToolResultVertexWire(t, claudeToolResultRequest(content))
			require.Error(t, err, "media must not reach Vertex as text: %s", raw)
			require.ErrorContains(t, err, "Vertex multimodal tool results require native response parts")
		})
	}
}

// TestVertexClaudeTextToolResultControls keeps text-only Claude tool results on the native functionResponse path.
func TestVertexClaudeTextToolResultControls(t *testing.T) {
	for name, tc := range map[string]struct {
		content  any
		response string
	}{
		"string":              {content: `{"width":3}`, response: `{"width":3}`},
		"text_blocks":         {content: []any{map[string]any{"type": "text", "text": `{"wid`}, map[string]any{"type": "text", "text": `th":3}`}}, response: `{"width":3}`},
		"plain_text_block":    {content: []any{map[string]any{"type": "text", "text": "done"}}, response: `{"output":"done"}`},
		"unknown_text_block":  {content: []any{map[string]any{"type": "search_result", "title": "t"}}, response: `{"type":"search_result","title":"t"}`},
		"plain_text_document": {content: []any{map[string]any{"type": "document", "source": map[string]any{"type": "text", "media_type": "text/plain", "data": "notes"}}}, response: `{"type":"document","source":{"type":"text","media_type":"text/plain","data":"notes"}}`},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := claudeToolResultVertexWire(t, claudeToolResultRequest(tc.content))
			require.NoError(t, err)
			var wire struct {
				Contents []struct {
					Parts []map[string]json.RawMessage `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(raw, &wire))
			require.Len(t, wire.Contents, 3)
			require.Len(t, wire.Contents[2].Parts, 1)
			require.JSONEq(t, `{"name":"screenshot","response":`+tc.response+`}`, string(wire.Contents[2].Parts[0]["functionResponse"]))
		})
	}
}

// TestVertexTypedTextToolResultKeepsText proves already-validated typed text parts are the function result,
// rather than the empty string the legacy string helper returns for []model.MessageContent.
func TestVertexTypedTextToolResultKeepsText(t *testing.T) {
	first, second, scalar := `{"temperature":`, `20}`, `20`
	for name, tc := range map[string]struct {
		content  any
		response string
	}{
		"typed_json_parts":  {content: []model.MessageContent{{Type: model.ContentTypeText, Text: &first}, {Type: model.ContentTypeText, Text: &second}}, response: `{"temperature":20}`},
		"typed_scalar_text": {content: []model.MessageContent{{Type: model.ContentTypeText, Text: &scalar}}, response: `{"output":20}`},
		"untyped_control":   {content: []any{map[string]any{"type": "text", "text": first}, map[string]any{"type": "text", "text": second}}, response: `{"temperature":20}`},
		"string_control":    {content: first + second, response: `{"temperature":20}`},
	} {
		t.Run(name, func(t *testing.T) {
			request := vertexContinuationRequest()
			request.Messages = request.Messages[:3]
			request.Messages[1].ToolCalls = request.Messages[1].ToolCalls[1:]
			request.Messages[2].Content = tc.content
			var wire struct {
				Contents []struct {
					Parts []map[string]json.RawMessage `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(vertexSchemaWire(t, request), &wire))
			require.Len(t, wire.Contents, 3)
			require.JSONEq(t, `{"name":"weather","response":`+tc.response+`}`, string(wire.Contents[2].Parts[0]["functionResponse"]))
		})
	}
}
