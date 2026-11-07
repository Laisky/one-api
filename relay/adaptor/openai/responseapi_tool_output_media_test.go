package openai

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// toolImageDataURL is a small inline image in the data URL shape Codex sends.
const toolImageDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

// decodeToolMediaRequest decodes a wire Responses payload the way the relay does.
// Parameters: t is the test handle and payload the JSON body. Returns: the request.
func decodeToolMediaRequest(t *testing.T, payload string) *ResponseAPIRequest {
	t.Helper()
	var request ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &request))
	return &request
}

// convertToolMediaRequest converts a wire Responses payload to Chat.
// Parameters: t is the test handle and payload the JSON body. Returns: the Chat request.
func convertToolMediaRequest(t *testing.T, payload string) *model.GeneralOpenAIRequest {
	t.Helper()
	converted, err := ConvertResponseAPIToChatCompletionRequest(decodeToolMediaRequest(t, payload))
	require.NoError(t, err)
	return converted
}

// TestResponseToolOutputImagesFollowParallelToolGroup proves image tool outputs
// keep their text in the tool messages, carry ordered parts on the side, and get
// exactly one carrier user message after the whole parallel tool group.
func TestResponseToolOutputImagesFollowParallelToolGroup(t *testing.T) {
	t.Parallel()
	payload := `{"model":"gpt-x","input":[
		{"role":"user","content":[{"type":"input_text","text":"look"}]},
		{"type":"function_call","call_id":"c1","name":"shell","arguments":"{}"},
		{"type":"function_call","call_id":"c2","name":"view_image","arguments":"{\"path\":\"a.png\"}"},
		{"type":"function_call","call_id":"c3","name":"view_image","arguments":"{\"path\":\"b.png\"}"},
		{"type":"function_call_output","call_id":"c1","output":"ok"},
		{"type":"function_call_output","call_id":"c2","output":[{"type":"input_text","text":"a.png"},{"type":"input_image","image_url":"` + toolImageDataURL + `","detail":"original"}]},
		{"type":"function_call_output","call_id":"c3","output":[{"type":"input_image","image_url":"https://example.com/b.png"}]},
		{"role":"user","content":[{"type":"input_text","text":"next"}]}
	]}`
	messages := convertToolMediaRequest(t, payload).Messages
	require.Len(t, messages, 7)
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	require.Equal(t, []string{"user", "assistant", "tool", "tool", "tool", "user", "user"}, roles)
	require.Len(t, messages[1].ToolCalls, 3)

	require.Equal(t, "ok", messages[2].Content)
	require.Empty(t, messages[2].ToolResultParts)

	require.Equal(t, "c2", messages[3].ToolCallId)
	require.Equal(t, "a.png", messages[3].Content)
	require.Len(t, messages[3].ToolResultParts, 2)
	require.Equal(t, model.ContentTypeText, messages[3].ToolResultParts[0].Type)
	require.Equal(t, model.ContentTypeImageURL, messages[3].ToolResultParts[1].Type)
	require.Equal(t, toolImageDataURL, messages[3].ToolResultParts[1].ImageURL.Url)
	require.Equal(t, "high", messages[3].ToolResultParts[1].ImageURL.Detail)

	require.Equal(t, responseToolOutputImagePlaceholder, messages[4].Content)
	require.Len(t, messages[4].ToolResultParts, 1)

	carrier := messages[5]
	require.True(t, carrier.ToolResultMediaCarrier)
	parts, ok := carrier.Content.([]model.MessageContent)
	require.True(t, ok)
	require.Len(t, parts, 4)
	require.Equal(t, "[Image output from tool call c2]", *parts[0].Text)
	require.Equal(t, toolImageDataURL, parts[1].ImageURL.Url)
	require.Equal(t, "[Image output from tool call c3]", *parts[2].Text)
	require.Equal(t, "https://example.com/b.png", parts[3].ImageURL.Url)
	require.Empty(t, parts[3].ImageURL.Detail)
	require.NotSame(t, messages[3].ToolResultParts[1].ImageURL, parts[1].ImageURL, "carrier must not alias tool parts")

	require.Equal(t, "next", messages[6].StringContent())
	require.False(t, messages[6].ToolResultMediaCarrier)
}

// TestResponseToolOutputImagesWireShape proves the gateway-internal fields never
// reach a Chat upstream: tool content stays a string and the carrier is plain.
func TestResponseToolOutputImagesWireShape(t *testing.T) {
	t.Parallel()
	payload := `{"model":"gpt-x","input":[
		{"type":"function_call","call_id":"c2","name":"view_image","arguments":"{}"},
		{"type":"function_call_output","call_id":"c2","output":[{"type":"input_image","image_url":"` + toolImageDataURL + `","detail":"low"}]}
	]}`
	encoded, err := json.Marshal(convertToolMediaRequest(t, payload))
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "ToolResult")

	var wire struct {
		Messages []map[string]any `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	require.Len(t, wire.Messages, 3)
	require.Equal(t, "tool", wire.Messages[1]["role"])
	require.Equal(t, responseToolOutputImagePlaceholder, wire.Messages[1]["content"])
	require.Equal(t, "user", wire.Messages[2]["role"])
	content, ok := wire.Messages[2]["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 2)
	image, ok := content[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "image_url", image["type"])
	require.Equal(t, map[string]any{"url": toolImageDataURL, "detail": "low"}, image["image_url"])
}

// TestResponseToolOutputWithoutImagesKeepsLegacyText proves outputs with no
// forwardable image keep the exact historical JSON stringification.
func TestResponseToolOutputWithoutImagesKeepsLegacyText(t *testing.T) {
	t.Parallel()
	for name, output := range map[string]string{
		"string":         `"plain result"`,
		"text_array":     `[{"type":"input_text","text":"hello"}]`,
		"object":         `{"exit_code":0}`,
		"file_id_image":  `[{"type":"input_text","text":"see file"},{"type":"input_image","file_id":"file-abc"}]`,
		"raw_file_data":  `[{"type":"input_image","file_data":"iVBORw0KGgo="}]`,
		"ftp_image":      `[{"type":"input_image","image_url":"ftp://example.com/a.png"}]`,
		"uppercase_data": `[{"type":"input_image","image_url":"DATA:IMAGE/PNG;BASE64,AAAA"}]`,
		"mixed_file":     `[{"type":"input_image","image_url":"` + toolImageDataURL + `"},{"type":"input_image","file_id":"file-abc"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			payload := `{"model":"gpt-x","input":[
				{"type":"function_call","call_id":"c1","name":"tool","arguments":"{}"},
				{"type":"function_call_output","call_id":"c1","output":` + output + `}
			]}`
			request := decodeToolMediaRequest(t, payload)
			expected := stringifyFunctionCallOutput(request.Input[1].(map[string]any)["output"])
			messages := convertToolMediaRequest(t, payload).Messages
			require.Len(t, messages, 2, "no carrier may be added")
			require.Equal(t, expected, messages[1].Content)
			require.Empty(t, messages[1].ToolResultParts)
		})
	}
}

// TestResponseCustomToolOutputImages proves custom tool outputs share the image path.
func TestResponseCustomToolOutputImages(t *testing.T) {
	t.Parallel()
	payload := `{"model":"gpt-x","tools":[{"type":"custom","name":"apply_patch","description":"patch files"}],"input":[
		{"type":"custom_tool_call","call_id":"c9","name":"apply_patch","input":"*** Begin Patch"},
		{"type":"custom_tool_call_output","call_id":"c9","output":[{"type":"input_text","text":"rendered"},{"type":"input_image","image_url":"` + toolImageDataURL + `"}]}
	]}`
	messages := convertToolMediaRequest(t, payload).Messages
	require.Len(t, messages, 3)
	require.Equal(t, "tool", messages[1].Role)
	require.Equal(t, "rendered", messages[1].Content)
	require.Len(t, messages[1].ToolResultParts, 2)
	require.True(t, messages[2].ToolResultMediaCarrier)
}

// TestResponseOrphanToolOutputImagesBecomeUserParts proves an orphan output with
// images is downgraded to a user message that carries the images in order.
func TestResponseOrphanToolOutputImagesBecomeUserParts(t *testing.T) {
	t.Parallel()
	payload := `{"model":"gpt-x","input":[
		{"type":"function_call_output","call_id":"missing","output":[{"type":"input_image","image_url":"` + toolImageDataURL + `"},{"type":"input_text","text":"after"}]}
	]}`
	messages := convertToolMediaRequest(t, payload).Messages
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].Role)
	require.False(t, messages[0].ToolResultMediaCarrier)
	parts, ok := messages[0].Content.([]model.MessageContent)
	require.True(t, ok)
	require.Len(t, parts, 2)
	require.Equal(t, model.ContentTypeImageURL, parts[0].Type)
	require.Equal(t, "after", *parts[1].Text)
}

// TestResponseToolOutputImagesKeepUnknownPartsInPlace proves unknown parts next
// to images keep the JSON text the legacy stringification exposed, in order.
func TestResponseToolOutputImagesKeepUnknownPartsInPlace(t *testing.T) {
	t.Parallel()
	parts, ok := responseToolOutputMediaParts([]any{
		map[string]any{"type": "input_file", "filename": "notes.txt"},
		map[string]any{"type": "input_image", "image_url": toolImageDataURL},
		"loose",
	})
	require.True(t, ok)
	require.Len(t, parts, 3)
	require.JSONEq(t, `{"type":"input_file","filename":"notes.txt"}`, *parts[0].Text)
	require.Equal(t, model.ContentTypeImageURL, parts[1].Type)
	require.Equal(t, `"loose"`, *parts[2].Text)
}

// TestResponseToolOutputFidelityAdmission covers the explicit admission policy
// for function and custom tool output arrays.
func TestResponseToolOutputFidelityAdmission(t *testing.T) {
	t.Parallel()
	image := map[string]any{"type": "input_image", "image_url": toolImageDataURL}
	cases := map[string]struct {
		output any
		reject bool
	}{
		"string":                     {output: "ok"},
		"object":                     {output: map[string]any{"k": "v"}},
		"text_only_bad_text_legacy":  {output: []any{map[string]any{"type": "input_text", "text": 5}}},
		"unknown_kinds_legacy":       {output: []any{map[string]any{"type": "input_file", "file_data": "abc"}, map[string]any{"type": "weird"}}},
		"image_with_text":            {output: []any{map[string]any{"type": "input_text", "text": "a"}, image}},
		"image_with_unknown":         {output: []any{map[string]any{"type": "weird"}, image}},
		"file_id_image":              {output: []any{map[string]any{"type": "input_image", "file_id": "file-abc"}}},
		"image_without_source":       {output: []any{map[string]any{"type": "input_image", "detail": "high"}}, reject: true},
		"image_with_object_url":      {output: []any{map[string]any{"type": "input_image", "image_url": map[string]any{"url": "https://example.com/a.png"}}}, reject: true},
		"image_with_non_string_text": {output: []any{map[string]any{"type": "input_text", "text": 5}, image}, reject: true},
	}
	for name, tc := range cases {
		for _, kind := range []string{"function_call_output", "custom_tool_call_output"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				t.Parallel()
				err := ValidateResponseAPIFallbackFidelity([]any{map[string]any{"type": kind, "call_id": "c1", "output": tc.output}}, false)
				if tc.reject {
					require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
					return
				}
				require.NoError(t, err)
			})
		}
	}
}

// TestResponseToolOutputImageCountBound proves the per-request image cap.
func TestResponseToolOutputImageCountBound(t *testing.T) {
	t.Parallel()
	build := func(images int) []any {
		input := make([]any, 0, images)
		for i := range images {
			input = append(input, map[string]any{
				"type":    "function_call_output",
				"call_id": "c" + strconv.Itoa(i),
				"output":  []any{map[string]any{"type": "input_image", "image_url": toolImageDataURL}},
			})
		}
		return input
	}
	require.NoError(t, ValidateResponseAPIFallbackFidelity(build(maxResponseFallbackToolOutputImages), false))
	err := ValidateResponseAPIFallbackFidelity(build(maxResponseFallbackToolOutputImages+1), false)
	require.ErrorIs(t, err, ErrResponseFallbackNotRepresentable)
	require.Contains(t, err.Error(), "more than 100 images")
}

// TestResponseToolOutputImageURLRepresentable pins which image URLs are forwarded as images.
func TestResponseToolOutputImageURLRepresentable(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]bool{
		toolImageDataURL:                      true,
		"data:image/webp;base64,AAAA":         true,
		"https://example.com/a.png":           true,
		"HTTP://example.com/a.png":            true,
		"https://":                            false,
		"data:image/png;base64,":              false,
		"data:image/;base64,AAAA":             false,
		"data:image/png,AAAA":                 false,
		"data:image/png;charset=x;base64,AAA": false,
		"data:text/plain;base64,AAAA":         false,
		"file:///etc/passwd":                  false,
		"":                                    false,
		strings.Repeat("a", 64):               false,
	} {
		require.Equal(t, want, responseToolOutputImageURLRepresentable(url), url)
	}
}

// TestNormalizeResponseToolOutputImageDetail pins the Chat detail vocabulary.
func TestNormalizeResponseToolOutputImageDetail(t *testing.T) {
	t.Parallel()
	for raw, want := range map[any]string{
		"low": "low", "HIGH": "high", "original": "high", "auto": "auto",
		"": "", "ultra": "", 3: "",
	} {
		require.Equal(t, want, normalizeResponseToolOutputImageDetail(raw))
	}
	require.Empty(t, normalizeResponseToolOutputImageDetail(nil))
}
