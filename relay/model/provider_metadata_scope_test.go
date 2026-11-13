package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// signedChatRequest returns a Chat request carrying every typed and untyped carrier position.
func signedChatRequest(t *testing.T) *GeneralOpenAIRequest {
	t.Helper()
	text := "typed"
	request := &GeneralOpenAIRequest{
		Model: "m",
		Messages: []Message{
			{Role: "user", Content: []any{map[string]any{"type": "text", "text": "hi", "extra_content": map[string]any{"google": map[string]any{"thought_signature": "c2ln"}, "oneapi": map[string]any{"gemini_part_index": 0}}}}},
			{Role: "user", Content: []MessageContent{{Type: ContentTypeText, Text: &text, ExtraContent: WithGeminiTextPresent(GoogleThoughtSignature("c2lnLXR5cGVk"), true)}}},
			{Role: "assistant", Content: "", ExtraContent: WithGeminiPartIndex(nil, 1), ToolCalls: []Tool{
				{Id: "call_1", Type: "function", Function: &Function{Name: "lookup", Arguments: `{"extra_content":"user"}`}, ExtraContent: WithGeminiPartIndex(GoogleThoughtSignature("c2lnLWNhbGw="), 0)},
				{Id: "call_2", Type: "function", Function: &Function{Name: "lookup", Arguments: `{}`}},
			}},
			{Role: "tool", ToolCallId: "call_1", Content: "ok"},
		},
		Tools: []Tool{{Type: "function", Function: &Function{Name: "lookup"}, ExtraContent: GoogleThoughtSignature("c2lnLXRvb2w=")}},
	}
	return request
}

// chatSliceSnapshot serializes the message and tool slices a holder captured before scoping.
func chatSliceSnapshot(t *testing.T, messages []Message, tools []Tool) string {
	t.Helper()
	encoded, err := json.Marshal([]any{messages, tools})
	require.NoError(t, err)
	return string(encoded)
}

// TestScopeChatRequestProviderMetadata covers each scope, every carrier position, in-place request
// identity (billing reads adaptor normalization from it) and copy-on-write of shared slice data.
func TestScopeChatRequestProviderMetadata(t *testing.T) {
	t.Run("native_keeps_identity", func(t *testing.T) {
		request := signedChatRequest(t)
		messages, tools := request.Messages, request.Tools
		require.Zero(t, ScopeChatRequestProviderMetadata(request, ProviderMetadataNative))
		require.Same(t, &messages[0], &request.Messages[0])
		require.Same(t, &tools[0], &request.Tools[0])
	})
	t.Run("none_strips_every_carrier", func(t *testing.T) {
		request := signedChatRequest(t)
		messages, tools := request.Messages, request.Tools
		before := chatSliceSnapshot(t, messages, tools)
		require.Equal(t, 5, ScopeChatRequestProviderMetadata(request, ProviderMetadataNone))
		require.Equal(t, before, chatSliceSnapshot(t, messages, tools), "previously shared slices, maps and carriers must not change")
		wire, err := json.Marshal(request)
		require.NoError(t, err)
		require.NotContains(t, string(wire), `"extra_content":{`)
		require.Contains(t, string(wire), `{\"extra_content\":\"user\"}`, "argument data is caller content")
		require.Equal(t, messages[3], request.Messages[3])
		require.Zero(t, ScopeChatRequestProviderMetadata(request, ProviderMetadataNone), "scoping is idempotent")
	})
	t.Run("google_wire_keeps_only_google", func(t *testing.T) {
		request := signedChatRequest(t)
		messages := request.Messages
		before := chatSliceSnapshot(t, messages, request.Tools)
		tool := request.Tools[0].ExtraContent
		require.Equal(t, 4, ScopeChatRequestProviderMetadata(request, ProviderMetadataGoogleWire), "the gateway-only message carrier is removed and three carriers lose oneapi; the Google-only tool carrier is unchanged")
		require.Nil(t, request.Messages[2].ExtraContent)
		require.Equal(t, &ToolExtraContent{Google: &GoogleToolExtraContent{ThoughtSignature: "c2lnLWNhbGw="}}, request.Messages[2].ToolCalls[0].ExtraContent)
		require.Same(t, tool, request.Tools[0].ExtraContent, "a Google-only carrier is reused unchanged")
		require.Equal(t, map[string]any{"google": map[string]any{"thought_signature": "c2ln"}}, request.Messages[0].Content.([]any)[0].(map[string]any)["extra_content"])
		require.Nil(t, request.Messages[1].Content.([]MessageContent)[0].ExtraContent.OneAPI)
		require.Equal(t, before, chatSliceSnapshot(t, messages, request.Tools), "previously shared data must not change")
		require.Contains(t, messages[0].Content.([]any)[0].(map[string]any)["extra_content"], "oneapi")
		require.NotNil(t, messages[1].Content.([]MessageContent)[0].ExtraContent.OneAPI)
	})
	t.Run("case_folded_part_key", func(t *testing.T) {
		request := &GeneralOpenAIRequest{Messages: []Message{{Role: "user", Content: []any{map[string]any{"type": "text", "text": "hi", "Extra_Content": map[string]any{"google": map[string]any{}}}}}}}
		require.Equal(t, 1, ScopeChatRequestProviderMetadata(request, ProviderMetadataNone))
		require.NotContains(t, request.Messages[0].Content.([]any)[0], "Extra_Content")
	})
	t.Run("carrier_free_request_is_untouched", func(t *testing.T) {
		request := &GeneralOpenAIRequest{Model: "m", Messages: []Message{{Role: "user", Content: "hi"}}}
		messages := request.Messages
		require.Zero(t, ScopeChatRequestProviderMetadata(request, ProviderMetadataNone))
		require.Same(t, &messages[0], &request.Messages[0])
	})
}

// TestScopeRawProviderMetadata covers positional raw scoping for the three wire formats.
func TestScopeRawProviderMetadata(t *testing.T) {
	t.Run("chat_google_wire", func(t *testing.T) {
		body := []byte(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"text","text":"<b>","extra_content":{"oneapi":{"gemini_text_present":true}}}],"tool_calls":[{"id":"c","function":{"name":"f","arguments":"{\"extra_content\":1}"},"extra_content":{"google":{"thought_signature":"c2ln"},"oneapi":{"gemini_part_index":0}}}],"vendor":9007199254740993}],"tools":[{"type":"function","function":{"name":"f","parameters":{"properties":{"extra_content":{"type":"string"}}}}}]}`)
		scoped, count, err := ScopeChatBodyProviderMetadata(body, ProviderMetadataGoogleWire)
		require.NoError(t, err)
		require.Equal(t, 2, count)
		require.JSONEq(t, `{"model":"m","messages":[{"role":"assistant","content":[{"type":"text","text":"<b>"}],"tool_calls":[{"id":"c","function":{"name":"f","arguments":"{\"extra_content\":1}"},"extra_content":{"google":{"thought_signature":"c2ln"}}}],"vendor":9007199254740993}],"tools":[{"type":"function","function":{"name":"f","parameters":{"properties":{"extra_content":{"type":"string"}}}}}]}`, string(scoped))
		require.Contains(t, string(scoped), "9007199254740993", "number literals keep their exact bytes")
		require.Contains(t, string(scoped), `"<b>"`, "strings are not HTML-escaped")
	})
	t.Run("chat_untouched_bodies_are_identical", func(t *testing.T) {
		for _, body := range []string{
			`{"messages":[{"role":"user","content":"hi"}]}`,
			`{"messages":[{"role":"user","content":"hi"}],"tools":[{"function":{"parameters":{"properties":{"extra_content":{}}}}}]}`,
			`{"messages":"not-an-array","extra_content":{"google":{}}}`,
		} {
			scoped, count, err := ScopeChatBodyProviderMetadata([]byte(body), ProviderMetadataNone)
			require.NoError(t, err)
			require.Zero(t, count)
			require.Equal(t, body, string(scoped))
		}
	})
	t.Run("chat_native_is_identity", func(t *testing.T) {
		body := []byte(`{"messages":[{"extra_content":{"google":{}}}]}`)
		scoped, count, err := ScopeChatBodyProviderMetadata(body, ProviderMetadataNative)
		require.NoError(t, err)
		require.Zero(t, count)
		require.Equal(t, body, scoped)
	})
	t.Run("claude_messages", func(t *testing.T) {
		messages := json.RawMessage(`[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"f","input":{"extra_content":"user"},"extra_content":{"google":{"thought_signature":"c2ln"}}}]},{"role":"user","content":"hi"}]`)
		scoped, count, err := StripClaudeMessagesProviderMetadata(messages)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.JSONEq(t, `[{"role":"assistant","content":[{"type":"tool_use","id":"t","name":"f","input":{"extra_content":"user"}}]},{"role":"user","content":"hi"}]`, string(scoped))
	})
	t.Run("responses_input", func(t *testing.T) {
		input := json.RawMessage(`[{"type":"function_call","call_id":"c","arguments":"{}","extra_content":{"google":{"thought_signature":"c2ln"}}},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"","extra_content":{"oneapi":{"gemini_text_present":true}}}]}]`)
		scoped, count, err := StripResponsesInputProviderMetadata(input)
		require.NoError(t, err)
		require.Equal(t, 2, count)
		require.JSONEq(t, `[{"type":"function_call","call_id":"c","arguments":"{}"},{"type":"message","role":"assistant","content":[{"type":"output_text","text":""}]}]`, string(scoped))
		text := json.RawMessage(`"extra_content"`)
		same, count, err := StripResponsesInputProviderMetadata(text)
		require.NoError(t, err)
		require.Zero(t, count)
		require.Equal(t, text, same)
	})
	t.Run("case_folded_and_escaped_keys", func(t *testing.T) {
		for key, canonical := range map[string]bool{`Extra_Content`: false, `EXTRA_CONTENT`: false, `extra\u005fcontent`: true, `extr\u0061_content`: true} {
			body := []byte(`{"messages":[{"role":"assistant","content":"x","` + key + `":{"google":{"thought_signature":"c2ln"}}}]}`)
			scoped, count, err := ScopeChatBodyProviderMetadata(body, ProviderMetadataNone)
			require.NoError(t, err)
			require.Equal(t, 1, count, key)
			require.JSONEq(t, `{"messages":[{"role":"assistant","content":"x"}]}`, string(scoped), "every spelling the decoder binds is a carrier")
			scoped, count, err = ScopeChatBodyProviderMetadata(body, ProviderMetadataGoogleWire)
			require.NoError(t, err)
			if canonical {
				require.Zero(t, count, "an escaped canonical key is the documented Google field")
				require.Equal(t, string(body), string(scoped))
				continue
			}
			require.Equal(t, 1, count, key)
			require.JSONEq(t, `{"messages":[{"role":"assistant","content":"x"}]}`, string(scoped), "case variants are never a Google field")
		}
	})
	t.Run("claude_metadata_only_text_blocks", func(t *testing.T) {
		messages := json.RawMessage(`[{"role":"user","content":"go"},` +
			`{"role":"assistant","content":[{"type":"text","text":"","extra_content":{"google":{"thought_signature":"c2ln"}}},{"type":"text","text":"kept","extra_content":{"oneapi":{"gemini_part_index":1}}},{"type":"text","text":""}]},` +
			`{"role":"assistant","content":[{"type":"text","extra_content":{"google":{"thought_signature":"c2ln"}}}]},` +
			`{"role":"user","content":[{"type":"text","text":"","extra_content":{"google":{}}}]}]`)
		scoped, count, err := StripClaudeMessagesProviderMetadata(messages)
		require.NoError(t, err)
		require.Equal(t, 4, count)
		require.JSONEq(t, `[{"role":"user","content":"go"},`+
			`{"role":"assistant","content":[{"type":"text","text":"kept"},{"type":"text","text":""}]},`+
			`{"role":"user","content":[{"type":"text","text":""}]}]`, string(scoped),
			"metadata-only assistant text blocks go with their carrier; caller-authored blocks and non-assistant turns keep their shape")
	})
	t.Run("invalid_subtree_is_an_error", func(t *testing.T) {
		_, _, err := ScopeChatBodyProviderMetadata([]byte(`{"messages":[{"extra_content":`), ProviderMetadataNone)
		require.Error(t, err)
	})
}
