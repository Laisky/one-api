package anthropic

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// toolMediaPayload is the base64 payload of the inline test image.
const toolMediaPayload = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// toolMediaText returns a text part for tool result fixtures.
// Parameters: text is the part text. Returns: the part.
func toolMediaText(text string) model.MessageContent {
	return model.MessageContent{Type: model.ContentTypeText, Text: &text}
}

// toolMediaImage returns an image_url part for tool result fixtures.
// Parameters: url is the image URL. Returns: the part.
func toolMediaImage(url string) model.MessageContent {
	return model.MessageContent{Type: model.ContentTypeImageURL, ImageURL: &model.ImageURL{Url: url, Detail: "high"}}
}

// toolMediaChatRequest builds the Chat history the Responses fallback produces
// for a parallel tool turn whose first output carries an image.
// Parameters: withParts controls whether the side-channel parts survive.
// Returns: the Chat request.
func toolMediaChatRequest(withParts bool) model.GeneralOpenAIRequest {
	imageURL := "data:image/png;base64," + toolMediaPayload
	view := model.Message{Role: "tool", ToolCallId: "toolu_view", Content: "a.png"}
	if withParts {
		view.ToolResultParts = []model.MessageContent{toolMediaText("a.png"), toolMediaImage(imageURL)}
	}
	label := "[Image output from tool call toolu_view]"
	return model.GeneralOpenAIRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 2048,
		Messages: []model.Message{
			{Role: "user", Content: "look"},
			{Role: "assistant", Content: "", ToolCalls: []model.Tool{
				{Id: "toolu_view", Type: "function", Function: &model.Function{Name: "view_image", Arguments: `{"path":"a.png"}`}},
				{Id: "toolu_shell", Type: "function", Function: &model.Function{Name: "shell", Arguments: `{}`}},
			}},
			view,
			{Role: "tool", ToolCallId: "toolu_shell", Content: "ok"},
			{Role: "user", ToolResultMediaCarrier: true, Content: []model.MessageContent{
				{Type: model.ContentTypeText, Text: &label}, toolMediaImage(imageURL),
			}},
			{Role: "user", Content: "next"},
		},
	}
}

// encodedClaudeMessages converts a Chat request and returns its wire messages.
// Parameters: t is the test handle and request the Chat request. Returns: the
// decoded upstream messages array.
func encodedClaudeMessages(t *testing.T, request model.GeneralOpenAIRequest) []map[string]any {
	t.Helper()
	converted, err := ConvertRequest(newThinkingContext(t, "/v1/chat/completions"), request)
	require.NoError(t, err)
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	var wire struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	return wire.Messages
}

// TestConvertRequestToolResultCarriesImageBlocks proves image tool outputs reach
// Claude inside tool_result content blocks, in order, without the Chat carrier.
func TestConvertRequestToolResultCarriesImageBlocks(t *testing.T) {
	t.Parallel()
	messages := encodedClaudeMessages(t, toolMediaChatRequest(true))
	require.Len(t, messages, 5, "the carrier must be folded into the tool_result")

	viewResult := messages[2]["content"].([]any)[0].(map[string]any)
	require.Equal(t, "tool_result", viewResult["type"])
	require.Equal(t, "toolu_view", viewResult["tool_use_id"])
	blocks, ok := viewResult["content"].([]any)
	require.True(t, ok, "image tool results must use a content block array")
	require.Equal(t, []any{
		map[string]any{"type": "text", "text": "a.png"},
		map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": toolMediaPayload}},
	}, blocks)

	shellResult := messages[3]["content"].([]any)[0].(map[string]any)
	require.Equal(t, "ok", shellResult["content"], "text-only tool results keep string content")
	require.Equal(t, []any{map[string]any{"type": "text", "text": "next"}}, messages[4]["content"])
}

// TestConvertRequestKeepsCarrierWithoutFoldedImages proves the carrier is only
// dropped when its images were actually placed in tool_result blocks.
func TestConvertRequestKeepsCarrierWithoutFoldedImages(t *testing.T) {
	t.Parallel()
	messages := encodedClaudeMessages(t, toolMediaChatRequest(false))
	require.Len(t, messages, 6)
	viewResult := messages[2]["content"].([]any)[0].(map[string]any)
	require.Equal(t, "a.png", viewResult["content"])
	carrier := messages[4]["content"].([]any)
	require.Len(t, carrier, 2)
	require.Equal(t, "image", carrier[1].(map[string]any)["type"])
}

// TestConvertRequestToolResultImageLoadFailure proves an image that cannot be
// loaded fails as a validation error instead of being sent empty.
func TestConvertRequestToolResultImageLoadFailure(t *testing.T) {
	t.Parallel()
	request := toolMediaChatRequest(true)
	request.Messages[2].ToolResultParts = []model.MessageContent{toolMediaImage("http://127.0.0.1/private.png")}
	_, err := ConvertRequest(newThinkingContext(t, "/v1/chat/completions"), request)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validation failed")
}

// TestConvertToolResultMediaBlocksWithoutImages proves text-only parts keep the
// caller on the historical string content.
func TestConvertToolResultMediaBlocksWithoutImages(t *testing.T) {
	t.Parallel()
	blocks, err := convertToolResultMediaBlocks([]model.MessageContent{toolMediaText("only text")})
	require.NoError(t, err)
	require.Nil(t, blocks)
	blocks, err = convertToolResultMediaBlocks(nil)
	require.NoError(t, err)
	require.Nil(t, blocks)
}

// TestContentMarshalJSONContentBlocks proves nested blocks replace string content
// for typed and raw-preserving blocks alike.
func TestContentMarshalJSONContentBlocks(t *testing.T) {
	t.Parallel()
	nested := []Content{{Type: "text", Text: "a"}}
	typed, err := json.Marshal(Content{Type: "tool_result", ToolUseId: "t1", Content: "ignored", ContentBlocks: nested})
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a"}]}`, string(typed))

	var decoded Content
	require.NoError(t, json.Unmarshal([]byte(`{"type":"tool_result","tool_use_id":"t1","content":"old","cache_control":{"type":"ephemeral"}}`), &decoded))
	decoded.ContentBlocks = nested
	raw, err := json.Marshal(decoded)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"tool_result","tool_use_id":"t1","content":[{"type":"text","text":"a"}],"cache_control":{"type":"ephemeral"}}`, string(raw))
}
