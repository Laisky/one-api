package controller

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/logger"
	"github.com/Laisky/one-api/relay/adaptor/openai"
	"github.com/Laisky/one-api/relay/adaptor/openai_compatible"
	"github.com/Laisky/one-api/relay/model"
)

// assertVertexCallSignature checks the signature and identity of one serialized call.
func assertVertexCallSignature(t *testing.T, value any, id, signature string) {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.Equal(t, id, wire["call_id"])
	if signature == "" {
		require.NotContains(t, wire, "extra_content")
		return
	}
	extra, ok := wire["extra_content"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"thought_signature": signature}, extra["google"])
}

// TestVertexResponsesCallSignatureBuffered verifies independent signatures survive buffered rendering and replay.
func TestVertexResponsesCallSignatureBuffered(t *testing.T) {
	var choices []openai_compatible.TextResponseChoice
	require.NoError(t, json.Unmarshal([]byte(`[{"message":{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"one","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLWE="}}},{"id":"call_b","type":"function","function":{"name":"two","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLWI="}}},{"id":"call_c","type":"function","function":{"name":"three","arguments":"{}"}}]}}]`), &choices))
	output := buildResponseOutput(logger.Logger, choices, nil)
	require.Len(t, output, 3)
	assertVertexCallSignature(t, output[0], "call_a", "c2lnLWE=")
	assertVertexCallSignature(t, output[1], "call_b", "c2lnLWI=")
	assertVertexCallSignature(t, output[2], "call_c", "")
}

// TestVertexResponsesCallSignatureReplay verifies public function-call metadata reaches the Chat carrier without mutation.
func TestVertexResponsesCallSignatureReplay(t *testing.T) {
	raw := []byte(`{"model":"gemini-3.8-flash","input":[{"type":"function_call","id":"call_a","call_id":"call_a","name":"one","arguments":"{}","extra_content":{"google":{"thought_signature":"c2lnLWE="}}},{"type":"function_call_output","call_id":"call_a","output":"done"}]}`)
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal(raw, &request))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
	require.NoError(t, err)
	require.NotEmpty(t, chat.Messages)
	var calls []model.Tool
	for _, m := range chat.Messages {
		calls = append(calls, m.ToolCalls...)
	}
	require.Len(t, calls, 1)
	encoded, err := json.Marshal(calls[0])
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"thought_signature":"c2lnLWE="`)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

// TestVertexResponsesCallSignatureStream checks added, done and terminal item carriers preserve per-call signatures.
func TestVertexResponsesCallSignatureStream(t *testing.T) {
	c, w := newBridgeTestContext(t)
	bridge := newTestBridge(t, c)
	for _, raw := range []string{
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_a","index":0,"type":"function","function":{"name":"one","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLWE="}}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"id":"call_b","index":1,"type":"function","function":{"name":"two","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnLWI="}}}]}}]}`,
	} {
		var chunk openai_compatible.ChatCompletionsStreamResponse
		require.NoError(t, json.Unmarshal([]byte(raw), &chunk))
		ok, _ := bridge.HandleChunk(c, &chunk)
		require.True(t, ok)
	}
	bridge.HandleChunk(c, bridgeFinishChunk("tool_calls"))
	bridge.HandleDone(c)
	events := parseBridgeSSE(w.Body.String())
	for _, kind := range []string{"response.output_item.added", "response.output_item.done"} {
		got := bridgeFindEvents(events, kind)
		require.Len(t, got, 2)
		for i, event := range got {
			var parsed struct {
				Item map[string]any `json:"item"`
			}
			require.NoError(t, json.Unmarshal(event.data, &parsed))
			assertVertexCallSignature(t, parsed.Item, []string{"call_a", "call_b"}[i], []string{"c2lnLWE=", "c2lnLWI="}[i])
		}
	}
	terminal := bridgeFindEvents(events, "response.completed")
	require.Len(t, terminal, 1)
	var final struct {
		Response struct {
			Output []map[string]any `json:"output"`
		} `json:"response"`
	}
	require.NoError(t, json.Unmarshal(terminal[0].data, &final))
	require.Len(t, final.Response.Output, 2)
	assertVertexCallSignature(t, final.Response.Output[0], "call_a", "c2lnLWE=")
	assertVertexCallSignature(t, final.Response.Output[1], "call_b", "c2lnLWI=")
}

// TestVertexResponsesCallMetadataValidation keeps native malformed metadata controlled and other providers unchanged.
func TestVertexResponsesCallMetadataValidation(t *testing.T) {
	var request openai.ResponseAPIRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","input":[{"type":"function_call","call_id":"call_a","name":"one","arguments":"{}","extra_content":{"google":{"thought_signature":7}}},{"type":"function_call_output","call_id":"call_a","output":"done"}]}`), &request))
	_, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, true)
	require.ErrorContains(t, err, "Google function call metadata")
	chat, err := openai.ConvertResponseAPIToChatCompletionRequest(&request, false)
	require.NoError(t, err)
	encoded, err := json.Marshal(chat)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "extra_content")
}
