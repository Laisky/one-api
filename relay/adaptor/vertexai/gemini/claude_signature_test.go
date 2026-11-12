package vertexai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/Laisky/one-api/common/ctxkey"
	"github.com/Laisky/one-api/relay/adaptor/anthropic"
	"github.com/Laisky/one-api/relay/adaptor/gemini"
	"github.com/Laisky/one-api/relay/meta"
	"github.com/Laisky/one-api/relay/model"
	"github.com/Laisky/one-api/relay/relaymode"
)

// claudeSignatureContext constructs an offline Claude request context.
func claudeSignatureContext() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	return c
}

// TestVertexClaudeSignedCallRoundTrip checks actual Vertex response routing and exact signature replay.
func TestVertexClaudeSignedCallRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			c := claudeSignatureContext()
			c.Set(ctxkey.ClaudeMessagesConversion, true)
			fixture := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"q":"one"}},"thoughtSignature":"c2lnKzEvPQ=="},{"functionCall":{"name":"lookup","args":{"q":"two"}},"thoughtSignature":"c2lnKzIvPQ=="}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"totalTokenCount":8}}`
			contentType := "application/json"
			if stream {
				fixture = "data: " + fixture + "\n\ndata: [DONE]\n\n"
				contentType = "text/event-stream"
			}
			response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(fixture))}
			usage, failure := (&Adaptor{}).DoResponse(c, response, &meta.Meta{ActualModelName: "gemini-3.8-flash", Mode: relaymode.ChatCompletions, IsStream: stream})
			require.Nil(t, failure)
			require.Nil(t, usage)
			converted, exists := c.Get(ctxkey.ConvertedResponse)
			require.True(t, exists, "Vertex must produce Claude format")
			body, err := io.ReadAll(converted.(*http.Response).Body)
			require.NoError(t, err)
			var blocks []any
			if !stream {
				var wire struct {
					Content []any `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body, &wire))
				blocks = wire.Content
			} else {
				blocks = claudeSignatureStreamBlocks(t, body)
			}
			require.Len(t, blocks, 2)
			for i, sig := range []string{"c2lnKzEvPQ==", "c2lnKzIvPQ=="} {
				block := blocks[i].(map[string]any)
				require.NotContains(t, block, "signature")
				require.Contains(t, block, "extra_content")
				require.Equal(t, map[string]any{"q": []string{"one", "two"}[i]}, block["input"])
				require.Equal(t, sig, block["extra_content"].(map[string]any)["google"].(map[string]any)["thought_signature"])
			}
			replay := &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 32, Messages: []model.ClaudeMessage{{Role: "assistant", Content: blocks}, {Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": blocks[0].(map[string]any)["id"], "content": "one"}, map[string]any{"type": "tool_result", "tool_use_id": blocks[1].(map[string]any)["id"], "content": "two"}}}}}
			chat, err := gemini.ConvertClaudeToNativeChat(claudeSignatureContext(), replay)
			require.NoError(t, err)
			native, err := (&Adaptor{}).ConvertRequest(claudeSignatureContext(), relaymode.ChatCompletions, chat)
			require.NoError(t, err)
			raw, err := json.Marshal(native)
			require.NoError(t, err)
			var wire struct {
				Contents []struct {
					Parts []map[string]any `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(raw, &wire))
			require.Len(t, wire.Contents[0].Parts, 2)
			require.Equal(t, "c2lnKzEvPQ==", wire.Contents[0].Parts[0]["thoughtSignature"])
			require.Equal(t, "c2lnKzIvPQ==", wire.Contents[0].Parts[1]["thoughtSignature"])
		})
	}
}

// TestVertexClaudeSignatureReplayAndAnthropicControl verifies the extension is opaque and not an Anthropic thinking signature.
func TestVertexClaudeSignatureReplayAndAnthropicControl(t *testing.T) {
	var request model.ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","max_tokens":32,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_one","name":"lookup","input":{},"extra_content":{"google":{"thought_signature":"c2lnKzEvPQ=="}}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_one","content":"done"}]}]}`), &request))
	before, err := json.Marshal(request)
	require.NoError(t, err)
	chat, err := gemini.ConvertClaudeToNativeChat(claudeSignatureContext(), &request)
	require.NoError(t, err)
	native, err := (&Adaptor{}).ConvertRequest(claudeSignatureContext(), relaymode.ChatCompletions, chat)
	require.NoError(t, err)
	raw, err := json.Marshal(native)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"thoughtSignature":"c2lnKzEvPQ=="`)
	after, err := json.Marshal(request)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	request.Model = "claude-sonnet-4-6"
	other, err := anthropic.ConvertClaudeRequest(claudeSignatureContext(), request)
	require.NoError(t, err)
	raw, err = json.Marshal(other)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "thought_signature")
	require.NotContains(t, string(raw), "thoughtSignature")
	require.NotContains(t, string(raw), "extra_content")
}

// TestVertexPlainChatRemainsChat verifies that Claude routing is scoped to the conversion flag.
func TestVertexPlainChatRemainsChat(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`))}
	usage, failure := (&Adaptor{}).DoResponse(c, response, &meta.Meta{ActualModelName: "gemini-3.8-flash", Mode: relaymode.ChatCompletions})
	require.Nil(t, failure)
	require.NotNil(t, usage)
	require.Contains(t, recorder.Body.String(), `"choices"`)
	_, exists := c.Get(ctxkey.ConvertedResponse)
	require.False(t, exists)
}

// claudeSignatureStreamBlocks reconstructs tool inputs and their gateway metadata from Claude SSE.
// Parameters: t owns assertions and body is a complete offline SSE fixture. Returns ordered blocks.
func claudeSignatureStreamBlocks(t *testing.T, body []byte) []any {
	t.Helper()
	var blocks []any
	byIndex := map[int]map[string]any{}
	arguments := map[int]string{}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type  string         `json:"type"`
			Index int            `json:"index"`
			Block map[string]any `json:"content_block"`
			Delta struct {
				Partial string `json:"partial_json"`
			} `json:"delta"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		switch event.Type {
		case "content_block_start":
			if event.Block["type"] == "tool_use" {
				byIndex[event.Index] = event.Block
				blocks = append(blocks, event.Block)
			}
		case "content_block_delta":
			arguments[event.Index] += event.Delta.Partial
		}
	}
	for index, block := range byIndex {
		var input any
		require.NoError(t, json.Unmarshal([]byte(arguments[index]), &input))
		block["input"] = input
	}
	return blocks
}

// TestVertexClaudeExplicitToolError preserves Claude's explicit failure status through native lowering.
func TestVertexClaudeExplicitToolError(t *testing.T) {
	var request model.ClaudeRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"gemini-3.8-flash","max_tokens":32,"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call_one","name":"lookup","input":{}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call_one","content":"{\"detail\":\"unavailable\"}","is_error":true}]}]}`), &request))
	chat, err := gemini.ConvertClaudeToNativeChat(claudeSignatureContext(), &request)
	require.NoError(t, err)
	require.True(t, chat.Messages[1].ToolResultIsError)
	native, err := (&Adaptor{}).ConvertRequest(claudeSignatureContext(), relaymode.ChatCompletions, chat)
	require.NoError(t, err)
	raw, err := json.Marshal(native)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"response":{"error":{"detail":"unavailable"}}`)
}

// TestVertexClaudeSignedTextBoundaries preserves opaque metadata on text and empty native Parts in both response modes.
func TestVertexClaudeSignedTextBoundaries(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			c := claudeSignatureContext()
			c.Set(ctxkey.ClaudeMessagesConversion, true)
			fixture := `{"candidates":[{"content":{"role":"model","parts":[{"text":"first","thought":true,"thoughtSignature":"dGV4dDE="},{"thoughtSignature":"ZW1wdHk="},{"functionCall":{"name":"lookup","args":{}},"thoughtSignature":"Y2FsbA=="},{"text":"last","thoughtSignature":"dGV4dDI="}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
			contentType := "application/json"
			if stream {
				fixture = "data: " + fixture + "\n\ndata: [DONE]\n\n"
				contentType = "text/event-stream"
			}
			_, failure := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(fixture))}, &meta.Meta{ActualModelName: "gemini-3.8-flash", Mode: relaymode.ChatCompletions, IsStream: stream})
			require.Nil(t, failure)
			converted, ok := c.Get(ctxkey.ConvertedResponse)
			require.True(t, ok)
			body, err := io.ReadAll(converted.(*http.Response).Body)
			require.NoError(t, err)
			var blocks []any
			if !stream {
				var wire struct {
					Content []any `json:"content"`
				}
				require.NoError(t, json.Unmarshal(body, &wire))
				blocks = wire.Content
			} else {
				blocks = claudeSignatureAllStreamBlocks(t, body)
			}
			require.Len(t, blocks, 4)
			signatures := []string{"dGV4dDE=", "ZW1wdHk=", "Y2FsbA==", "dGV4dDI="}
			for i, sig := range signatures {
				block := blocks[i].(map[string]any)
				require.Contains(t, block, "extra_content")
				require.NotContains(t, block, "signature")
				require.Equal(t, sig, block["extra_content"].(map[string]any)["google"].(map[string]any)["thought_signature"])
			}
			require.Equal(t, "", blocks[1].(map[string]any)["text"])
			replay := &model.ClaudeRequest{Model: "gemini-3.8-flash", MaxTokens: 32, Messages: []model.ClaudeMessage{{Role: "assistant", Content: blocks}, {Role: "user", Content: []any{map[string]any{"type": "tool_result", "tool_use_id": blocks[2].(map[string]any)["id"], "content": "done"}}}}}
			chat, err := gemini.ConvertClaudeToNativeChat(claudeSignatureContext(), replay)
			require.NoError(t, err)
			native, err := (&Adaptor{}).ConvertRequest(claudeSignatureContext(), relaymode.ChatCompletions, chat)
			require.NoError(t, err)
			raw, err := json.Marshal(native)
			require.NoError(t, err)
			var wire struct {
				Contents []struct {
					Parts []map[string]any `json:"parts"`
				} `json:"contents"`
			}
			require.NoError(t, json.Unmarshal(raw, &wire))
			require.Len(t, wire.Contents[0].Parts, 4)
			for i, sig := range signatures {
				require.Equal(t, sig, wire.Contents[0].Parts[i]["thoughtSignature"])
			}
			require.Equal(t, true, wire.Contents[0].Parts[0]["thought"])
		})
	}
}

// claudeSignatureAllStreamBlocks reconstructs ordered text/tool blocks without combining signed boundaries.
func claudeSignatureAllStreamBlocks(t *testing.T, body []byte) []any {
	t.Helper()
	var blocks []any
	byIndex := map[int]map[string]any{}
	arguments := map[int]string{}
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event struct {
			Type  string         `json:"type"`
			Index int            `json:"index"`
			Block map[string]any `json:"content_block"`
			Delta struct {
				Text    string `json:"text"`
				Partial string `json:"partial_json"`
			} `json:"delta"`
		}
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
		if event.Type == "content_block_start" {
			blocks = append(blocks, event.Block)
			byIndex[event.Index] = event.Block
		}
		if event.Type == "content_block_delta" {
			block := byIndex[event.Index]
			if block["type"] == "text" {
				block["text"] = block["text"].(string) + event.Delta.Text
			}
			arguments[event.Index] += event.Delta.Partial
		}
	}
	for index, block := range byIndex {
		if block["type"] == "tool_use" {
			var input any
			require.NoError(t, json.Unmarshal([]byte(arguments[index]), &input))
			block["input"] = input
		}
	}
	return blocks
}

// TestVertexUnsignedClaudeResponseOmitsProviderMetadata protects the existing unsigned Claude wire shape.
func TestVertexUnsignedClaudeResponseOmitsProviderMetadata(t *testing.T) {
	c := claudeSignatureContext()
	c.Set(ctxkey.ClaudeMessagesConversion, true)
	response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"candidates":[{"content":{"role":"model","parts":[{"text":"hello"},{"functionCall":{"name":"lookup","args":{}}}]},"finishReason":"STOP","index":0}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`))}
	_, failure := (&Adaptor{}).DoResponse(c, response, &meta.Meta{ActualModelName: "gemini-3.8-flash", Mode: relaymode.ChatCompletions})
	require.Nil(t, failure)
	converted, ok := c.Get(ctxkey.ConvertedResponse)
	require.True(t, ok)
	body, err := io.ReadAll(converted.(*http.Response).Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "extra_content")
	require.NotContains(t, string(body), "thought_signature")
	require.NotContains(t, string(body), "oneapi")
	require.Contains(t, string(body), `"stop_reason":"tool_use"`)
}

// TestVertexClaudeRejectsIncrementalFunctionFragments prevents unsupported native fragments from becoming empty tool arguments.
func TestVertexClaudeRejectsIncrementalFunctionFragments(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonstream", true: "stream"}[stream], func(t *testing.T) {
			c := claudeSignatureContext()
			c.Set(ctxkey.ClaudeMessagesConversion, true)
			fixture := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","partialArgs":[{"jsonPath":"$.q","stringValue":"one"}],"willContinue":true}}]},"index":0}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}`
			contentType := "application/json"
			if stream {
				fixture = "data: " + fixture + "\n\ndata: [DONE]\n\n"
				contentType = "text/event-stream"
			}
			_, failure := (&Adaptor{}).DoResponse(c, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(strings.NewReader(fixture))}, &meta.Meta{ActualModelName: "gemini-3.8-flash", Mode: relaymode.ChatCompletions, IsStream: stream})
			require.NotNil(t, failure)
			require.Equal(t, http.StatusBadGateway, failure.StatusCode)
			receipt, exists := c.Get(gemini.NativeUsageReceiptKey)
			require.True(t, exists)
			require.Equal(t, 3, receipt.(*model.Usage).TotalTokens)
			_, exists = c.Get(ctxkey.ConvertedResponse)
			require.False(t, exists)
		})
	}
}
