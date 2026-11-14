package openai_compatible

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/relay/model"
)

// TestClaudeGoogleToolMetadataBridge verifies the optional gateway carrier survives generic Chat conversion without leaking to unsigned calls.
func TestClaudeGoogleToolMetadataBridge(t *testing.T) {
	calls := `[{"id":"call_one","index":0,"type":"function","function":{"name":"lookup","arguments":"{}"},"extra_content":{"google":{"thought_signature":"c2lnPQ=="}}},{"id":"call_two","index":1,"type":"function","function":{"name":"lookup","arguments":"{}"}}]`
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			c, w := newGinTestContext()
			var blocks []map[string]any
			if !stream {
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","tool_calls":` + calls + `},"finish_reason":"tool_calls"}]}`))}
				converted, failure := ConvertOpenAIResponseToClaudeResponse(c, response)
				require.Nil(t, failure)
				body, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				var wire struct {
					Content []map[string]any `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body, &wire))
				blocks = wire.Content
			} else {
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"tool_calls":` + calls + `},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"))}
				_, failure := ConvertOpenAIStreamToClaudeSSE(c, response, 0, "gemini-3.8-flash")
				require.Nil(t, failure)
				for _, event := range helperParseSSEDataLines(w.Body.String()) {
					if event["type"] == "content_block_start" {
						block := event["content_block"].(map[string]any)
						if block["type"] == "tool_use" {
							blocks = append(blocks, block)
						}
					}
				}
			}
			require.Len(t, blocks, 2)
			require.Contains(t, blocks[0], "extra_content")
			require.NotContains(t, blocks[1], "extra_content")
			require.NotContains(t, blocks[0], "signature")
			require.Equal(t, "c2lnPQ==", blocks[0]["extra_content"].(map[string]any)["google"].(map[string]any)["thought_signature"])
		})
	}
}

// TestClaudeGoogleToolMetadataNativeScope keeps unrelated Claude-to-Chat lowering's established wire contract.
func TestClaudeGoogleToolMetadataNativeScope(t *testing.T) {
	block := map[string]any{"type": "tool_use", "id": "call_one", "name": "lookup", "input": map[string]any{}, "extra_content": map[string]any{"google": map[string]any{"thought_signature": "c2lnPQ=="}}}
	ordinary := convertClaudeMessageToOpenAI(model.ClaudeMessage{Role: "assistant", Content: []any{block}})
	native := convertClaudeMessageToOpenAI(model.ClaudeMessage{Role: "assistant", Content: []any{block}}, true)
	require.Nil(t, ordinary[0].ToolCalls[0].ExtraContent)
	require.Equal(t, "c2lnPQ==", model.ToolThoughtSignature(native[0].ToolCalls[0]))
}

// TestClaudeGoogleTextMetadataBridge preserves text and empty-Part boundaries on generic Chat bridges.
func TestClaudeGoogleTextMetadataBridge(t *testing.T) {
	content := `[{"type":"text","text":"first","extra_content":{"google":{"thought_signature":"dGV4dA==","thought":true}}},{"type":"text","text":"","extra_content":{"google":{"thought_signature":"ZW1wdHk="}}},{"type":"text","text":"last"}]`
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			c, w := newGinTestContext()
			var blocks []map[string]any
			if !stream {
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"choices":[{"message":{"role":"assistant","content":` + content + `},"finish_reason":"stop"}]}`))}
				converted, failure := ConvertOpenAIResponseToClaudeResponse(c, response)
				require.Nil(t, failure)
				body, err := io.ReadAll(converted.Body)
				require.NoError(t, err)
				var wire struct {
					Content []map[string]any `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body, &wire))
				blocks = wire.Content
			} else {
				response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`data: {"choices":[{"delta":{"content":` + content + `},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"))}
				_, failure := ConvertOpenAIStreamToClaudeSSE(c, response, 0, "gemini-3.8-flash")
				require.Nil(t, failure)
				for _, event := range helperParseSSEDataLines(w.Body.String()) {
					if event["type"] == "content_block_start" {
						blocks = append(blocks, event["content_block"].(map[string]any))
					}
				}
			}
			require.Len(t, blocks, 3)
			require.Contains(t, blocks[0], "extra_content")
			require.Contains(t, blocks[1], "extra_content")
			require.NotContains(t, blocks[2], "extra_content")
			require.NotContains(t, blocks[0], "signature")
			require.Equal(t, true, blocks[0]["extra_content"].(map[string]any)["google"].(map[string]any)["thought"])
		})
	}
}

// TestClaudeNativePositionMetadata preserves the gateway position namespace without requiring a Google signature.
func TestClaudeNativePositionMetadata(t *testing.T) {
	c, _ := newGinTestContext()
	var request model.ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","messages":[{"role":"assistant","content":[{"type":"text","text":"first","extra_content":{"oneapi":{"gemini_part_index":0}}}]}]}`), &request))
	converted, err := ConvertClaudeRequestWithNativeMedia(c, &request)
	require.NoError(t, err)
	parts, err := converted.(*model.GeneralOpenAIRequest).Messages[0].ParseContentChecked()
	require.NoError(t, err)
	index, ok := model.GeminiPartIndex(parts[0].ExtraContent)
	require.True(t, ok)
	require.Equal(t, 0, index)
	for _, value := range []string{"1.5", "9223372036854775808", "\"not-an-index\""} {
		raw := `{"model":"gemini-3.8-flash","messages":[{"role":"assistant","content":[{"type":"text","text":"first","extra_content":{"oneapi":{"gemini_part_index":` + value + `}}}]}]}`
		require.NoError(t, json.Unmarshal([]byte(raw), &request))
		_, err := ConvertClaudeRequestWithNativeMedia(c, &request)
		require.ErrorContains(t, err, "invalid native Claude provider metadata")
	}
}
